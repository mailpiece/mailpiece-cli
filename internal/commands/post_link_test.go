package commands

import (
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

func TestPostLinkList(t *testing.T) {
	mock, result := mockTest(t)
	mock.WithListData([]any{map[string]any{"id": "l1", "url": "https://example.com", "stale": false}})

	assertExitCode(t, postLinkListCmd.RunE(postLinkListCmd, []string{"p1"}), 0)

	if got := mock.GetWithPaginationCalls[0].Path; got != "/posts/p1/links.json" {
		t.Errorf("path = %q", got)
	}
	if result.Response.Summary != "1 links" {
		t.Errorf("summary = %q", result.Response.Summary)
	}
	if got := registerBreadcrumb(result, "update"); got != "mailpiece post link update p1 LINK_ID --url URL" {
		t.Errorf("update breadcrumb = %q", got)
	}
}

func TestPostLinkUpdate(t *testing.T) {
	t.Run("changes the url", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithPatchData(map[string]any{"id": "l1", "url": "https://example.com/new"})
		registerSetFlags(t, postLinkUpdateCmd, "url", "https://example.com/new")

		assertExitCode(t, postLinkUpdateCmd.RunE(postLinkUpdateCmd, []string{"p1", "l1"}), 0)

		if got := mock.PatchCalls[0].Path; got != "/posts/p1/links/l1.json" {
			t.Errorf("path = %q", got)
		}
		if body := postBodyOf(t, mock.PatchCalls[0]); len(body) != 1 || body["url"] != "https://example.com/new" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("needs --url", func(t *testing.T) {
		mock, _ := mockTest(t)
		assertExitCode(t, postLinkUpdateCmd.RunE(postLinkUpdateCmd, []string{"p1", "l1"}), errors.ExitInvalidArgs)
		if len(mock.PatchCalls) != 0 {
			t.Errorf("a refused update sent %d requests", len(mock.PatchCalls))
		}
	})
}
