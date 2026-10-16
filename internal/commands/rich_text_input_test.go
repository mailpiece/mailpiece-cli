package commands

import (
	"encoding/json"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

// withStdin pipes text to cmd for one test.
func withStdin(t *testing.T, cmd *cobra.Command, text string) {
	t.Helper()
	cmd.SetIn(strings.NewReader(text))
	t.Cleanup(func() { cmd.SetIn(nil) })
}

// withTerminal makes stdin and stdout a terminal and $EDITOR answer edited, recording what
// it was opened with.
func withTerminal(t *testing.T, edited string) *string {
	t.Helper()
	opened := new(string)
	stdinWas, stdoutWas, editorWas := stdinIsTerminal, stdoutIsTerminal, openEditor
	stdinIsTerminal = func() bool { return true }
	stdoutIsTerminal = func() bool { return true }
	openEditor = func(initial string) (string, error) {
		*opened = initial
		if edited == "" {
			return "", stderrors.New("empty content")
		}
		return edited, nil
	}
	t.Cleanup(func() { stdinIsTerminal, stdoutIsTerminal, openEditor = stdinWas, stdoutWas, editorWas })
	return opened
}

// devNull is stdin with nothing piped, as an agent's shell gives it.
func devNull(t *testing.T, cmd *cobra.Command) {
	t.Helper()
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetIn(f)
	t.Cleanup(func() { cmd.SetIn(nil); f.Close() })
}

func TestRichTextShortFlags(t *testing.T) {
	for _, tc := range []struct {
		cmd   *cobra.Command
		flags map[string]string
	}{
		{postCreateCmd, map[string]string{"t": "title", "c": "body"}},
		{postUpdateCmd, map[string]string{"t": "title", "c": "body"}},
		{snippetCreateCmd, map[string]string{"c": "content"}},
		{snippetUpdateCmd, map[string]string{"c": "content"}},
		{journeyEmailCreateCmd, map[string]string{"t": "subject", "c": "body"}},
		{journeyEmailUpdateCmd, map[string]string{"t": "subject", "c": "body"}},
	} {
		for short, long := range tc.flags {
			if f := tc.cmd.Flags().ShorthandLookup(short); f == nil || f.Name != long {
				t.Errorf("%s: -%s is not --%s", tc.cmd.CommandPath(), short, long)
			}
		}
	}
}

func TestPostCreateReadsTheBody(t *testing.T) {
	t.Run("from piped stdin", func(t *testing.T) {
		mock, _ := mockTest(t)
		withStdin(t, postCreateCmd, "Some **bold** news\n")
		registerSetFlags(t, postCreateCmd, "title", "Shipping week")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		body := postBodyOf(t, mock.PostCalls[0])
		if html, _ := body["body"].(string); !strings.Contains(html, "<strong>bold</strong>") || body["title"] != "Shipping week" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("a flag before stdin, which is left unread", func(t *testing.T) {
		mock, _ := mockTest(t)
		stdin := strings.NewReader("from stdin")
		postCreateCmd.SetIn(stdin)
		t.Cleanup(func() { postCreateCmd.SetIn(nil) })
		registerSetFlags(t, postCreateCmd, "body", "from the flag")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if got := postBodyOf(t, mock.PostCalls[0])["body"]; got != "from the flag" {
			t.Errorf("body = %#v", got)
		}
		if stdin.Len() == 0 {
			t.Error("stdin was read though --body was given")
		}
	})

	t.Run("empty stdin gives a draft with a title alone", func(t *testing.T) {
		mock, _ := mockTest(t)
		withStdin(t, postCreateCmd, "  \n")
		registerSetFlags(t, postCreateCmd, "title", "Hello")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if _, sent := postBodyOf(t, mock.PostCalls[0])["body"]; sent {
			t.Error("an empty stdin was sent as the body")
		}
	})

	t.Run("in $EDITOR on a terminal", func(t *testing.T) {
		mock, _ := mockTest(t)
		opened := withTerminal(t, "Written in the _editor_")
		registerSetFlags(t, postCreateCmd, "title", "Hello")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if *opened != "" {
			t.Errorf("the editor opened on %q", *opened)
		}
		if html, _ := postBodyOf(t, mock.PostCalls[0])["body"].(string); !strings.Contains(html, "<em>editor</em>") {
			t.Errorf("body = %q", html)
		}
	})

	t.Run("an editor closed empty creates nothing", func(t *testing.T) {
		mock, _ := mockTest(t)
		withTerminal(t, "")
		registerSetFlags(t, postCreateCmd, "title", "Hello")

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), errors.ExitUsage)
		if len(mock.PostCalls) != 0 {
			t.Errorf("sent %d requests", len(mock.PostCalls))
		}
	})

	t.Run("with nothing at all, a usage error showing stdin and --body-file", func(t *testing.T) {
		mock, _ := mockTest(t)
		devNull(t, postCreateCmd)

		err := postCreateCmd.RunE(postCreateCmd, nil)
		assertExitCode(t, err, errors.ExitUsage)
		var cliErr *errors.CLIError
		if !stderrors.As(err, &cliErr) || !strings.Contains(cliErr.Hint, "| mailpiece post create") || !strings.Contains(cliErr.Hint, "--body-file") {
			t.Errorf("err = %#v", err)
		}
		if len(mock.PostCalls) != 0 {
			t.Errorf("sent %d requests", len(mock.PostCalls))
		}
	})
}

func TestPostUpdateReadsTheBody(t *testing.T) {
	t.Run("from piped stdin when nothing else is passed", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithPatchData(map[string]any{"id": "p1"})
		withStdin(t, postUpdateCmd, "<p>New text</p>")

		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), 0)

		body := postBodyOf(t, mock.PatchCalls[0])
		if len(body) != 1 || body["body"] != "<p>New text</p>" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("never from stdin when another field is passed", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.WithPatchData(map[string]any{"id": "p1"})
		stdin := strings.NewReader("<p>Not this</p>")
		postUpdateCmd.SetIn(stdin)
		t.Cleanup(func() { postUpdateCmd.SetIn(nil) })
		registerSetFlags(t, postUpdateCmd, "title", "Renamed")

		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), 0)

		body := postBodyOf(t, mock.PatchCalls[0])
		if len(body) != 1 || body["title"] != "Renamed" {
			t.Errorf("body = %#v", body)
		}
		if stdin.Len() == 0 {
			t.Error("stdin was read though --title was given")
		}
	})

	t.Run("in $EDITOR, starting from the body as written", func(t *testing.T) {
		mock, _ := mockTest(t)
		mock.OnGet("/posts/p1.json", &APIResponse{StatusCode: 200, Data: map[string]any{"id": "p1", "body_html": "<p>Old</p>"}})
		mock.WithPatchData(map[string]any{"id": "p1"})
		opened := withTerminal(t, "<p>Old, and new</p>")

		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), 0)

		if *opened != "<p>Old</p>" {
			t.Errorf("the editor opened on %q", *opened)
		}
		if got := postBodyOf(t, mock.PatchCalls[0])["body"]; got != "<p>Old, and new</p>" {
			t.Errorf("body = %#v", got)
		}
	})

	t.Run("with nothing at all, a usage error showing stdin and --body-file", func(t *testing.T) {
		mock, _ := mockTest(t)
		devNull(t, postUpdateCmd)

		err := postUpdateCmd.RunE(postUpdateCmd, []string{"p1"})
		assertExitCode(t, err, errors.ExitUsage)
		var cliErr *errors.CLIError
		if !stderrors.As(err, &cliErr) || !strings.Contains(cliErr.Hint, "| mailpiece post update p1") {
			t.Errorf("err = %#v", err)
		}
		if len(mock.PatchCalls) != 0 {
			t.Errorf("sent %d requests", len(mock.PatchCalls))
		}
	})
}

