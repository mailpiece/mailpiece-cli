package commands

import (
	"net/http"
	"strings"
	"testing"
)

func TestShow(t *testing.T) {
	t.Run("reads the record through its json path on the URL's account", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{"id":"abc","title":"Hello"}`)
		result := setRawAPITest(server, "")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://mailpiece.example.com/834206594/posts/abc"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Method != "GET" || reqs[0].Path != "/834206594/posts/abc.json" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
		data := result.Response.Data.(map[string]any)
		if data["title"] != "Hello" {
			t.Errorf("expected the record, got %#v", data)
		}
		if result.Response.Summary != "Hello" {
			t.Errorf("expected the record's title as summary, got %q", result.Response.Summary)
		}
		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || crumbs[0].Cmd != "mailpiece api get posts/abc.json --account 834206594" {
			t.Errorf("expected the api breadcrumb, got %#v", crumbs)
		}
	})

	t.Run("sends the token to the configured host whatever host the URL names", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{"id":"abc"}`)
		setRawAPITest(server, "")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://evil.example/834206594/posts/abc"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Path != "/834206594/posts/abc.json" {
			t.Fatalf("expected the path on the configured host, got %#v", reqs)
		}
		if reqs[0].Authorization == "" {
			t.Error("expected the token on the configured host")
		}
	})

	t.Run("reads a page outside any account unprefixed", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{"id":"i1","accounts":[]}`)
		result := setRawAPITest(server, "12345")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://mailpiece.example.com/my/identity"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Path != "/my/identity.json" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
		if len(result.Response.Breadcrumbs) != 0 {
			t.Errorf("expected no api breadcrumb for an unscoped page, got %#v", result.Response.Breadcrumbs)
		}
	})

	t.Run("shows a list", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `[{"id":"c1"},{"id":"c2"}]`)
		result := setRawAPITest(server, "")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://mailpiece.example.com/834206594/posts"})
		assertExitCode(t, err, 0)
		if result.Response.Summary != "2 items" {
			t.Errorf("expected '2 items', got %q", result.Response.Summary)
		}
	})

	t.Run("a record out of reach is not found", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusNotFound, ``)
		setRawAPITest(server, "")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://mailpiece.example.com/834206594/posts/nope"})
		assertExitCode(t, err, 2)
	})

	t.Run("refuses what is not a URL without a request", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{}`)
		setRawAPITest(server, "")
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"posts/abc"})
		assertExitCode(t, err, 1)
		if err != nil && !strings.Contains(err.Error(), "Not a Mailpiece URL") {
			t.Errorf("unexpected error %q", err.Error())
		}
		if n := len(server.recorded()); n != 0 {
			t.Errorf("expected no request, got %d", n)
		}
	})

	t.Run("requires a token", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{}`)
		setRawAPITest(server, "")
		SetTestConfig("", "", server.URL)
		defer ResetTestMode()

		err := showCmd.RunE(showCmd, []string{"https://mailpiece.example.com/834206594/posts/abc"})
		assertExitCode(t, err, 3)
	})
}
