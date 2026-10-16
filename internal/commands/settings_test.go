package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestSettingsCommands(t *testing.T) {
	newsletterRun(t, settingsCmd, []newsletterCase{
		{name: "newsletter show", args: []string{"settings", "newsletter", "show"}, method: "GET", path: "/account/newsletter.json"},
		{
			name: "newsletter update", args: []string{"settings", "newsletter", "update", "--name", "Acme News", "--description", "All the *news*", "--locale", "es"},
			method: "PATCH", path: "/account/newsletter.json",
			body: map[string]any{"name": "Acme News", "description": "<p>All the <em>news</em></p>\n", "locale": "es"},
		},
		{
			name: "newsletter description and description-file together are refused",
			args: []string{"settings", "newsletter", "update", "--description", "News", "--description-file", "news.md"}, exit: errors.ExitUsage,
		},
		{name: "newsletter update with nothing is refused", args: []string{"settings", "newsletter", "update"}, exit: errors.ExitUsage},
		{name: "newsletter update refuses a locale the app does not have", args: []string{"settings", "newsletter", "update", "--locale", "fr"}, exit: errors.ExitUsage},

		{name: "handle show", args: []string{"settings", "handle", "show"}, method: "GET", path: "/account/handle.json"},
		{
			name: "handle update", args: []string{"settings", "handle", "update", "--handle", "acme-news"},
			method: "PATCH", path: "/account/handle.json", body: map[string]any{"handle": "acme-news"},
		},
		{
			name: "handle update clears with an empty handle", args: []string{"settings", "handle", "update", "--handle", ""},
			method: "PATCH", path: "/account/handle.json", body: map[string]any{"handle": ""},
		},
		{name: "handle update needs a handle", args: []string{"settings", "handle", "update"}, exit: errors.ExitUsage},

		{name: "sending show", args: []string{"settings", "sending", "show"}, method: "GET", path: "/account/sending.json"},
		{
			name: "sending update sends only what was passed", args: []string{"settings", "sending", "update", "--from-name", "Acme", "--reply-to-address", ""},
			method: "PATCH", path: "/account/sending.json", body: map[string]any{"from_name": "Acme", "reply_to_address": ""},
		},
		{name: "sending update with nothing is refused", args: []string{"settings", "sending", "update"}, exit: errors.ExitUsage},

		{name: "subscribe form show", args: []string{"settings", "subscribe-form", "show"}, method: "GET", path: "/account/subscribe_form.json"},
		{
			name: "subscribe form update", args: []string{"settings", "subscribe-form", "update", "--return-url", "https://acme.example/thanks"},
			method: "PATCH", path: "/account/subscribe_form.json", body: map[string]any{"subscription_return_url": "https://acme.example/thanks"},
		},
		{
			name: "subscribe form update clears with an empty url", args: []string{"settings", "subscribe-form", "update", "--return-url", ""},
			method: "PATCH", path: "/account/subscribe_form.json", body: map[string]any{"subscription_return_url": ""},
		},
		{name: "subscribe form update needs the url", args: []string{"settings", "subscribe-form", "update"}, exit: errors.ExitUsage},

		{name: "join code show", args: []string{"settings", "join-code", "show"}, method: "GET", path: "/account/join_code.json"},
		{
			name: "join code update", args: []string{"settings", "join-code", "update", "--usage-limit", "25"},
			method: "PATCH", path: "/account/join_code.json", body: map[string]any{"usage_limit": float64(25)},
		},
		{name: "join code update needs the limit", args: []string{"settings", "join-code", "update"}, exit: errors.ExitUsage},
		{name: "join code reset", args: []string{"settings", "join-code", "reset"}, method: "DELETE", path: "/account/join_code.json"},

		{name: "notifications show", args: []string{"settings", "notifications", "show"}, method: "GET", path: "/notifications/settings.json"},
		{
			name: "notifications update", args: []string{"settings", "notifications", "update", "--new-subscriber=false"},
			method: "PATCH", path: "/notifications/settings.json", body: map[string]any{"new_subscriber_notifications": false},
		},
		{name: "notifications update needs the flag", args: []string{"settings", "notifications", "update"}, exit: errors.ExitUsage},
	})
}

func TestSettingsShowPrintsThePart(t *testing.T) {
	mock := NewMockClient().WithGetData(map[string]any{"code": "abc", "usage_count": 1, "usage_limit": 10, "active": true})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("settings", "join-code", "show")
	assertExitCode(t, err, 0)
	data := responseDataMap(t, lastResult)
	if data["code"] != "abc" || data["usage_limit"] != float64(10) {
		t.Errorf("expected the join code, got %#v", data)
	}
	if got := registerBreadcrumb(lastResult, "reset"); got != "mailpiece settings join-code reset" {
		t.Errorf("expected a reset breadcrumb, got %q", got)
	}
}
