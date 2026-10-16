package commands

import (
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestSubscriberList(t *testing.T) {
	t.Run("lists the confirmed ones", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.WithListData([]any{
			map[string]any{"id": "s1", "email": "ana@example.com", "status": "confirmed"},
			map[string]any{"id": "s2", "email": "bea@example.com", "status": "confirmed"},
		})

		err := subscriberListCmd.RunE(subscriberListCmd, []string{})

		assertExitCode(t, err, 0)
		if got := mock.GetWithPaginationCalls[0].Path; got != "/subscribers.json" {
			t.Errorf("expected path '/subscribers.json', got '%s'", got)
		}
		if result.Response.Summary != "2 subscribers" {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
	})

	t.Run("lists a state on its own list, page and all", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			LinkNext:   "/acme/subscribers/bounced.json?page=3",
			Data:       []any{map[string]any{"id": "s1", "email": "ana@example.com", "status": "bounced"}},
		}

		subscriberListStatus = "bounced"
		subscriberListPages.page = 2
		err := subscriberListCmd.RunE(subscriberListCmd, []string{})
		subscriberListStatus = ""
		subscriberListPages.page = 0

		assertExitCode(t, err, 0)
		if got := mock.GetWithPaginationCalls[0].Path; got != "/subscribers/bounced.json?page=2" {
			t.Errorf("expected the bounced list's second page, got '%s'", got)
		}
		if result.Response.Summary != "1 bounced subscribers (page 2)" {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "next"); got != "mailpiece subscriber list --status bounced --page 3" {
			t.Errorf("expected next breadcrumb to keep the state, got %q", got)
		}
	})

	t.Run("refuses a state the app has no list for", func(t *testing.T) {
		mock, _ := mockTest(t)

		subscriberListStatus = "active"
		err := subscriberListCmd.RunE(subscriberListCmd, []string{})
		subscriberListStatus = ""

		assertExitCode(t, err, errors.ExitInvalidArgs)
		if len(mock.GetWithPaginationCalls) != 0 {
			t.Errorf("expected no request, got %d", len(mock.GetWithPaginationCalls))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		SetTestModeWithSDK(NewMockClient())
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := subscriberListCmd.RunE(subscriberListCmd, []string{})
		assertExitCode(t, err, errors.ExitAuthFailure)
	})
}

func TestSubscriberShow(t *testing.T) {
	detail := func(deliveries ...any) map[string]any {
		return map[string]any{"id": "s1", "email": "ana@example.com", "status": "confirmed", "deliveries": deliveries}
	}
	delivery := map[string]any{"id": "d1", "post": map[string]any{"id": "p1", "title": "Hello"}, "status": "sent", "created_at": "2026-01-02T03:04:05Z"}

	t.Run("shows the subscriber with its deliveries", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.GetResponse = &APIResponse{StatusCode: 200, LinkNext: "/acme/subscribers/s1.json?page=2", Data: detail(delivery)}

		err := subscriberShowCmd.RunE(subscriberShowCmd, []string{"s1"})

		assertExitCode(t, err, 0)
		if got := mock.GetCalls[0].Path; got != "/subscribers/s1.json" {
			t.Errorf("expected path '/subscribers/s1.json', got '%s'", got)
		}
		data := responseDataMap(t, result)
		deliveries, ok := data["deliveries"].([]any)
		if !ok || len(deliveries) != 1 {
			t.Fatalf("expected one delivery, got %#v", data["deliveries"])
		}
		if row := deliveries[0].(map[string]any); row["id"] != "d1" || row["created_at"] != "2026-01-02T03:04:05Z" {
			t.Errorf("expected the delivery's id and created_at, got %#v", row)
		}
		if got := registerBreadcrumb(result, "next"); got != "mailpiece subscriber show s1 --page 2" {
			t.Errorf("expected next breadcrumb, got %q", got)
		}
		if got := registerBreadcrumb(result, "deactivate"); got != "mailpiece subscriber deactivate s1" {
			t.Errorf("expected deactivate breadcrumb, got %q", got)
		}
	})

	t.Run("asks for a page, and an empty one has no next", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.GetResponse = &APIResponse{StatusCode: 200, LinkNext: "/acme/subscribers/s1.json?page=4", Data: detail()}

		subscriberShowPage = 3
		err := subscriberShowCmd.RunE(subscriberShowCmd, []string{"s1"})
		subscriberShowPage = 0

		assertExitCode(t, err, 0)
		if got := mock.GetCalls[0].Path; got != "/subscribers/s1.json?page=3" {
			t.Errorf("expected page 3, got '%s'", got)
		}
		if got := registerBreadcrumb(result, "next"); got != "" {
			t.Errorf("expected no next breadcrumb past the end, got %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithNotFoundError()

		err := subscriberShowCmd.RunE(subscriberShowCmd, []string{"nope"})
		assertExitCode(t, err, errors.ExitNotFound)
	})
}