func TestSnippetAndJourneyEmailReadStdin(t *testing.T) {
	t.Run("snippet create", func(t *testing.T) {
		mock, _ := mockTest(t)
		withStdin(t, snippetCreateCmd, "Bye, **Ana**")
		registerSetFlags(t, snippetCreateCmd, "name", "Signature")

		assertExitCode(t, snippetCreateCmd.RunE(snippetCreateCmd, nil), 0)

		if html, _ := postBodyOf(t, mock.PostCalls[0])["content"].(string); !strings.Contains(html, "<strong>Ana</strong>") {
			t.Errorf("content = %q", html)
		}
	})

	t.Run("snippet update given nothing else", func(t *testing.T) {
		mock, _ := mockTest(t)
		withStdin(t, snippetUpdateCmd, "<p>Cheers</p>")

		assertExitCode(t, snippetUpdateCmd.RunE(snippetUpdateCmd, []string{"s1"}), 0)

		if body := postBodyOf(t, mock.PatchCalls[0]); len(body) != 1 || body["content"] != "<p>Cheers</p>" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("journey email update given nothing else", func(t *testing.T) {
		mock, _ := mockTest(t)
		withStdin(t, journeyEmailUpdateCmd, "<p>Welcome aboard</p>")

		assertExitCode(t, journeyEmailUpdateCmd.RunE(journeyEmailUpdateCmd, []string{"subscriber_confirmed", "e1"}), 0)

		if body := postBodyOf(t, mock.PatchCalls[0]); len(body) != 1 || body["body"] != "<p>Welcome aboard</p>" {
			t.Errorf("body = %#v", body)
		}
	})

	t.Run("journey email update given nothing at all", func(t *testing.T) {
		mock, _ := mockTest(t)
		devNull(t, journeyEmailUpdateCmd)

		assertExitCode(t, journeyEmailUpdateCmd.RunE(journeyEmailUpdateCmd, []string{"subscriber_confirmed", "e1"}), errors.ExitUsage)
		if len(mock.PatchCalls) != 0 {
			t.Errorf("sent %d requests", len(mock.PatchCalls))
		}
	})
}

// attachServer is the app as an upload meets it: the direct upload, the storage PUT and the
// post's write.
type attachServer struct {
	*httptest.Server
	mu       sync.Mutex
	uploaded []string
	posts    []map[string]any
}

func newAttachServer(t *testing.T, currentBody string) *attachServer {
	t.Helper()
	s := &attachServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		defer s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "POST" && r.URL.Path == "/acme/rails/active_storage/direct_uploads.json":
			var req struct {
				Blob struct {
					Filename    string `json:"filename"`
					ContentType string `json:"content_type"`
					ByteSize    int64  `json:"byte_size"`
				} `json:"blob"`
			}
			_ = json.Unmarshal(body, &req)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"signed_id": "SIGNED", "attachable_sgid": "SGID-" + req.Blob.Filename,
				"filename": req.Blob.Filename, "content_type": req.Blob.ContentType, "byte_size": req.Blob.ByteSize,
				"direct_upload": map[string]any{"url": s.URL + "/disk/" + req.Blob.Filename, "headers": map[string]string{}},
			})
		case r.Method == "PUT" && strings.HasPrefix(r.URL.Path, "/disk/"):
			if r.Header.Get("Authorization") != "" {
				t.Error("the storage PUT carried the token")
			}
			s.uploaded = append(s.uploaded, string(body))
			w.WriteHeader(http.StatusNoContent)
		case r.Method == "GET" && r.URL.Path == "/acme/posts/p1.json":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "p1", "body_html": currentBody})
		case r.URL.Path == "/acme/posts.json" || r.URL.Path == "/acme/posts/p1.json":
			var post map[string]any
			_ = json.Unmarshal(body, &post)
			s.posts = append(s.posts, post)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"p1"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func attachTest(t *testing.T, currentBody string) *attachServer {
	t.Helper()
	srv := newAttachServer(t, currentBody)
	SetTestMode()
	SetTestSDK(srv.URL)
	SetTestConfig("token", "acme", srv.URL)
	setContextOnAll(t.Context(), rootCmd)
	t.Cleanup(resetTest)
	return srv
}

