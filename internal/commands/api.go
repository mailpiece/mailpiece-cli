package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/spf13/cobra"
)

var apiCmd = &cobra.Command{
	Use:     "api <verb> <path>",
	Short:   "Raw API access",
	Long:    "Make raw API requests to any Mailpiece endpoint. Useful for operations not covered by dedicated commands. A read token can only GET.",
	Example: "$ mailpiece api get posts.json\n$ mailpiece api post posts.json -d '{\"post\":{\"title\":\"Hello\"}}'",
}

var apiGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "GET request to API",
	Long:    "Make a raw GET request to any Mailpiece API endpoint.",
	Example: "$ mailpiece api get posts.json\n$ mailpiece api get posts/<id>.json\n$ mailpiece api get https://mailpiece.example.com/834206594/posts.json",
	Args:    apiPathArgs,
	RunE:    runAPIGet,
}

var apiPostCmd = &cobra.Command{
	Use:   "post <path>",
	Short: "POST request to API",
	Long: `Make a raw POST request to any Mailpiece API endpoint. Needs a token with write permission.

Use --data - to read the JSON body from stdin:
  printf '{"post":{"title":"Hello"}}' | mailpiece api post posts.json --data -`,
	Example: "$ mailpiece api post posts.json -d '{\"post\":{\"title\":\"Hello\"}}'",
	Args:    apiPathArgs,
	RunE:    runAPIPost,
}

var apiPutCmd = &cobra.Command{
	Use:   "put <path>",
	Short: "PUT request to API",
	Long: `Make a raw PUT request to any Mailpiece API endpoint. Needs a token with write permission.

Use --data - to read the JSON body from stdin:
  printf '{"post":{"title":"Hello again"}}' | mailpiece api put posts/<id>.json --data -`,
	Example: "$ mailpiece api put posts/<id>.json -d '{\"post\":{\"title\":\"Hello again\"}}'",
	Args:    apiPathArgs,
	RunE:    runAPIPut,
}

var apiDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Short:   "DELETE request to API",
	Long:    "Make a raw DELETE request to any Mailpiece API endpoint. Needs a token with write permission.",
	Example: "$ mailpiece api delete posts/<id>.json",
	Args:    apiPathArgs,
	RunE:    runAPIDelete,
}

func init() {
	rootCmd.AddCommand(apiCmd)
	apiCmd.AddCommand(apiGetCmd, apiPostCmd, apiPutCmd, apiDeleteCmd)

	apiPostCmd.Flags().StringP("data", "d", "", "JSON request body (required); use - to read from stdin")
	apiPutCmd.Flags().StringP("data", "d", "", "JSON request body (required); use - to read from stdin")
}

func runAPIGet(cmd *cobra.Command, args []string) error {
	if err := requireAuthAndAccount(); err != nil {
		return err
	}

	path, err := parsePath(args[0], getSDKClient().Config().BaseURL, cfg.Account)
	if err != nil {
		return err
	}
	resp, err := getSDK().Get(cmd.Context(), path)
	if err != nil {
		return convertSDKError(err)
	}

	data := normalizeAny(resp.Data)
	if list, ok := data.([]map[string]any); ok {
		printList(list, inferColumns(list), apiSummary(data), nil)
		return nil
	}
	printDetail(data, apiSummary(data), nil)
	return nil
}

func runAPIPost(cmd *cobra.Command, args []string) error {
	return runAPIWrite(cmd, args, "POST")
}

func runAPIPut(cmd *cobra.Command, args []string) error {
	return runAPIWrite(cmd, args, "PUT")
}

// runAPIWrite sends a POST or PUT with the --data body.
func runAPIWrite(cmd *cobra.Command, args []string, method string) error {
	if err := requireAuth(); err != nil {
		return err
	}
	if err := requireSDK(); err != nil {
		return err
	}
	data, _ := cmd.Flags().GetString("data")
	if data == "" {
		return newRequiredFlagError("data")
	}

	// Refuse a foreign host before the pipe is drained; the rest of the
	// path parse needs the resolved account, so it follows.
	if err := rejectForeignAPIPath(args[0], getSDKClient().Config().BaseURL); err != nil {
		return err
	}

	if data == "-" {
		raw, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return errors.NewError(fmt.Sprintf("Failed to read --data from stdin: %v", err))
		}
		data = string(raw)
	}

	if err := requireAccount(); err != nil {
		return err
	}

	path, err := parsePath(args[0], getSDKClient().Config().BaseURL, cfg.Account)
	if err != nil {
		return err
	}

	var body any
	if err = json.Unmarshal([]byte(data), &body); err != nil {
		return output.ErrUsageHint(
			"Invalid JSON data",
			fmt.Sprintf("JSON parse error: %v", err),
		)
	}

	sdk := getSDK()
	send := sdk.Post
	if method == "PUT" {
		send = sdk.Put
	}
	resp, err := send(cmd.Context(), path, body)
	if err != nil {
		return convertSDKError(err)
	}

	result := normalizeAny(resp.Data)
	printMutation(result, fmt.Sprintf("%s %s: %s", method, path, apiSummary(result)), nil)
	return nil
}

