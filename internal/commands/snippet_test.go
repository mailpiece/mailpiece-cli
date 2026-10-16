package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

// newsletterCase is one command line of the newsletter's commands and the one write it must
// send, or the exit code it must refuse with before sending anything.
type newsletterCase struct {
	name   string
	args   []string
	method string
	path   string
	body   map[string]any
	exit   int
}

// newsletterRun runs each case through cobra against a fresh mock, and puts the flags of every
// command under root back to their defaults afterwards.
func newsletterRun(t *testing.T, root *cobra.Command, cases []newsletterCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A record of some kind, so a read is not answered with the mock's empty list.
			mock := NewMockClient().WithGetData(map[string]any{"id": "1"})
			SetTestModeWithSDK(mock)
			SetTestConfig("token", "acme", "https://mailpiece.example.com")
			defer resetTest()
			defer newsletterResetFlags(root)

			_, err := runCobraWithArgs(tc.args...)
			assertExitCode(t, err, tc.exit)

			calls := map[string][]MockCall{"POST": mock.PostCalls, "PATCH": mock.PatchCalls, "DELETE": mock.DeleteCalls, "GET": mock.GetCalls}
			if tc.exit != 0 {
				for method, sent := range calls {
					if len(sent) > 0 {
						t.Errorf("a refused command sent %s %s", method, sent[0].Path)
					}
				}
				return
			}
			sent := calls[tc.method]
			if len(sent) != 1 || sent[0].Path != tc.path {
				t.Fatalf("expected %s %s, got %#v", tc.method, tc.path, sent)
			}
			if tc.body == nil {
				if sent[0].Body != nil {
					t.Errorf("expected no body, got %#v", sent[0].Body)
				}
			} else if !reflect.DeepEqual(sent[0].Body, any(tc.body)) {
				t.Errorf("body = %#v, want %#v", sent[0].Body, tc.body)
			}
		})
	}
}

func newsletterResetFlags(cmd *cobra.Command) {
	registerResetFlags(cmd)
	for _, child := range cmd.Commands() {
		newsletterResetFlags(child)
	}
}

func TestSnippetCommands(t *testing.T) {
	contentFile := filepath.Join(t.TempDir(), "signature.md")
	if err := os.WriteFile(contentFile, []byte("Bye, **Ana**"), 0o600); err != nil {
		t.Fatal(err)
	}
	newsletterRun(t, snippetCmd, []newsletterCase{
		{
			name: "create reads the content from a file", args: []string{"snippet", "create", "--name", "Signature", "--content-file", contentFile},
			method: "POST", path: "/snippets.json",
			body: map[string]any{"name": "Signature", "content": "<p>Bye, <strong>Ana</strong></p>\n"},
		},
		{
			name: "content and content-file together are refused",
			args: []string{"snippet", "update", "s1", "--content", "Bye", "--content-file", contentFile}, exit: errors.ExitUsage,
		},
		{name: "show", args: []string{"snippet", "show", "s1"}, method: "GET", path: "/snippets/s1.json"},
		{
			name: "create turns Markdown into HTML", args: []string{"snippet", "create", "--name", "Signature", "--content", "Bye, **Ana**"},
			method: "POST", path: "/snippets.json",
			body: map[string]any{"name": "Signature", "content": "<p>Bye, <strong>Ana</strong></p>\n"},
		},
		{name: "create needs a name", args: []string{"snippet", "create", "--content", "Bye"}, exit: errors.ExitUsage},
		{
			name: "update sends only what was passed", args: []string{"snippet", "update", "s1", "--name", "Sign-off"},
			method: "PATCH", path: "/snippets/s1.json", body: map[string]any{"name": "Sign-off"},
		},
		{name: "update with nothing is refused", args: []string{"snippet", "update", "s1"}, exit: errors.ExitUsage},
		{name: "delete", args: []string{"snippet", "delete", "s1"}, method: "DELETE", path: "/snippets/s1.json"},
	})
}

func TestSnippetList(t *testing.T) {
	mock := NewMockClient().WithListData([]any{
		map[string]any{"id": "s1", "name": "Signature"},
		map[string]any{"id": "s2", "name": "Sponsor"},
	})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("snippet", "list")
	assertExitCode(t, err, 0)
	if got := mock.GetCalls[0].Path; got != "/snippets.json" {
		t.Errorf("expected GET /snippets.json, got %s", got)
	}
	if lastResult.Response == nil || lastResult.Response.Summary != "2 snippets" {
		t.Fatalf("expected two snippets, got %#v", lastResult.Response)
	}
}

func TestSnippetCreateNamesTheNewSnippet(t *testing.T) {
	mock := NewMockClient()
	mock.PostResponse = &APIResponse{StatusCode: 201, Location: "/acme/snippets/s9.json", Data: map[string]any{"id": "s9", "name": "Signature"}}
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()
	defer newsletterResetFlags(snippetCmd)

	_, err := runCobraWithArgs("snippet", "create", "--name", "Signature")
	assertExitCode(t, err, 0)
	if data := responseDataMap(t, lastResult); data["id"] != "s9" {
		t.Errorf("expected the snippet, got %#v", data)
	}
	if got := registerBreadcrumb(lastResult, "show"); got != "mailpiece snippet show s9" {
		t.Errorf("expected a show breadcrumb, got %q", got)
	}
}
