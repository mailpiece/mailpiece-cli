package mailpiece

import (
	"context"
	"fmt"
	"net/url"
)

// SearchService handles the newsletter's search. Admins only.
type SearchService struct{ client *AccountClient }

// NewSearchService builds a SearchService bound to an account client.
func NewSearchService(c *AccountClient) *SearchService {
	return &SearchService{client: c}
}

// Search returns the posts and the subscribers q matches. The app pages the
// subscribers and repeats the posts on every page: the posts are the first
// page's and the subscribers every page's, the next links followed as GetAll
// follows them. The response is the last page's.
func (s *SearchService) Search(ctx context.Context, q string) (*SearchResults, *Response, error) {
	if q == "" {
		return nil, nil, ErrUsage("A search needs a query")
	}

	c := s.client.parent
	path := s.client.accountPath("/search.json?q=" + url.QueryEscape(q))
	baseURL, err := c.buildURL(path)
	if err != nil {
		return nil, nil, err
	}
	ctx = markCallerURL(ctx, path)
	pageURL := baseURL

	var results *SearchResults
	var resp *Response
	var page int

	for page = 1; page <= c.httpOpts.MaxPages; page++ {
		var found *SearchResults
		found, resp, err = decode[SearchResults](c.doRequestURL(ctx, "GET", pageURL, nil))
		if err != nil {
			return nil, resp, err
		}
		if results == nil {
			results = found
		} else {
			results.Subscribers = append(results.Subscribers, found.Subscribers...)
		}
		// The app links a next page past the end of a list.
		if len(found.Subscribers) == 0 {
			break
		}

		nextURL := resp.NextPageURL()
		if nextURL == "" {
			break
		}
		nextURL = resolveURL(pageURL, nextURL)
		if !isSameOrigin(nextURL, baseURL) {
			return nil, resp, fmt.Errorf("pagination Link header points to different origin: %s", nextURL)
		}
		pageURL = nextURL
	}

	if page > c.httpOpts.MaxPages {
		c.logger.Warn("pagination capped", "maxPages", c.httpOpts.MaxPages)
	}

	return results, resp, nil
}
