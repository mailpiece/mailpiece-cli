package mailpiece

import (
	"context"
	"net/http"
	"testing"
)

func TestPostsService(t *testing.T) {
	hour := 9
	sdkRunCases(t, []sdkCase{
		{name: "list", method: "GET", uri: "/acme/posts.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).List(ctx)
				return err
			}},
		{name: "published", method: "GET", uri: "/acme/posts/published.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Published(ctx, "")
				return err
			}},
		{name: "sent, another page", method: "GET", uri: "/acme/posts/sent.json?page=2", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Sent(ctx, "/posts/sent.json?page=2")
				return err
			}},
		{name: "get", method: "GET", uri: "/acme/posts/p1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Get(ctx, "p1")
				return err
			}},
		{name: "create", method: "POST", uri: "/acme/posts.json", body: `{"title":"Hello","body":"<p>Hi</p>"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Create(ctx, &CreatePostRequest{Title: "Hello", Body: "<p>Hi</p>"})
				return err
			}},
		{name: "update sends only what is set, an emptied field included", method: "PATCH", uri: "/acme/posts/p1.json",
			body: `{"summary":"","visibility":"unlisted"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Update(ctx, "p1", &UpdatePostRequest{Summary: sdkString(""), Visibility: sdkString("unlisted")})
				return err
			}},
		{name: "delete", method: "DELETE", uri: "/acme/posts/p1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).Delete(ctx, "p1")
				return err
			}},
		{name: "publish as it is", method: "POST", uri: "/acme/posts/p1/publication.json", body: `{}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Publish(ctx, "p1", nil)
				return err
			}},
		{name: "publish under a past date", method: "POST", uri: "/acme/posts/p1/publication.json",
			body: `{"destination":"link","day":"2024-03-01","hour":9}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Publish(ctx, "p1", &PublishPostRequest{Destination: "link", Day: "2024-03-01", Hour: &hour})
				return err
			}},
		{name: "unpublish", method: "DELETE", uri: "/acme/posts/p1/publication.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).Unpublish(ctx, "p1")
				return err
			}},
		{name: "schedule with a delivery", method: "POST", uri: "/acme/posts/p1/schedule.json",
			body: `{"scheduled_posting_at":"2030-01-01T09:00:00Z","scheduled_delivery":true}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Schedule(ctx, "p1", &SchedulePostRequest{ScheduledPostingAt: "2030-01-01T09:00:00Z", ScheduledDelivery: true})
				return err
			}},
		{name: "unschedule", method: "DELETE", uri: "/acme/posts/p1/schedule.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).Unschedule(ctx, "p1")
				return err
			}},
		{name: "deliver", method: "POST", uri: "/acme/posts/p1/delivery.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Deliver(ctx, "p1")
				return err
			}},
		{name: "test to yourself", method: "POST", uri: "/acme/posts/p1/test_delivery.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).DeliverTest(ctx, "p1", nil)
				return err
			}},
		{name: "test to an address", method: "POST", uri: "/acme/posts/p1/test_delivery.json", body: `{"email_address":"ana@example.com"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).DeliverTest(ctx, "p1", &TestPostDeliveryRequest{EmailAddress: "ana@example.com"})
				return err
			}},
		{name: "relist", method: "POST", uri: "/acme/posts/p1/listing.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).Relist(ctx, "p1")
				return err
			}},
		{name: "unlist", method: "DELETE", uri: "/acme/posts/p1/listing.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewPostsService(ac).Unlist(ctx, "p1")
				return err
			}},
		{name: "links", method: "GET", uri: "/acme/posts/p1/links.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Links(ctx, "p1")
				return err
			}},
		{name: "update link", method: "PATCH", uri: "/acme/posts/p1/links/l1.json", body: `{"url":"https://example.com/new"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).UpdateLink(ctx, "p1", "l1", &UpdatePostLinkRequest{URL: "https://example.com/new"})
				return err
			}},
		{name: "revisions", method: "GET", uri: "/acme/posts/p1/history.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Revisions(ctx, "p1")
				return err
			}},
		{name: "latest revision", method: "GET", uri: "/acme/posts/p1/history/latest.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Revision(ctx, "p1", "latest")
				return err
			}},
	})

	sdkRunRefusals(t, []sdkRefusal{
		{name: "an empty post", want: "A post needs a title, a summary or a body",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Create(ctx, &CreatePostRequest{Visibility: "listed"})
				return err
			}},
		{name: "a visibility the app does not take", want: "visibility must be one of",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Create(ctx, &CreatePostRequest{Title: "Hello", Visibility: "public"})
				return err
			}},
		{name: "an update of nothing", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Update(ctx, "p1", &UpdatePostRequest{})
				return err
			}},
		{name: "a destination the app does not take", want: "destination must be one of",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Publish(ctx, "p1", &PublishPostRequest{Destination: "everywhere"})
				return err
			}},
		{name: "a date given twice", want: "not both",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Publish(ctx, "p1", &PublishPostRequest{PublishedAt: "2024-03-01T09:00:00Z", Day: "2024-03-01"})
				return err
			}},
		{name: "an hour without a day", want: "An hour needs a day",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Publish(ctx, "p1", &PublishPostRequest{Hour: &hour})
				return err
			}},
		{name: "a schedule without a time", want: "A schedule needs a time",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).Schedule(ctx, "p1", &SchedulePostRequest{ScheduledDelivery: true})
				return err
			}},
		{name: "an hour past the day", want: "hour must be from 0 to 23",
			call: func(ctx context.Context, ac *AccountClient) error {
				late := 24
				_, _, err := NewPostsService(ac).Schedule(ctx, "p1", &SchedulePostRequest{Day: "2030-01-01", Hour: &late})
				return err
			}},
		{name: "a link without a url", want: "A link needs a url",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewPostsService(ac).UpdateLink(ctx, "p1", "l1", &UpdatePostLinkRequest{})
				return err
			}},
	})
}

func TestPostDecodesWhatTheAppRenders(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{
		"id": "p1", "title": null, "summary": null, "slug": "p1", "deliveries_count": 3,
		"body": "Hi", "body_html": "<aside data-audience=\"web\"><p>Hi</p></aside>",
		"published": true, "visibility": "listed",
		"published_at": "2026-01-02T03:04:05.000Z", "sent_at": null,
		"scheduled_posting_at": null, "scheduled_delivery": false,
		"public_url": "https://news.example.com/acme/hi-0a1b2c3d",
		"creator": {"id": "u1", "name": "Ana", "role": "owner"}
	}`))
	post, _, err := NewPostsService(sdkAccount(srv.URL)).Get(context.Background(), "p1")
	if err != nil {
		t.Fatal(err)
	}
	if post.Title != nil || post.SentAt != nil {
		t.Errorf("a null title and sent_at should stay nil, got %v and %v", post.Title, post.SentAt)
	}
	if post.PublishedAt == nil || *post.PublishedAt != "2026-01-02T03:04:05.000Z" {
		t.Errorf("published_at = %v", post.PublishedAt)
	}
	if post.BodyHTML != `<aside data-audience="web"><p>Hi</p></aside>` {
		t.Errorf("body_html = %q", post.BodyHTML)
	}
	if post.DeliveriesCount != 3 || post.Creator.Name != "Ana" || post.PublicURL == "" {
		t.Errorf("post = %+v", post)
	}
}

// A delivery mails people, so a failure the app may have answered after mailing is
// never sent again by the client.
func TestDeliverIsNotRetried(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusInternalServerError, `{}`))
	if _, _, err := NewPostsService(sdkAccount(srv.URL)).Deliver(context.Background(), "p1"); err == nil {
		t.Fatal("expected an error")
	}
	if got := len(srv.recorded()); got != 1 {
		t.Errorf("sent %d requests, want 1", got)
	}
}
