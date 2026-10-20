package commands

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/spf13/cobra"
)

// TestMain keeps every test off the developer's own home, keyring, local
// config and MAILPIECE_* environment.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "mailpiece-commands-test-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, name := range []string{
		"MAILPIECE_TOKEN", "MAILPIECE_ACCOUNT", "MAILPIECE_PROFILE", "MAILPIECE_API_URL",
		"MAILPIECE_DEBUG",
		"CODEX_HOME",
	} {
		os.Unsetenv(name)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	os.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	os.Setenv("XDG_STATE_HOME", filepath.Join(home, "state"))
	os.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	os.Setenv("MAILPIECE_NO_KEYRING", "1")
	for i := range skillLocations {
		if skillLocations[i].Name == "Codex (Global)" {
			skillLocations[i].Path = codexGlobalSkillPath()
		}
	}
	if err := os.Chdir(home); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}

// testHTTPServer holds the current httptest server for cleanup.
var testHTTPServer *httptest.Server

// SetTestModeWithSDK captures output and starts an httptest server that
// delegates to the MockClient, with the SDK pointed at it.
// Call resetTest() (or defer resetTest()) to clean up.
func SetTestModeWithSDK(mock *MockClient) *CommandResult {
	result := SetTestMode()

	// Create httptest server that delegates to the mock
	testHTTPServer = httptest.NewServer(mockHandler(mock))

	// Wire up SDK to point at the httptest server
	SetTestSDK(testHTTPServer.URL)

	// Set context on all commands so cmd.Context() doesn't return nil
	// when tests call RunE directly instead of through cobra Execute
	setContextOnAll(context.Background(), rootCmd)

	return result
}

// mockTest starts a mocked app for one test, signed in to the account acme, and returns
// the mock and the result.
func mockTest(t *testing.T) (*MockClient, *CommandResult) {
	t.Helper()
	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	t.Cleanup(resetTest)
	return mock, result
}

// setContextOnAll recursively sets context on a command and all its children.
func setContextOnAll(ctx context.Context, cmd *cobra.Command) {
	cmd.SetContext(ctx)
	for _, child := range cmd.Commands() {
		setContextOnAll(ctx, child)
	}
}

// resetTest cleans up both the httptest server and test mode state.
func resetTest() {
	if testHTTPServer != nil {
		testHTTPServer.Close()
		testHTTPServer = nil
	}
	ResetTestMode()
}

// runCobraWithArgs exercises full Cobra arg parsing, PersistentPreRunE and
// the command's output. Call SetTestModeWithSDK (or SetTestMode) first.
func runCobraWithArgs(args ...string) (string, error) {
	// Reset format flags to prevent leaking between calls.
	cfgJSON = false
	cfgQuiet = false
	cfgIDsOnly = false
	cfgCount = false
	cfgAgent = false
	cfgStyled = false
	cfgMarkdown = false
	cfgJQ = ""
	testBuf.Reset()
	lastRawOutput = ""
	out = output.New(output.Options{Format: output.FormatJSON, Writer: &testBuf})
	lastResult = &CommandResult{}
	if args == nil {
		args = []string{}
	}
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	// Capture any remaining output not yet captured by captureResponse.
	if lastRawOutput == "" {
		lastRawOutput = testBuf.String()
	}
	return lastRawOutput, err
}