func runAPIDelete(cmd *cobra.Command, args []string) error {
	if err := requireAuthAndAccount(); err != nil {
		return err
	}

	path, err := parsePath(args[0], getSDKClient().Config().BaseURL, cfg.Account)
	if err != nil {
		return err
	}
	resp, err := getSDK().Delete(cmd.Context(), path)
	if err != nil {
		return convertSDKError(err)
	}

	printMutation(normalizeAny(resp.Data), fmt.Sprintf("DELETE %s", path), nil)
	return nil
}

// apiPathArgs validates that exactly one positional arg (the API path) is given.
func apiPathArgs(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return output.ErrUsage("Path required")
	}
	return nil
}

// accountSegmentPattern matches a leading /<account> segment in an API path,
// the account's number or its handle, so it can be dropped: the SDK re-prefixes
// the configured account. The trailing path is optional so a bare "/<account>"
// also matches, leaving m[2] empty.
var accountSegmentPattern = regexp.MustCompile(`^/([0-9]+|[a-z][a-z0-9-]*)(/.*)?$`)

// rejectForeignAPIPath refuses an absolute URL on any host but the configured
// API host, so the token never leaves it. It needs only the base URL, so it
// runs before a "-" drains stdin for an invocation that cannot be sent.
func rejectForeignAPIPath(input, baseURL string) error {
	candidate := strings.TrimLeft(input, "/")
	lower := strings.ToLower(candidate)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return nil
	}
	u, err := url.Parse(candidate)
	if err != nil || u.Host == "" {
		return output.ErrUsage("Invalid API URL: " + input)
	}
	base, baseErr := url.Parse(baseURL)
	if baseErr != nil || base.Host == "" || !sameHostPort(u, base) {
		return output.ErrUsage("API path must be relative or a Mailpiece URL on the configured host; refusing to send credentials to " + input)
	}
	return nil
}

// parsePath normalizes the API path against the configured base URL. It takes
// relative paths ("posts.json", "/posts.json") and absolute URLs
// on the configured host, from which the path is taken and a leading account
// segment dropped. An absolute URL on any other host is refused, so the token
// is never sent to it. A leading account segment must be the configured
// account: the SDK re-prefixes it, so stripping another account's would
// retarget the request. All leading slashes and a mixed-case scheme are
// normalized first so neither "//https://evil/..." nor "HTTPS://evil/..." can
// smuggle an absolute URL past the host check.
func parsePath(input, baseURL, accountID string) (string, error) {
	if err := rejectForeignAPIPath(input, baseURL); err != nil {
		return "", err
	}
	candidate := strings.TrimLeft(input, "/")
	lower := strings.ToLower(candidate)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		u, _ := url.Parse(candidate)
		path := u.EscapedPath()
		if m := accountSegmentPattern.FindStringSubmatch(path); m != nil && !unscopedSegments[m[1]] {
			if accountID != "" && m[1] != accountID {
				return "", output.ErrUsage(fmt.Sprintf("URL is for account %s; refusing to retarget to configured account %s", m[1], accountID))
			}
			if path = m[2]; path == "" {
				path = "/"
			}
		}
		if path == "" {
			path = "/"
		}
		if u.RawQuery != "" {
			path += "?" + u.RawQuery
		}
		return path, nil
	}

	return candidate, nil
}

// sameHostPort reports whether u and base address the same host: hostnames
// compared case-insensitively, ports compared after normalizing an omitted
// port to the scheme default. A matching hostname on a different port is
// still a different origin and must not receive credentials.
func sameHostPort(u, base *url.URL) bool {
	if !strings.EqualFold(u.Hostname(), base.Hostname()) {
		return false
	}
	return effectivePort(u) == effectivePort(base)
}

// effectivePort returns the URL's explicit port, or the scheme's default
// (80 for http, 443 for https) when omitted.
func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	return "443"
}

// apiSummary generates a summary from an API response.
func apiSummary(data any) string {
	switch v := data.(type) {
	case []map[string]any:
		return fmt.Sprintf("%d items", len(v))
	case []any:
		return fmt.Sprintf("%d items", len(v))
	case map[string]any:
		for _, key := range []string{"title", "name", "subject"} {
			if title, ok := v[key].(string); ok && title != "" {
				if runes := []rune(title); len(runes) > 50 {
					title = string(runes[:47]) + "..."
				}
				return title
			}
		}
	}
	return "API response"
}