func TestSubscriberCreate(t *testing.T) {
	t.Run("adds an address", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"id": "s1", "email": "ana@example.com", "status": "confirmed"}}

		subscriberCreateEmail = "ana@example.com"
		err := subscriberCreateCmd.RunE(subscriberCreateCmd, []string{})
		subscriberCreateEmail = ""

		assertExitCode(t, err, 0)
		if got := mock.PostCalls[0].Path; got != "/subscribers.json" {
			t.Errorf("expected path '/subscribers.json', got '%s'", got)
		}
		if body := mock.PostCalls[0].Body.(map[string]any); len(body) != 1 || body["email"] != "ana@example.com" {
			t.Errorf("expected a flat email, got %#v", body)
		}
		if data := responseDataMap(t, result); data["status"] != "confirmed" {
			t.Errorf("expected the subscriber, got %#v", data)
		}
		if got := registerBreadcrumb(result, "show"); got != "mailpiece subscriber show s1" {
			t.Errorf("expected show breadcrumb, got %q", got)
		}
	})

	t.Run("needs an address", func(t *testing.T) {
		mock, _ := mockTest(t)

		err := subscriberCreateCmd.RunE(subscriberCreateCmd, []string{})

		assertExitCode(t, err, errors.ExitInvalidArgs)
		if len(mock.PostCalls) != 0 {
			t.Errorf("expected no request, got %d", len(mock.PostCalls))
		}
	})

	for _, message := range []string{
		"isn't an email address",
		"unsubscribed, and only they can subscribe again",
		"bounced, and only they can subscribe again",
		"was stopped by an admin, and is resumed from its own page",
	} {
		t.Run("a refused address is an error that says it "+message, func(t *testing.T) {
			mock, _ := mockTest(t)
			mock.PostResponse = &APIResponse{StatusCode: 422, Data: map[string]any{"email": []any{message}}}

			subscriberCreateEmail = "gone@example.com"
			err := subscriberCreateCmd.RunE(subscriberCreateCmd, []string{})
			subscriberCreateEmail = ""

			assertExitCode(t, err, errors.ExitAPI)
			if err == nil || !strings.Contains(err.Error(), "email: "+message) {
				t.Errorf("error = %v, want %q as the app wrote it", err, message)
			}
		})
	}
}

func TestSubscriberDelete(t *testing.T) {
	mock, result := mockTest(t)

	err := subscriberDeleteCmd.RunE(subscriberDeleteCmd, []string{"s1"})

	assertExitCode(t, err, 0)
	if got := mock.DeleteCalls[0].Path; got != "/subscribers/s1.json" {
		t.Errorf("expected path '/subscribers/s1.json', got '%s'", got)
	}
	if data := responseDataMap(t, result); data["deleted"] != true || data["id"] != "s1" {
		t.Errorf("expected a deletion, got %#v", data)
	}
}

func TestSubscriberActivation(t *testing.T) {
	t.Run("deactivate stops it and offers to resume", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.DeleteResponse = &APIResponse{StatusCode: 200, Data: map[string]any{"id": "s1", "status": "deactivated"}}

		err := subscriberDeactivateCmd.RunE(subscriberDeactivateCmd, []string{"s1"})

		assertExitCode(t, err, 0)
		if got := mock.DeleteCalls[0].Path; got != "/subscribers/s1/activation.json" {
			t.Errorf("expected the activation's path, got '%s'", got)
		}
		if data := responseDataMap(t, result); data["status"] != "deactivated" {
			t.Errorf("expected the subscriber, got %#v", data)
		}
		if got := registerBreadcrumb(result, "activate"); got != "mailpiece subscriber activate s1" {
			t.Errorf("expected activate breadcrumb, got %q", got)
		}
	})

	t.Run("deactivate is refused with the app's reason", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.DeleteError = errors.NewForbiddenError("Sending can't be stopped for an address that unsubscribed or bounced.")

		err := subscriberDeactivateCmd.RunE(subscriberDeactivateCmd, []string{"s1"})

		assertExitCode(t, err, errors.ExitForbidden)
		if err == nil || !strings.Contains(err.Error(), "Sending can't be stopped for an address that unsubscribed or bounced.") {
			t.Errorf("error = %v, want the app's reason", err)
		}
	})

	t.Run("activate says it resumed the one it resumed", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"id": "s1", "status": "confirmed"}}

		assertExitCode(t, subscriberActivateCmd.RunE(subscriberActivateCmd, []string{"s1"}), 0)

		if result.Response.Summary != "Resumed; it is confirmed again" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "deactivate"); got != "mailpiece subscriber deactivate s1" {
			t.Errorf("expected deactivate breadcrumb, got %q", got)
		}
	})

	t.Run("activate changes nothing for one that was not stopped, and a bounce stays a bounce", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 200, Data: map[string]any{"id": "s1", "status": "bounced"}}

		err := subscriberActivateCmd.RunE(subscriberActivateCmd, []string{"s1"})

		assertExitCode(t, err, 0)
		if got := mock.PostCalls[0].Path; got != "/subscribers/s1/activation.json" {
			t.Errorf("expected the activation's path, got '%s'", got)
		}
		if data := responseDataMap(t, result); data["status"] != "bounced" {
			t.Errorf("expected the subscriber as it stands, got %#v", data)
		}
		if result.Response.Summary != "Nothing changed: it was not stopped, and is still bounced" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "deactivate"); got != "" {
			t.Errorf("expected no deactivate breadcrumb for a bounced subscriber, got %q", got)
		}
	})
}
