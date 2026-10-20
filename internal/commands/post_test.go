package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

// postBodyOf is the JSON body of a recorded write.
func postBodyOf(t *testing.T, call MockCall) map[string]any {
	t.Helper()
	body, ok := call.Body.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON object body, got %#v", call.Body)
	}
	return body
}

func TestPostList(t *testing.T) {
	t.Run("lists drafts and scheduled posts on one page", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.WithListData([]any{map[string]any{"id": "p1", "title": "Draft"}, map[string]any{"id": "p2", "title": "Scheduled"}})

		assertExitCode(t, postListCmd.RunE(postListCmd, nil), 0)

		if got := mock.GetWithPaginationCalls[0].Path; got != "/posts.json" {
			t.Errorf("path = %q", got)
		}
		if result.Response.Summary != "2 drafts and scheduled posts" {
			t.Errorf("summary = %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "sent"); got != "mailpiece post list --sent" {
			t.Errorf("sent breadcrumb = %q", got)
		}
	})

	t.Run("--published and --sent read their own paged lists", func(t *testing.T) {
		for flag, want := range map[string]string{"published": "/posts/published.json?page=2", "sent": "/posts/sent.json?page=2"} {
			mock, result := mockTest(t)
			mock.GetWithPaginationResponse = &APIResponse{StatusCode: 200, Data: []any{map[string]any{"id": "p1"}}, LinkNext: "/acme/posts/" + flag + ".json?page=3"}
			registerSetFlags(t, postListCmd, flag, "true", "page", "2")

			assertExitCode(t, postListCmd.RunE(postListCmd, nil), 0)

			if got := mock.GetWithPaginationCalls[0].Path; got != want {
				t.Errorf("--%s path = %q, want %q", flag, got, want)
			}
			if got := registerBreadcrumb(result, "next"); got != "mailpiece post list --"+flag+" --page 3" {
				t.Errorf("--%s next breadcrumb = %q", flag, got)
			}
			registerResetFlags(postListCmd)
			resetTest()
		}
	})

	t.Run("refuses --page on the one-page list, and both lists at once", func(t *testing.T) {
		mock, _ := mockTest(t)
		registerSetFlags(t, postListCmd, "page", "2")
		assertExitCode(t, postListCmd.RunE(postListCmd, nil), errors.ExitInvalidArgs)

		registerSetFlags(t, postListCmd, "page", "0", "published", "true", "sent", "true")
		assertExitCode(t, postListCmd.RunE(postListCmd, nil), errors.ExitInvalidArgs)

		if len(mock.GetWithPaginationCalls) != 0 {
			t.Errorf("a refused list sent %d requests", len(mock.GetWithPaginationCalls))
		}
	})

	t.Run("requires authentication", func(t *testing.T) {
		mockTest(t)
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		assertExitCode(t, postListCmd.RunE(postListCmd, nil), errors.ExitAuth)
	})
}

func TestPostShow(t *testing.T) {
	t.Run("a draft is offered its release", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.WithGetData(map[string]any{"id": "p1", "title": "Draft", "published": false})

		assertExitCode(t, postShowCmd.RunE(postShowCmd, []string{"p1"}), 0)

		if got := mock.GetCalls[0].Path; got != "/posts/p1.json" {
			t.Errorf("path = %q", got)
		}
		if got := registerBreadcrumb(result, "publish"); got != "mailpiece post publish p1" {
			t.Errorf("publish breadcrumb = %q", got)
		}
	})

	t.Run("a sent post is offered its links, never a second delivery", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.WithGetData(map[string]any{"id": "p1", "published": true, "sent_at": "2026-01-02T03:04:05.000Z"})

		assertExitCode(t, postShowCmd.RunE(postShowCmd, []string{"p1"}), 0)

		if got := registerBreadcrumb(result, "links"); got != "mailpiece post link list p1" {
			t.Errorf("links breadcrumb = %q", got)
		}
		if got := registerBreadcrumb(result, "deliver"); got != "" {
			t.Errorf("a sent post was offered %q", got)
		}
	})

	t.Run("not found", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithNotFoundError()
		assertExitCode(t, postShowCmd.RunE(postShowCmd, []string{"nope"}), errors.ExitNotFound)
	})
}

