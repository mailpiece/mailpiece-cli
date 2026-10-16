package commands

import (
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestPostPublish(t *testing.T) {
	t.Run("publishes the post as it is", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"id": "p1", "published": true}}

		assertExitCode(t, postPublishCmd.RunE(postPublishCmd, []string{"p1"}), 0)

		if got := mock.PostCalls[0].Path; got != "/posts/p1/publication.json" {
			t.Errorf("path = %q", got)
		}
		if body := postBodyOf(t, mock.PostCalls[0]); len(body) != 0 {
			t.Errorf("body = %#v, want it empty", body)
		}
		if got := registerBreadcrumb(result, "deliver"); got != "mailpiece post deliver p1" {
			t.Errorf("deliver breadcrumb = %q", got)
		}
	})

	t.Run("releases under a past day and hour", func(t *testing.T) {
		mock, _ := mockTest(t)
		registerSetFlags(t, postPublishCmd, "destination", "link", "day", "2024-03-01", "hour", "9")

		assertExitCode(t, postPublishCmd.RunE(postPublishCmd, []string{"p1"}), 0)

		body := postBodyOf(t, mock.PostCalls[0])
		if body["destination"] != "link" || body["day"] != "2024-03-01" || body["hour"] != float64(9) {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("refuses a date given twice and a destination the app does not take", func(t *testing.T) {
		mock, _ := mockTest(t)
		registerSetFlags(t, postPublishCmd, "published-at", "2024-03-01T09:00:00Z", "day", "2024-03-01")
		assertExitCode(t, postPublishCmd.RunE(postPublishCmd, []string{"p1"}), errors.ExitInvalidArgs)
		registerResetFlags(postPublishCmd)

		registerSetFlags(t, postPublishCmd, "destination", "everywhere")
		assertExitCode(t, postPublishCmd.RunE(postPublishCmd, []string{"p1"}), errors.ExitInvalidArgs)

		if len(mock.PostCalls) != 0 {
			t.Errorf("a refused publication sent %d requests", len(mock.PostCalls))
		}
	})

	t.Run("a refusal by the app is a refusal, with its reason", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.PostError = errors.NewForbiddenError("This post cannot go to that destination")

		err := postPublishCmd.RunE(postPublishCmd, []string{"p1"})

		assertExitCode(t, err, errors.ExitForbidden)
		if err == nil || !strings.Contains(err.Error(), "This post cannot go to that destination") {
			t.Errorf("error = %v, want the app's reason", err)
		}
	})
}

func TestPostUnpublish(t *testing.T) {
	mock, result := mockTest(t)

	assertExitCode(t, postUnpublishCmd.RunE(postUnpublishCmd, []string{"p1"}), 0)

	if got := mock.DeleteCalls[0].Path; got != "/posts/p1/publication.json" {
		t.Errorf("path = %q", got)
	}
	if data := responseDataMap(t, result); data["unpublished"] != true {
		t.Errorf("data = %#v", data)
	}
}

func TestPostSchedule(t *testing.T) {
	t.Run("schedules a delivery at an instant", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"id": "p1", "scheduled_delivery": true}}
		registerSetFlags(t, postScheduleCmd, "at", "2030-01-01T09:00:00Z", "deliver", "true")

		assertExitCode(t, postScheduleCmd.RunE(postScheduleCmd, []string{"p1"}), 0)

		if got := mock.PostCalls[0].Path; got != "/posts/p1/schedule.json" {
			t.Errorf("path = %q", got)
		}
		body := postBodyOf(t, mock.PostCalls[0])
		if len(body) != 2 || body["scheduled_posting_at"] != "2030-01-01T09:00:00Z" || body["scheduled_delivery"] != true {
			t.Errorf("body = %#v", body)
		}
		if got := registerBreadcrumb(result, "unschedule"); got != "mailpiece post unschedule p1" {
			t.Errorf("unschedule breadcrumb = %q", got)
		}
	})

	t.Run("schedules by day, with a destination and no delivery of its own", func(t *testing.T) {
		mock, _ := mockTest(t)
		registerSetFlags(t, postScheduleCmd, "day", "2030-01-01", "destination", "web_and_email")

		assertExitCode(t, postScheduleCmd.RunE(postScheduleCmd, []string{"p1"}), 0)

		body := postBodyOf(t, mock.PostCalls[0])
		if len(body) != 2 || body["day"] != "2030-01-01" || body["destination"] != "web_and_email" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("needs a time", func(t *testing.T) {
		mock, _ := mockTest(t)
		registerSetFlags(t, postScheduleCmd, "deliver", "true")
		assertExitCode(t, postScheduleCmd.RunE(postScheduleCmd, []string{"p1"}), errors.ExitInvalidArgs)
		if len(mock.PostCalls) != 0 {
			t.Errorf("a refused schedule sent %d requests", len(mock.PostCalls))
		}
	})
}

func TestPostUnschedule(t *testing.T) {
	mock, _ := mockTest(t)

	assertExitCode(t, postUnscheduleCmd.RunE(postUnscheduleCmd, []string{"p1"}), 0)

	if got := mock.DeleteCalls[0].Path; got != "/posts/p1/schedule.json" {
		t.Errorf("path = %q", got)
	}
}

func TestPostDeliver(t *testing.T) {
	t.Run("mails the list and says how many", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Data: map[string]any{"id": "p1", "published": true, "sent_at": "2026-01-02T03:04:05.000Z", "deliveries_count": 42}}

		assertExitCode(t, postDeliverCmd.RunE(postDeliverCmd, []string{"p1"}), 0)

		if got := mock.PostCalls[0].Path; got != "/posts/p1/delivery.json" {
			t.Errorf("path = %q", got)
		}
		if mock.PostCalls[0].Body != nil {
			t.Errorf("body = %#v, want none", mock.PostCalls[0].Body)
		}
		if result.Response.Summary != "Mailing started, to 42 subscribers" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
	})

	t.Run("says so when the post was already sent", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 200, Data: map[string]any{"id": "p1", "published": true, "sent_at": "2026-01-02T03:04:05.000Z", "deliveries_count": 42}}

		assertExitCode(t, postDeliverCmd.RunE(postDeliverCmd, []string{"p1"}), 0)

		if result.Response.Summary != "Already sent; nobody was mailed again" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
	})

	t.Run("says so when nobody is active to mail", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 200, Data: map[string]any{"id": "p1", "published": true, "sent_at": nil}}

		assertExitCode(t, postDeliverCmd.RunE(postDeliverCmd, []string{"p1"}), 0)

		if result.Response.Summary != "Published; nobody is active on the list, so nobody was mailed" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
	})

	t.Run("a newsletter without a sender is refused", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithValidationError("sender is incomplete")
		assertExitCode(t, postDeliverCmd.RunE(postDeliverCmd, []string{"p1"}), errors.ExitAPI)
	})

	t.Run("its help says it cannot be undone", func(t *testing.T) {
		if !strings.Contains(postDeliverCmd.Long, "cannot be undone") {
			t.Error("the help of a delivery must say it cannot be undone")
		}
	})
}