func writeAttachment(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPostAttach(t *testing.T) {
	t.Run("create uploads each file and embeds it after the body, as the editor does", func(t *testing.T) {
		srv := attachTest(t, "")
		chart, notes := writeAttachment(t, "chart.png", "PNGDATA"), writeAttachment(t, "notes.pdf", "PDF")
		registerSetFlags(t, postCreateCmd, "title", "Shipping week", "body", "See below.", "attach", chart, "attach", notes)

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), 0)

		if strings.Join(srv.uploaded, ",") != "PNGDATA,PDF" {
			t.Errorf("uploaded %q", srv.uploaded)
		}
		want := "See below.\n" +
			`<action-text-attachment sgid="SGID-chart.png" content-type="image/png" filename="chart.png" filesize="7" presentation="gallery"></action-text-attachment>` + "\n" +
			`<action-text-attachment sgid="SGID-notes.pdf" content-type="application/pdf" filename="notes.pdf" filesize="3" presentation="gallery"></action-text-attachment>`
		if len(srv.posts) != 1 || srv.posts[0]["body"] != want {
			t.Errorf("posts = %#v\nwant body %q", srv.posts, want)
		}
	})

	t.Run("update with --attach alone appends to the body as written", func(t *testing.T) {
		srv := attachTest(t, "<p>Old</p>")
		registerSetFlags(t, postUpdateCmd, "attach", writeAttachment(t, "chart.png", "PNG"))

		assertExitCode(t, postUpdateCmd.RunE(postUpdateCmd, []string{"p1"}), 0)

		if len(srv.posts) != 1 {
			t.Fatalf("posts = %#v", srv.posts)
		}
		body, _ := srv.posts[0]["body"].(string)
		if !strings.HasPrefix(body, "<p>Old</p>\n<action-text-attachment sgid=\"SGID-chart.png\"") || len(srv.posts[0]) != 1 {
			t.Errorf("post = %#v", srv.posts[0])
		}
	})

	t.Run("a missing file refuses the command before anything is sent", func(t *testing.T) {
		srv := attachTest(t, "")
		registerSetFlags(t, postCreateCmd, "title", "Hello", "attach", filepath.Join(t.TempDir(), "missing.png"))

		assertExitCode(t, postCreateCmd.RunE(postCreateCmd, nil), errors.ExitUsage)
		if len(srv.uploaded) != 0 || len(srv.posts) != 0 {
			t.Errorf("sent uploads %q and posts %#v", srv.uploaded, srv.posts)
		}
	})
}
