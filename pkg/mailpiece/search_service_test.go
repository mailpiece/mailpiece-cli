package mailpiece

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestSearchService(t *testing.T) {
	sdkRunCases(t, []sdkCase{
		{name: "search escapes the query", method: "GET", uri: "/acme/search.json?q=ana%40example.com+%26+co",
			reply: `{"posts":[],"subscribers":[]}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSearchService(ac).Search(ctx, "ana@example.com & co")
				return err
			}},
	})
}

func TestSearchReadsSubscribers(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{"posts":[{"id":"p1"}],"subscribers":[{"id":"s1","email":"ana@example.com","status":"confirmed"}]}`))
	results, _, err := NewSearchService(sdkAccount(srv.URL)).Search(context.Background(), "ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Posts) != 1 || results.Posts[0].ID != "p1" {
		t.Errorf("posts = %+v", results.Posts)
	}
	if len(results.Subscribers) != 1 {
		t.Fatalf("subscribers = %d, want 1", len(results.Subscribers))
	}
	if got := results.Subscribers[0]; got.ID != "s1" || got.Email != "ana@example.com" || got.Status != "confirmed" {
		t.Errorf("subscriber = %+v", got)
	}
}

func TestSearchFollowsTheSubscribersPages(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Total-Count", "3")
		if r.URL.Query().Get("page") == "" {
			w.Header().Set("Link", `</acme/search.json?page=2&q=example.org>; rel="next"`)
			sdkReply(http.StatusOK, `{"posts":[{"id":"p1"}],"subscribers":[{"id":"s1"},{"id":"s2"}]}`)(w, r)
			return
		}
		sdkReply(http.StatusOK, `{"posts":[{"id":"p1"}],"subscribers":[{"id":"s3"}]}`)(w, r)
	})

	results, resp, err := NewSearchService(sdkAccount(srv.URL)).Search(context.Background(), "example.org")
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Posts) != 1 {
		t.Errorf("every page repeats the posts: expected 1, got %d", len(results.Posts))
	}
	if len(results.Subscribers) != 3 || results.Subscribers[2].ID != "s3" {
		t.Errorf("subscribers = %+v", results.Subscribers)
	}
	if resp.NextPageURL() != "" {
		t.Errorf("the response is the last page's, got a next link %q", resp.NextPageURL())
	}
	recorded := srv.recorded()
	if len(recorded) != 2 || recorded[0].URI != "/acme/search.json?q=example.org" || recorded[1].URI != "/acme/search.json?page=2&q=example.org" {
		t.Errorf("expected the next link followed, got %+v", recorded)
	}
}

func TestSearchStopsOnAnEmptyPage(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</acme/search.json?page=9&q=ana>; rel="next"`)
		if r.URL.Query().Get("page") == "" {
			sdkReply(http.StatusOK, `{"posts":[{"id":"p1"}],"subscribers":[{"id":"s1"}]}`)(w, r)
			return
		}
		sdkReply(http.StatusOK, `{"posts":[{"id":"p1"}],"subscribers":[]}`)(w, r)
	})

	results, _, err := NewSearchService(sdkAccount(srv.URL)).Search(context.Background(), "ana")
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Subscribers) != 1 || len(srv.recorded()) != 2 {
		t.Errorf("the app links a page past the end: expected 1 subscriber in 2 requests, got %d in %d", len(results.Subscribers), len(srv.recorded()))
	}
}

func TestSearchRefusesACrossOriginNextLink(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<https://evil.example.com/acme/search.json?page=2&q=ana>; rel="next"`)
		sdkReply(http.StatusOK, `{"posts":[],"subscribers":[{"id":"s1"}]}`)(w, r)
	})

	_, _, err := NewSearchService(sdkAccount(srv.URL)).Search(context.Background(), "ana")
	if err == nil || !strings.Contains(err.Error(), "pagination Link header points to different origin") {
		t.Fatalf("expected the cross-origin link refused, got %v", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("expected 1 request, got %d", n)
	}
}
