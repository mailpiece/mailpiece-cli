package mailpiece

import (
	"context"
	"testing"
)

func TestSubscribersService(t *testing.T) {
	sdkRunCases(t, []sdkCase{
		{name: "list the ones mailed", method: "GET", uri: "/acme/subscribers.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).List(ctx, "", "")
				return err
			}},
		{name: "list confirmed is the same list", method: "GET", uri: "/acme/subscribers.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).List(ctx, "confirmed", "")
				return err
			}},
		{name: "list a state", method: "GET", uri: "/acme/subscribers/bounced.json", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).List(ctx, "bounced", "")
				return err
			}},
		{name: "list a page", method: "GET", uri: "/acme/subscribers/unsubscribed.json?page=2", reply: "[]",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).List(ctx, "unsubscribed", "/subscribers/unsubscribed.json?page=2")
				return err
			}},
		{name: "get", method: "GET", uri: "/acme/subscribers/s1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).Get(ctx, "s1", 0)
				return err
			}},
		{name: "get a page of deliveries", method: "GET", uri: "/acme/subscribers/s1.json?page=3",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).Get(ctx, "s1", 3)
				return err
			}},
		{name: "create", method: "POST", uri: "/acme/subscribers.json", body: `{"email":"ana@example.com"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).Create(ctx, &CreateSubscriberRequest{Email: "ana@example.com"})
				return err
			}},
		{name: "delete", method: "DELETE", uri: "/acme/subscribers/s1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSubscribersService(ac).Delete(ctx, "s1")
				return err
			}},
		{name: "deactivate", method: "DELETE", uri: "/acme/subscribers/s1/activation.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSubscribersService(ac).Deactivate(ctx, "s1")
				return err
			}},
		{name: "activate", method: "POST", uri: "/acme/subscribers/s1/activation.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSubscribersService(ac).Activate(ctx, "s1")
				return err
			}},
		{name: "bounce", method: "POST", uri: "/acme/subscribers/s1/bounce.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSubscribersService(ac).Bounce(ctx, "s1")
				return err
			}},
		{name: "bounce an address", method: "POST", uri: "/acme/subscribers/bounces.json", body: `{"email_address":"ana@example.com"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSubscribersService(ac).BounceAddress(ctx, &BounceAddressRequest{EmailAddress: "ana@example.com"})
				return err
			}},
	})
}

func TestSubscriberDetailDecodes(t *testing.T) {
	reply := `{"id":"s1","email":"ana@example.com","status":"bounced","confirmed_at":"2026-01-02T03:04:05Z","unsubscribed_at":null,` +
		`"bounced_at":"2026-02-02T03:04:05Z","deactivated_at":null,"created_at":"2026-01-01T00:00:00Z","url":"http://x/acme/subscribers/s1.json",` +
		`"deliveries":[{"id":"d1","post":{"id":"p1","title":"Hello","url":"http://x/acme/posts/p1.json"},"status":"refused","sent_at":null,"refused_at":"2026-02-02T03:04:05Z","created_at":"2026-02-01T03:04:05Z"}]}`
	srv := newSDKServer(t, sdkReply(200, reply))

	got, _, err := NewSubscribersService(sdkAccount(srv.URL)).Get(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Email != "ana@example.com" || got.Status != "bounced" || got.BouncedAt == nil || got.UnsubscribedAt != nil {
		t.Errorf("subscriber = %+v", got.Subscriber)
	}
	if len(got.Deliveries) != 1 || got.Deliveries[0].Post.Title != "Hello" || got.Deliveries[0].Status != "refused" || got.Deliveries[0].SentAt != nil {
		t.Errorf("deliveries = %+v", got.Deliveries)
	}
	if got.Deliveries[0].ID != "d1" || got.Deliveries[0].CreatedAt != "2026-02-01T03:04:05Z" {
		t.Errorf("delivery = %+v, want its id and created_at", got.Deliveries[0])
	}
}
