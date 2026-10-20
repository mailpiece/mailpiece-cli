package commands

import (
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// registerSetFlags sets flags on cmd as a parse would, so Changed reports them,
// and puts every flag of cmd back to its default when the test ends.
func registerSetFlags(t *testing.T, cmd *cobra.Command, pairs ...string) {
	t.Helper()
	t.Cleanup(func() { registerResetFlags(cmd) })
	for i := 0; i+1 < len(pairs); i += 2 {
		if err := cmd.Flags().Set(pairs[i], pairs[i+1]); err != nil {
			t.Fatalf("setting --%s: %v", pairs[i], err)
		}
	}
}

func registerResetFlags(cmd *cobra.Command) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}

// registerBreadcrumb returns the command of the breadcrumb with action, or "".
func registerBreadcrumb(result *CommandResult, action string) string {
	if result == nil || result.Response == nil {
		return ""
	}
	for _, b := range result.Response.Breadcrumbs {
		if b.Action == action {
			return b.Cmd
		}
	}
	return ""
}

func TestPagedListRun(t *testing.T) {
	t.Run("returns one page with summary and breadcrumbs", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data: []any{
				map[string]any{"id": "s1", "name": "North"},
				map[string]any{"id": "s2", "name": "South"},
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		assertExitCode(t, err, 0)

		if !result.Response.OK {
			t.Error("expected success response")
		}
		if result.Response.Summary != "2 fixtures" {
			t.Errorf("expected summary '2 fixtures', got %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "show"); got != "mailpiece fixture show ID" {
			t.Errorf("expected show breadcrumb, got %q", got)
		}
		if got := registerBreadcrumb(result, "next"); got != "" {
			t.Errorf("expected no next breadcrumb, got %q", got)
		}
		if _, ok := result.Response.Context["pagination"]; ok {
			t.Error("expected no pagination context without a next link")
		}
	})

	t.Run("next link adds breadcrumb, notice and pagination context", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1"}},
			LinkNext:   "/acme/fixtures.json?page=2",
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		assertExitCode(t, err, 0)

		if got := registerBreadcrumb(result, "next"); got != "mailpiece fixture list --page 2" {
			t.Errorf("expected next breadcrumb 'mailpiece fixture list --page 2', got %q", got)
		}
		want := "Showing 1 results: there are more (use --page, or --all for the complete list)"
		if result.Response.Notice != want {
			t.Errorf("expected notice %q, got %q", want, result.Response.Notice)
		}
		pagination, ok := result.Response.Context["pagination"].(map[string]any)
		if !ok {
			t.Fatalf("expected pagination context, got %#v", result.Response.Context)
		}
		if pagination["has_next"] != true || pagination["next_url"] != "/acme/fixtures.json?page=2" {
			t.Errorf("unexpected pagination context %#v", pagination)
		}
	})

	t.Run("next breadcrumb follows the page asked for", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1"}},
			LinkNext:   "/acme/fixtures.json?page=4",
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		fixtureListPages.page = 3
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.page = 0

		assertExitCode(t, err, 0)
		if got := mock.GetWithPaginationCalls[0].Path; got != "/fixtures.json?page=3" {
			t.Errorf("expected path '/fixtures.json?page=3', got '%s'", got)
		}
		if result.Response.Summary != "1 fixtures (page 3)" {
			t.Errorf("expected summary '1 fixtures (page 3)', got %q", result.Response.Summary)
		}
		if got := registerBreadcrumb(result, "next"); got != "mailpiece fixture list --page 4" {
			t.Errorf("expected next breadcrumb for page 4, got %q", got)
		}
	})

	t.Run("an empty page ignores the next link the app sends past the end", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{},
			LinkNext:   "/acme/fixtures.json?page=3",
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		fixtureListPages.page = 2
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.page = 0

		assertExitCode(t, err, 0)
		if got := registerBreadcrumb(result, "next"); got != "" {
			t.Errorf("expected no next breadcrumb, got %q", got)
		}
		if result.Response.Notice != "" {
			t.Errorf("expected no notice, got %q", result.Response.Notice)
		}
	})

	t.Run("--all fetches every page from the first", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1"}, map[string]any{"id": "s2"}},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		fixtureListPages.all = true
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.all = false

		assertExitCode(t, err, 0)
		if len(mock.GetWithPaginationCalls) != 1 {
			t.Fatalf("expected 1 GET call, got %d", len(mock.GetWithPaginationCalls))
		}
		if got := mock.GetWithPaginationCalls[0].Path; got != "/fixtures.json" {
			t.Errorf("expected path '/fixtures.json', got '%s'", got)
		}
		if result.Response.Summary != "2 fixtures (all)" {
			t.Errorf("expected summary '2 fixtures (all)', got %q", result.Response.Summary)
		}
		if items, ok := result.Response.Data.([]any); !ok || len(items) != 2 {
			t.Errorf("expected 2 items, got %#v", result.Response.Data)
		}
	})

	t.Run("--limit truncates a page", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1"}, map[string]any{"id": "s2"}, map[string]any{"id": "s3"}},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgLimit = 2
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})

		assertExitCode(t, err, 0)
		if items, ok := result.Response.Data.([]any); !ok || len(items) != 2 {
			t.Errorf("expected 2 items, got %#v", result.Response.Data)
		}
		if result.Response.Notice != "Showing 2 of 3 results" {
			t.Errorf("expected limit notice, got %q", result.Response.Notice)
		}
	})

	t.Run("refuses --limit with --all", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgLimit = 5
		fixtureListPages.all = true
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.all = false

		assertExitCode(t, err, errors.ExitInvalidArgs)
		if len(mock.GetWithPaginationCalls) != 0 {
			t.Errorf("expected no request, got %d", len(mock.GetWithPaginationCalls))
		}
	})

	t.Run("refuses --all with --page", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		fixtureListPages.all = true
		fixtureListPages.page = 2
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.all = false
		fixtureListPages.page = 0

		assertExitCode(t, err, errors.ExitInvalidArgs)
		if len(mock.GetWithPaginationCalls) != 0 {
			t.Errorf("expected no request, got %d", len(mock.GetWithPaginationCalls))
		}
	})

	t.Run("refuses a negative page", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		fixtureListPages.page = -1
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.page = 0

		assertExitCode(t, err, errors.ExitInvalidArgs)
	})

	t.Run("styled output lists the next steps", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1", "name": "North"}},
			LinkNext:   "/acme/fixtures.json?page=2",
		}

		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()
		SetTestFormat(output.FormatStyled)

		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		assertExitCode(t, err, 0)

		got := TestOutput()
		for _, want := range []string{"North", "there are more", "Next steps:", "mailpiece fixture list --page 2  # Next page"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected styled output to contain %q, got:\n%s", want, got)
			}
		}
	})

	t.Run("markdown output lists the next steps", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       []any{map[string]any{"id": "s1", "name": "North"}},
		}

		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()
		SetTestFormat(output.FormatMarkdown)

		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		assertExitCode(t, err, 0)

		got := TestOutput()
		for _, want := range []string{"### Next steps", "- `mailpiece fixture show ID`: View a fixture"} {
			if !strings.Contains(got, want) {
				t.Errorf("expected markdown output to contain %q, got:\n%s", want, got)
			}
		}
	})
}

func TestListPath(t *testing.T) {
	t.Run("escapes ids", func(t *testing.T) {
		got, err := listPath("/fixtures/%s/events.json", "a/b c")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/fixtures/a%2Fb%20c/events.json" {
			t.Errorf("expected escaped path, got %q", got)
		}
	})

	t.Run("fills numbers", func(t *testing.T) {
		got, err := listPath("/posts/%d/links.json", int64(7))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "/posts/7/links.json" {
			t.Errorf("expected '/posts/7/links.json', got %q", got)
		}
	})

	t.Run("refuses a number below 1", func(t *testing.T) {
		_, err := listPath("/posts/%d/links.json", int64(0))
		assertExitCode(t, err, errors.ExitInvalidArgs)
	})
}

func TestJSONAnySlice(t *testing.T) {
	got := jsonAnySlice(nil)
	maps, ok := got.([]map[string]any)
	if !ok || len(maps) != 0 {
		t.Fatalf("expected an empty list of maps, got %#v", got)
	}
}