// mockHandler creates an http.Handler that delegates to a MockClient.
func mockHandler(mock *MockClient) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip account prefix for account-scoped SDK paths: /<account>/path -> /path
		path := r.URL.Path
		if parts := strings.Split(strings.TrimPrefix(path, "/"), "/"); len(parts) > 1 {
			first := parts[0]
			if first != "my" && first != "session" {
				path = "/" + strings.Join(parts[1:], "/")
			}
		}

		// Re-append query string
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}

		var resp *APIResponse
		var err error

		switch r.Method {
		case "GET":
			// Record as a GetWithPagination call since many tests assert on that.
			mock.GetWithPaginationCalls = append(mock.GetWithPaginationCalls, MockCall{Path: path})
			// Use mock.Get which checks path-based responses, then falls back to GetResponse.
			resp, err = mock.Get(path)
			// When GetResponse is still the default empty map (test didn't override it),
			// fall back to GetWithPaginationResponse if available, where list data is set.
			if err == nil && len(mock.getPathResponses) == 0 && isDefaultGetResponse(resp) {
				if mock.GetWithPaginationError != nil {
					resp = nil
					err = mock.GetWithPaginationError
				} else if mock.GetWithPaginationResponse != nil {
					resp = mock.GetWithPaginationResponse
				}
			}
		case "POST":
			resp, err = mock.Post(path, readMockBody(mock, r))
		case "PATCH":
			resp, err = mock.Patch(path, readMockBody(mock, r))
		case "PUT":
			resp, err = mock.Put(path, readMockBody(mock, r))
		case "DELETE":
			resp, err = mock.Delete(path)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err != nil {
			statusCode := http.StatusInternalServerError
			errMsg := err.Error()
			// Use HTTPStatus from output.Error if available
			var outputErr *output.Error
			if stderrors.As(err, &outputErr) && outputErr.HTTPStatus > 0 {
				statusCode = outputErr.HTTPStatus
			} else if outputErr != nil && outputErr.Code == output.CodeAuth {
				statusCode = http.StatusUnauthorized
			} else if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "Not found") {
				statusCode = http.StatusNotFound
			} else if strings.Contains(strings.ToLower(errMsg), "unauthorized") {
				statusCode = http.StatusUnauthorized
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			errResp := map[string]string{"error": errMsg}
			_ = json.NewEncoder(w).Encode(errResp)
			return
		}

		if resp == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Set Location header if present
		if resp.Location != "" {
			w.Header().Set("Location", resp.Location)
		}
		// Set Link header if present
		if resp.LinkNext != "" {
			w.Header().Set("Link", `<`+resp.LinkNext+`>; rel="next"`)
		}

		statusCode := resp.StatusCode
		if statusCode == 0 {
			statusCode = 200
		}

		if resp.Body != nil {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.WriteHeader(statusCode)
			_, _ = w.Write(resp.Body)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)

		if statusCode == http.StatusNoContent {
			return
		}
		if resp.Data != nil {
			_ = json.NewEncoder(w).Encode(resp.Data)
		} else {
			_, _ = w.Write([]byte("null"))
		}
	})
}

// readMockBody decodes a JSON request body. A multipart body is recorded as
// {"fields": {name: [values]}, "files": {name: [file names]}}, and each file
// name is added to mock.UploadFileCalls.
func readMockBody(mock *MockClient, r *http.Request) any {
	if r.Body == nil {
		return nil
	}
	data, _ := io.ReadAll(r.Body)
	if len(data) == 0 {
		return nil
	}
	mediaType, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType == "multipart/form-data" {
		form, err := multipart.NewReader(bytes.NewReader(data), params["boundary"]).ReadForm(32 << 20)
		if err != nil {
			return nil
		}
		defer form.RemoveAll()
		fields := map[string]any{}
		for name, values := range form.Value {
			fields[name] = values
		}
		files := map[string]any{}
		for name, headers := range form.File {
			names := make([]string, 0, len(headers))
			for _, h := range headers {
				names = append(names, h.Filename)
				mock.UploadFileCalls = append(mock.UploadFileCalls, h.Filename)
			}
			files[name] = names
		}
		return map[string]any{"fields": fields, "files": files}
	}
	var body any
	_ = json.Unmarshal(data, &body)
	return body
}

// isDefaultGetResponse checks if a response looks like the NewMockClient default
// GetResponse (status 200 with an empty map). Tests that explicitly set GetResponse
// with real data will not match this check.
func isDefaultGetResponse(resp *APIResponse) bool {
	if resp == nil {
		return false
	}
	m, ok := resp.Data.(map[string]any)
	return ok && len(m) == 0
}

// testAPIURL stands in for a Mailpiece install: there is no default address.
const testAPIURL = "https://mailpiece.example.com"
