package mailpiece

import (
	"context"
	"net/http"
	"testing"
)

func TestNewsletterLaneRequests(t *testing.T) {
	sdkRunCases(t, []sdkCase{
		{
			name: "snippets list", method: "GET", uri: "/acme/snippets.json", reply: `[{"id":"s1","name":"Signature"}]`,
			call: func(ctx context.Context, ac *AccountClient) error {
				snippets, _, err := NewSnippetsService(ac).List(ctx)
				if err == nil && (len(snippets) != 1 || snippets[0].Name != "Signature") {
					t.Errorf("got %+v", snippets)
				}
				return err
			},
		},
		{
			name: "snippet get", method: "GET", uri: "/acme/snippets/s%201.json", reply: `{"id":"s1","content":"Bye","content_html":"<p>Bye</p>"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				snippet, _, err := NewSnippetsService(ac).Get(ctx, "s 1")
				if err == nil && snippet.ContentHTML != "<p>Bye</p>" {
					t.Errorf("got %+v", snippet)
				}
				return err
			},
		},
		{
			name: "snippet create", method: "POST", uri: "/acme/snippets.json", body: `{"name":"Signature","content":"<p>Bye</p>"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSnippetsService(ac).Create(ctx, &SnippetRequest{Name: sdkString("Signature"), Content: sdkString("<p>Bye</p>")})
				return err
			},
		},
		{
			name: "snippet update sends only what is set", method: "PATCH", uri: "/acme/snippets/s1.json", body: `{"content":""}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSnippetsService(ac).Update(ctx, "s1", &SnippetRequest{Content: sdkString("")})
				return err
			},
		},
		{
			name: "snippet delete", method: "DELETE", uri: "/acme/snippets/s1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSnippetsService(ac).Delete(ctx, "s1")
				return err
			},
		},
		{
			name: "journeys list", method: "GET", uri: "/acme/journeys.json", reply: `[{"trigger":"subscriber_confirmed","emails":[]}]`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).List(ctx)
				return err
			},
		},
		{
			name: "journey get", method: "GET", uri: "/acme/journeys/subscriber_confirmed.json",
			reply: `{"trigger":"subscriber_confirmed","emails":[{"id":"e1","subject":null,"summary":null,"body":"","body_html":"","delay_days":2,"send_hour":null,"time_zone":null,"active":false}]}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				journey, _, err := NewJourneysService(ac).Get(ctx, "subscriber_confirmed")
				if err == nil && (len(journey.Emails) != 1 || journey.Emails[0].DelayDays != 2 || journey.Emails[0].Subject != nil) {
					t.Errorf("got %+v", journey)
				}
				return err
			},
		},
		{
			name: "journey email create", method: "POST", uri: "/acme/journeys/subscriber_confirmed/emails.json",
			body: `{"subject":"Welcome","body":"<p>Hi</p>","delay_days":0,"send_hour":9}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).CreateEmail(ctx, "subscriber_confirmed", &JourneyEmailRequest{
					Subject: sdkString("Welcome"), Body: sdkString("<p>Hi</p>"), DelayDays: sdkInt(0), SendHour: sdkInt(9),
				})
				return err
			},
		},
		{
			name: "journey email update", method: "PATCH", uri: "/acme/journeys/subscriber_unsubscribed/emails/e1.json", body: `{"summary":""}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).UpdateEmail(ctx, "subscriber_unsubscribed", "e1", &JourneyEmailRequest{Summary: sdkString("")})
				return err
			},
		},
		{
			name: "journey email get", method: "GET", uri: "/acme/journeys/subscriber_confirmed/emails/e1.json", reply: `{"id":"e1","subject":"Welcome","delay_days":2}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				email, _, err := NewJourneysService(ac).GetEmail(ctx, "subscriber_confirmed", "e1")
				if err == nil && (email.ID != "e1" || email.Subject == nil || email.DelayDays != 2) {
					t.Errorf("got %+v", email)
				}
				return err
			},
		},
		{
			name: "journey email delete", method: "DELETE", uri: "/acme/journeys/subscriber_confirmed/emails/e1.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewJourneysService(ac).DeleteEmail(ctx, "subscriber_confirmed", "e1")
				return err
			},
		},
		{
			name: "journey email activate", method: "POST", uri: "/acme/journeys/subscriber_confirmed/emails/e1/activation.json", reply: `{"id":"e1","active":true}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				email, _, err := NewJourneysService(ac).ActivateEmail(ctx, "subscriber_confirmed", "e1")
				if err == nil && !email.Active {
					t.Errorf("got %+v", email)
				}
				return err
			},
		},
		{
			name: "journey email deactivate", method: "DELETE", uri: "/acme/journeys/subscriber_confirmed/emails/e1/activation.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewJourneysService(ac).DeactivateEmail(ctx, "subscriber_confirmed", "e1")
				return err
			},
		},
		{
			name: "journey email test to oneself sends no body", method: "POST", uri: "/acme/journeys/subscriber_confirmed/emails/e1/test_delivery.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewJourneysService(ac).TestEmail(ctx, "subscriber_confirmed", "e1", nil)
				return err
			},
		},
		{
			name: "journey email test to an address", method: "POST", uri: "/acme/journeys/subscriber_confirmed/emails/e1/test_delivery.json",
			body: `{"email_address":"ana@example.com"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewJourneysService(ac).TestEmail(ctx, "subscriber_confirmed", "e1", &JourneyEmailTestRequest{EmailAddress: "ana@example.com"})
				return err
			},
		},
		{
			name: "newsletter get", method: "GET", uri: "/acme/account/newsletter.json", reply: `{"name":"Acme","description":"News","description_html":"<p>News</p>","locale":"en"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				newsletter, _, err := NewSettingsService(ac).Newsletter(ctx)
				if err == nil && (newsletter.Locale != "en" || newsletter.AvatarURL != "") {
					t.Errorf("got %+v", newsletter)
				}
				return err
			},
		},
		{
			name: "newsletter update", method: "PATCH", uri: "/acme/account/newsletter.json", body: `{"locale":"es"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateNewsletter(ctx, &NewsletterRequest{Locale: sdkString("es")})
				return err
			},
		},
		{
			name: "handle get", method: "GET", uri: "/acme/account/handle.json", reply: `{"handle":null}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				handle, _, err := NewSettingsService(ac).Handle(ctx)
				if err == nil && handle.Handle != nil {
					t.Errorf("got %+v", handle)
				}
				return err
			},
		},
		{
			name: "handle update", method: "PATCH", uri: "/acme/account/handle.json", body: `{"handle":"acme-news"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateHandle(ctx, &HandleRequest{Handle: "acme-news"})
				return err
			},
		},
		{
			name: "handle update clears with an empty handle", method: "PATCH", uri: "/acme/account/handle.json", body: `{"handle":""}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateHandle(ctx, &HandleRequest{})
				return err
			},
		},
		{
			name: "sender get", method: "GET", uri: "/acme/account/sending.json", reply: `{"from_name":null,"from_address":null,"reply_to_address":null}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).Sender(ctx)
				return err
			},
		},
		{
			name: "sender update", method: "PATCH", uri: "/acme/account/sending.json", body: `{"from_name":"Acme","reply_to_address":""}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateSender(ctx, &SenderRequest{FromName: sdkString("Acme"), ReplyToAddress: sdkString("")})
				return err
			},
		},
		{
			name: "subscribe form get", method: "GET", uri: "/acme/account/subscribe_form.json", reply: `{"subscription_return_url":null}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).SubscribeForm(ctx)
				return err
			},
		},
		{
			name: "subscribe form update clears with an empty url", method: "PATCH", uri: "/acme/account/subscribe_form.json", body: `{"subscription_return_url":""}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateSubscribeForm(ctx, &SubscribeFormRequest{})
				return err
			},
		},
		{
			name: "join code get", method: "GET", uri: "/acme/account/join_code.json", reply: `{"code":"abc","usage_count":1,"usage_limit":10,"url":"https://app.example/acme/join/abc","active":true}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				code, _, err := NewSettingsService(ac).JoinCode(ctx)
				if err == nil && (code.UsageLimit != 10 || !code.Active) {
					t.Errorf("got %+v", code)
				}
				return err
			},
		},
		{
			name: "join code update", method: "PATCH", uri: "/acme/account/join_code.json", body: `{"usage_limit":25}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSettingsService(ac).UpdateJoinCode(ctx, &JoinCodeRequest{UsageLimit: 25})
				return err
			},
		},
		{
			name: "join code reset", method: "DELETE", uri: "/acme/account/join_code.json",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSettingsService(ac).ResetJoinCode(ctx)
				return err
			},
		},
		{
			name: "notifications get", method: "GET", uri: "/acme/notifications/settings.json", reply: `{"new_subscriber_notifications":true}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				notifications, _, err := NewSettingsService(ac).Notifications(ctx)
				if err == nil && !notifications.NewSubscriberNotifications {
					t.Errorf("got %+v", notifications)
				}
				return err
			},
		},
		{
			name: "notifications update", method: "PATCH", uri: "/acme/notifications/settings.json", body: `{"new_subscriber_notifications":false}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewSettingsService(ac).UpdateNotifications(ctx, &NotificationsRequest{})
				return err
			},
		},
		{
			name: "user get", method: "GET", uri: "/acme/users/u1.json", reply: `{"id":"u1","name":"Ana","role":"admin","active":true}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				user, _, err := NewUsersService(ac).Get(ctx, "u1")
				if err == nil && user.Role != "admin" {
					t.Errorf("got %+v", user)
				}
				return err
			},
		},
		{
			name: "user update", method: "PATCH", uri: "/acme/users/u1.json", body: `{"name":"Ana Ruiz"}`,
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewUsersService(ac).Update(ctx, "u1", &UserRequest{Name: "Ana Ruiz"})
				return err
			},
		},
	})
}

func TestNewsletterLaneRefusals(t *testing.T) {
	sdkRunRefusals(t, []sdkRefusal{
		{
			name: "a snippet with no name", want: "needs a name",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSnippetsService(ac).Create(ctx, &SnippetRequest{Content: sdkString("x")})
				return err
			},
		},
		{
			name: "a snippet update with nothing", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSnippetsService(ac).Update(ctx, "s1", &SnippetRequest{})
				return err
			},
		},
		{
			name: "a trigger the app does not have", want: "is not a journey",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).Get(ctx, "post_published")
				return err
			},
		},
		{
			name: "an email of a trigger the app does not have", want: "is not a journey",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).GetEmail(ctx, "post_published", "e1")
				return err
			},
		},
		{
			name: "an email with no field", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).CreateEmail(ctx, "subscriber_confirmed", &JourneyEmailRequest{})
				return err
			},
		},
		{
			name: "a delay on the journey that takes none", want: "takes no delay_days",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewJourneysService(ac).UpdateEmail(ctx, "subscriber_unsubscribed", "e1", &JourneyEmailRequest{DelayDays: sdkInt(1)})
				return err
			},
		},
		{
			name: "a test on a trigger the app does not have", want: "is not a journey",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, err := NewJourneysService(ac).TestEmail(ctx, "nope", "e1", nil)
				return err
			},
		},
		{
			name: "a newsletter update with nothing", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateNewsletter(ctx, &NewsletterRequest{})
				return err
			},
		},
		{
			name: "a locale the app does not have", want: "locale must be one of",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateNewsletter(ctx, &NewsletterRequest{Locale: sdkString("fr")})
				return err
			},
		},
		{
			name: "a sender update with nothing", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewSettingsService(ac).UpdateSender(ctx, &SenderRequest{})
				return err
			},
		},
		{
			name: "a user update with no name", want: "Nothing to change",
			call: func(ctx context.Context, ac *AccountClient) error {
				_, _, err := NewUsersService(ac).Update(ctx, "u1", &UserRequest{})
				return err
			},
		},
	})
}

func TestJourneyEmailTestAnswersNoBody(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusCreated) })
	if _, err := NewJourneysService(sdkAccount(srv.URL)).TestEmail(context.Background(), "subscriber_confirmed", "e1", nil); err != nil {
		t.Fatalf("TestEmail: %v", err)
	}
}
