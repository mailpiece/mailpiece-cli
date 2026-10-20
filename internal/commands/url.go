package commands

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/basecamp/cli/output"
)

// appURL is what a Mailpiece app URL names: the account (empty for a path
// outside any account, such as /my/identity) and the JSON path of the page,
// relative to that account.
type appURL struct {
	Account string
	Path    string
}

// accountSlugPattern is what names an account in a path: its number, which is
// the slug the app gives, or its handle, which also answers but changes.
var accountSlugPattern = regexp.MustCompile(`^([0-9]+|[a-z][a-z0-9-]*)$`)

// unscopedSegments are the first words of the pages outside any account. A
// handle is a word too, and the app never lets an account take one of these.
var unscopedSegments = map[string]bool{"my": true}

// parseAppURL reads a Mailpiece app URL as the browser shows it. Only its path
// and query are kept: the request goes to the configured API host whatever host
// the URL names, as a download does, so the token is sent nowhere else.
func parseAppURL(raw string) (*appURL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (!strings.EqualFold(u.Scheme, "https") && !strings.EqualFold(u.Scheme, "http")) {
		return nil, output.ErrUsageHint(
			fmt.Sprintf("Not a Mailpiece URL: %s", raw),
			"Expected a URL like: https://mailpiece.example.com/834206594/posts/<id>",
		)
	}

	segments := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	parsed := &appURL{}
	if accountSlugPattern.MatchString(segments[0]) && !unscopedSegments[segments[0]] {
		parsed.Account = segments[0]
		segments = segments[1:]
	}
	if len(segments) == 0 || segments[0] == "" {
		return nil, output.ErrUsageHint(
			fmt.Sprintf("This URL names no record: %s", raw),
			"Paste the URL of a record's page, e.g. https://mailpiece.example.com/834206594/posts/<id>",
		)
	}

	path := "/" + strings.Join(segments, "/")
	if !strings.HasSuffix(path, ".json") {
		path += ".json"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	parsed.Path = path
	return parsed, nil
}