func TestPostCreate(t *testing.T) {
	t.Run("sends the fields flat, the body as HTML", func(t *testing.T) {
		mock, result := mockTest(t)
		mock.PostResponse = &APIResponse{StatusCode: 201, Location: "/acme/posts/p1.json", Data: map[string]any{"id": "p1", "title": "Hello"}}
		registerSetFlags(t, postCreateCmd, "title", "Hello", "body", "Some **bold** text", "visibility", "unlisted")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if got := mock.PostCalls[0].Path; got != "/posts.json" {
			t.Errorf("path = %q", got)
		}
		body := postBodyOf(t, mock.PostCalls[0])
		if body["title"] != "Hello" || body["visibility"] != "unlisted" {
			t.Errorf("body = %#v", body)
		}
		if html, _ := body["body"].(string); !strings.Contains(html, "<strong>bold</strong>") {
			t.Errorf("body was not converted to HTML: %q", body["body"])
		}
		if _, sent := body["summary"]; sent {
			t.Error("a summary not given was sent")
		}
		if got := registerBreadcrumb(result, "test"); got != "mailpiece post test p1" {
			t.Errorf("test breadcrumb = %q", got)
		}
	})

	t.Run("reads the body from a file, audience boxes untouched", func(t *testing.T) {
		mock, _ := mockTest(t)
		text := "# Title\n\n<aside data-audience=\"web\">\n\nOnly on the **web**.\n\n</aside>\n\n<aside data-audience=\"email\"><p>Only by email.</p></aside>\n"
		path := filepath.Join(t.TempDir(), "post.md")
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		registerSetFlags(t, postCreateCmd, "body-file", path)

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		html, _ := postBodyOf(t, mock.PostCalls[0])["body"].(string)
		for _, want := range []string{
			`<aside data-audience="web">`, "<strong>web</strong>", "</aside>",
			`<aside data-audience="email"><p>Only by email.</p></aside>`, "<h1>Title</h1>",
		} {
			if !strings.Contains(html, want) {
				t.Errorf("expected the body to contain %q\ngot: %s", want, html)
			}
		}
	})

	t.Run("reads the body from stdin", func(t *testing.T) {
		mock, _ := mockTest(t)
		postCreateCmd.SetIn(strings.NewReader("<p>From stdin</p>"))
		t.Cleanup(func() { postCreateCmd.SetIn(nil) })
		registerSetFlags(t, postCreateCmd, "title", "Hello", "body-file", "-")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if html, _ := postBodyOf(t, mock.PostCalls[0])["body"].(string); strings.TrimSpace(html) != "<p>From stdin</p>" {
			t.Errorf("body = %q", html)
		}
	})

	t.Run("refuses an empty post, two bodies and a file it cannot read", func(t *testing.T) {
		mock, _ := mockTest(t)
		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), errors.ExitInvalidArgs)

		registerSetFlags(t, postCreateCmd, "body", "a", "body-file", "b")
		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), errors.ExitInvalidArgs)
		registerResetFlags(postCreateCmd)

		registerSetFlags(t, postCreateCmd, "body-file", filepath.Join(t.TempDir(), "missing.md"))
		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), errors.ExitInvalidArgs)

		if len(mock.PostCalls) != 0 {
			t.Errorf("a refused create sent %d requests", len(mock.PostCalls))
		}
	})
}

func TestPostUpdate(t *testing.T) {
	t.Run("sends only what was passed, an emptied field included", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithPatchData(map[string]any{"id": "p1"})
		registerSetFlags(t, postUpdateCmd, "summary", "", "published-at", "2026-01-02T03:00:00Z")

		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), 0)

		if got := mock.PatchCalls[0].Path; got != "/posts/p1.json" {
			t.Errorf("path = %q", got)
		}
		body := postBodyOf(t, mock.PatchCalls[0])
		if len(body) != 2 || body["summary"] != "" || body["published_at"] != "2026-01-02T03:00:00Z" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("refuses an update of nothing", func(t *testing.T) {
		mock, _ := mockTest(t)
		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), errors.ExitInvalidArgs)
		if len(mock.PatchCalls) != 0 {
			t.Errorf("a refused update sent %d requests", len(mock.PatchCalls))
		}
	})
}

func TestPostDelete(t *testing.T) {
	mock, result := mockTest(t)

	assertExitCode(t, postDeleteCmd.RunE(postDeleteCmd, []string{"p1"}), 0)

	if got := mock.DeleteCalls[0].Path; got != "/posts/p1.json" {
		t.Errorf("path = %q", got)
	}
	if data := responseDataMap(t, result); data["deleted"] != true || data["id"] != "p1" {
		t.Errorf("data = %#v", data)
	}
	if !strings.Contains(postDeleteCmd.Long, "cannot be undone") {
		t.Error("the help of a delete must say it cannot be undone")
	}
}
