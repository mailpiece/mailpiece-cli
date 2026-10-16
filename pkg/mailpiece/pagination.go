package mailpiece

import (
	"context"
	"strings"
)

// parseNextLink extracts the next URL from a Link header.
func parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	for _, part := range strings.Split(linkHeader, ",") {
		part = strings.TrimSpace(part)
		if strings.Contains(part, `rel="next"`) {
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				return part[start+1 : end]
			}
		}
	}

	return ""
}

// NextPageURL returns the next page URL from a response's Link header, or "".
func (r *Response) NextPageURL() string {
	if r == nil {
		return ""
	}
	return parseNextLink(r.Headers.Get("Link"))
}

// listAt fetches one page of a list: path when given, as a list's own path with
// its page, else the first page, which format and args build.
func listAt[T any](ctx context.Context, ac *AccountClient, path, format string, args ...any) ([]T, *Response, error) {
	if path == "" {
		var err error
		if path, err = buildPath(format, args...); err != nil {
			return nil, nil, err
		}
	}
	return decodeList[T](ac.Get(ctx, path))
}
