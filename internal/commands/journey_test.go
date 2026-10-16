package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestJourneyCommands(t *testing.T) {
	const emails = "/journeys/subscriber_confirmed/emails"
	newsletterRun(t, journeyCmd, []newsletterCase{
		{name: "show", args: []string{"journey", "show", "subscriber_confirmed"}, method: "GET", path: "/journeys/subscriber_confirmed.json"},
		{name: "show refuses a trigger the app does not have", args: []string{"journey", "show", "post_published"}, exit: errors.ExitUsage},
		{name: "email show", args: []string{"journey", "email", "show", "subscriber_confirmed", "e1"}, method: "GET", path: emails + "/e1.json"},
		{name: "email show refuses a trigger the app does not have", args: []string{"journey", "email", "show", "post_published", "e1"}, exit: errors.ExitUsage},
		{
			name:   "email create",
			args:   []string{"journey", "email", "create", "subscriber_confirmed", "--subject", "Welcome", "--body", "Glad you are **here**.", "--delay-days", "0", "--send-hour", "9"},
			method: "POST", path: emails + ".json",
			body: map[string]any{"subject": "Welcome", "body": "<p>Glad you are <strong>here</strong>.</p>\n", "delay_days": float64(0), "send_hour": float64(9)},
		},
		{
			name: "email body and body-file together are refused",
			args: []string{"journey", "email", "update", "subscriber_confirmed", "e1", "--body", "Hi", "--body-file", "hi.md"}, exit: errors.ExitUsage,
		},
		{name: "email create needs a subject", args: []string{"journey", "email", "create", "subscriber_confirmed", "--body", "Hi"}, exit: errors.ExitUsage},
		{
			name: "email create refuses a delay the journey would drop",
			args: []string{"journey", "email", "create", "subscriber_unsubscribed", "--subject", "Bye", "--delay-days", "2"}, exit: errors.ExitUsage,
		},
		{
			name: "email update sends only what was passed", args: []string{"journey", "email", "update", "subscriber_confirmed", "e1", "--summary", ""},
			method: "PATCH", path: emails + "/e1.json", body: map[string]any{"summary": ""},
		},
		{name: "email update with nothing is refused", args: []string{"journey", "email", "update", "subscriber_confirmed", "e1"}, exit: errors.ExitUsage},
		{name: "email delete", args: []string{"journey", "email", "delete", "subscriber_confirmed", "e1"}, method: "DELETE", path: emails + "/e1.json"},
		{name: "email activate", args: []string{"journey", "email", "activate", "subscriber_confirmed", "e1"}, method: "POST", path: emails + "/e1/activation.json"},
		{name: "email deactivate", args: []string{"journey", "email", "deactivate", "subscriber_confirmed", "e1"}, method: "DELETE", path: emails + "/e1/activation.json"},
		{name: "email test to oneself", args: []string{"journey", "email", "test", "subscriber_confirmed", "e1"}, method: "POST", path: emails + "/e1/test_delivery.json"},
		{
			name: "email test to an address", args: []string{"journey", "email", "test", "subscriber_confirmed", "e1", "--to", "ana@example.com"},
			method: "POST", path: emails + "/e1/test_delivery.json", body: map[string]any{"email_address": "ana@example.com"},
		},
	})
}

func TestJourneyList(t *testing.T) {
	mock := NewMockClient().WithListData([]any{
		map[string]any{"trigger": "subscriber_confirmed", "emails": []any{}},
		map[string]any{"trigger": "subscriber_unsubscribed", "emails": []any{}},
	})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("journey", "list")
	assertExitCode(t, err, 0)
	if got := mock.GetCalls[0].Path; got != "/journeys.json" {
		t.Errorf("expected GET /journeys.json, got %s", got)
	}
	if lastResult.Response == nil || lastResult.Response.Summary != "2 journeys" {
		t.Fatalf("expected two journeys, got %#v", lastResult.Response)
	}
}

func TestJourneyShowCountsItsEmails(t *testing.T) {
	mock := NewMockClient().WithGetData(map[string]any{
		"trigger": "subscriber_confirmed",
		"emails":  []any{map[string]any{"id": "e1", "subject": "Welcome", "active": true}},
	})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("journey", "show", "subscriber_confirmed")
	assertExitCode(t, err, 0)
	if lastResult.Response == nil || lastResult.Response.Summary != "1 emails" {
		t.Fatalf("expected one email, got %#v", lastResult.Response)
	}
	emails := responseDataMap(t, lastResult)["emails"].([]any)
	if emails[0].(map[string]any)["subject"] != "Welcome" {
		t.Errorf("expected the email in full, got %#v", emails)
	}
}

func TestJourneyEmailDeactivateAnswersNoBody(t *testing.T) {
	mock, result := mockTest(t)
	mock.DeleteResponse = &APIResponse{StatusCode: 204}

	_, err := runCobraWithArgs("journey", "email", "deactivate", "subscriber_confirmed", "e1")
	assertExitCode(t, err, 0)
	if data := responseDataMap(t, result); data["deactivated"] != true || data["id"] != "e1" {
		t.Errorf("expected a deactivation, got %#v", data)
	}
}