func TestPostTest(t *testing.T) {
	t.Run("mails yourself with no body", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201}

		assertExitCode(t, postTestCmd.RunE(postTestCmd, []string{"p1"}), 0)

		if got := mock.PostCalls[0].Path; got != "/posts/p1/test_delivery.json" {
			t.Errorf("path = %q", got)
		}
		if mock.PostCalls[0].Body != nil {
			t.Errorf("body = %#v, want none", mock.PostCalls[0].Body)
		}
		if data := responseDataMap(t, result); data["test_sent"] != true {
			t.Errorf("data = %#v", data)
		}
	})

	t.Run("mails the address --to names", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201}
		registerSetFlags(t, postTestCmd, "to", "ana@example.com")

		assertExitCode(t, postTestCmd.RunE(postTestCmd, []string{"p1"}), 0)

		if body := postBodyOf(t, mock.PostCalls[0]); body["email_address"] != "ana@example.com" {
			t.Errorf("body = %#v", body)
		}
	})
}

func TestPostRelistAndUnlist(t *testing.T) {
	mock, result := mockTest(t)

	assertExitCode(t, postRelistCmd.RunE(postRelistCmd, []string{"p1"}), 0)
	if got := mock.PostCalls[0].Path; got != "/posts/p1/listing.json" {
		t.Errorf("relist path = %q", got)
	}
	if data := responseDataMap(t, result); data["visibility"] != "listed" {
		t.Errorf("relist data = %#v", data)
	}

	assertExitCode(t, postUnlistCmd.RunE(postUnlistCmd, []string{"p1"}), 0)
	if got := mock.DeleteCalls[0].Path; got != "/posts/p1/listing.json" {
		t.Errorf("unlist path = %q", got)
	}
}
