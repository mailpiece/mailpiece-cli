package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
)

func TestResolveFormat(t *testing.T) {
	defer resetTest()

	t.Run("default is Auto", func(t *testing.T) {
		resetTest()
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatAuto {
			t.Errorf("expected FormatAuto, got %v", f)
		}
	})

	t.Run("--json resolves to JSON", func(t *testing.T) {
		resetTest()
		cfgJSON = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatJSON {
			t.Errorf("expected FormatJSON, got %v", f)
		}
	})

	t.Run("--quiet resolves to Quiet", func(t *testing.T) {
		resetTest()
		cfgQuiet = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatQuiet {
			t.Errorf("expected FormatQuiet, got %v", f)
		}
	})

	t.Run("--ids-only resolves to IDs", func(t *testing.T) {
		resetTest()
		cfgIDsOnly = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatIDs {
			t.Errorf("expected FormatIDs, got %v", f)
		}
	})

	t.Run("--count resolves to Count", func(t *testing.T) {
		resetTest()
		cfgCount = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatCount {
			t.Errorf("expected FormatCount, got %v", f)
		}
	})

	t.Run("multiple flags is an error", func(t *testing.T) {
		resetTest()
		cfgQuiet = true
		cfgCount = true
		_, err := resolveFormat()
		if err == nil {
			t.Fatal("expected error for multiple format flags")
		}
		if !strings.Contains(err.Error(), "only one") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("--json with another flag is an error", func(t *testing.T) {
		resetTest()
		cfgJSON = true
		cfgIDsOnly = true
		_, err := resolveFormat()
		if err == nil {
			t.Fatal("expected error for multiple format flags")
		}
	})
}

func TestFormatQuietOutput(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []any{
			map[string]any{"id": "1", "name": "Fixture 1"},
			map[string]any{"id": "2", "name": "Fixture 2"},
		},
	}

	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatQuiet)
	defer resetTest()

	err := fixtureListCmd.RunE(fixtureListCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := TestOutput()

	var data []map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("expected raw JSON array, got parse error: %v\noutput: %s", err, raw)
	}
	if len(data) != 2 {
		t.Errorf("expected 2 items, got %d", len(data))
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err == nil {
		if _, hasOK := envelope["ok"]; hasOK {
			t.Error("quiet format should not include envelope 'ok' key")
		}
	}
}

func TestFormatIDsOutput(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []map[string]any{
			{"id": "101", "name": "Fixture 1"},
			{"id": "202", "name": "Fixture 2"},
			{"id": "303", "name": "Fixture 3"},
		},
	}

	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatIDs)
	defer resetTest()

	err := fixtureListCmd.RunE(fixtureListCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := strings.TrimSpace(TestOutput())
	lines := strings.Split(raw, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), raw)
	}
	expected := []string{"101", "202", "303"}
	for i, want := range expected {
		if strings.TrimSpace(lines[i]) != want {
			t.Errorf("line %d: expected %q, got %q", i, want, lines[i])
		}
	}
}

func TestFormatCountOutput(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []any{
			map[string]any{"id": "1"},
			map[string]any{"id": "2"},
			map[string]any{"id": "3"},
			map[string]any{"id": "4"},
			map[string]any{"id": "5"},
		},
	}

	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatCount)
	defer resetTest()

	err := fixtureListCmd.RunE(fixtureListCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := strings.TrimSpace(TestOutput())
	if raw != "5" {
		t.Errorf("expected count '5', got %q", raw)
	}
}

func TestFormatCountSingleObject(t *testing.T) {
	mock := NewMockClient()
	mock.GetResponse = &APIResponse{
		StatusCode: 200,
		Data:       map[string]any{"id": "1", "name": "Fixture 1"},
	}

	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatCount)
	defer resetTest()

	err := fixtureShowCmd.RunE(fixtureShowCmd, []string{"1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := strings.TrimSpace(TestOutput())
	if raw != "1" {
		t.Errorf("expected count '1' for single object, got %q", raw)
	}
}

func TestFormatJSONEnvelope(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []any{
			map[string]any{"id": "1", "name": "Fixture 1"},
		},
	}

	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	err := fixtureListCmd.RunE(fixtureListCmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := TestOutput()
	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("expected JSON object, got parse error: %v", err)
	}
	if envelope["ok"] != true {
		t.Error("expected ok: true in JSON envelope")
	}
	if _, hasData := envelope["data"]; !hasData {
		t.Error("expected data key in JSON envelope")
	}
	if envelope["summary"] != "1 fixtures" {
		t.Errorf("expected summary '1 fixtures', got %v", envelope["summary"])
	}
}

func TestVersionJSONOutput(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("version", "--json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope map[string]any
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatalf("expected JSON object, got parse error: %v\noutput: %s", err, raw)
	}
	if envelope["ok"] != true {
		t.Errorf("expected ok=true, got %v", envelope["ok"])
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", envelope["data"])
	}
	if data["version"] != rootCmd.Version {
		t.Errorf("expected version %q, got %v", rootCmd.Version, data["version"])
	}
}

func TestVersionQuietOutput(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("version", "--quiet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("expected JSON object, got parse error: %v\noutput: %s", err, raw)
	}
	if _, hasOK := data["ok"]; hasOK {
		t.Error("quiet output should not include envelope")
	}
	if data["version"] != rootCmd.Version {
		t.Errorf("expected version %q, got %v", rootCmd.Version, data["version"])
	}
}

func TestPrettyFlagRemoved(t *testing.T) {
	flag := rootCmd.PersistentFlags().Lookup("pretty")
	if flag != nil {
		t.Error("--pretty flag should be removed")
	}
}

func TestFormatFlagsRegistered(t *testing.T) {
	flags := []string{"json", "quiet", "ids-only", "count"}
	for _, name := range flags {
		if rootCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected --%s flag to be registered", name)
		}
	}
}

// Integration tests: exercise full Cobra arg parsing → PersistentPreRunE → command output.

func TestBareRootNonTTYReturnsQuickStartJSON(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("expected JSON object, got parse error: %v\noutput: %s", err, raw)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok response, got %#v", resp)
	}
	if _, ok := resp["summary"].(string); !ok {
		t.Fatalf("expected summary string, got %#v", resp["summary"])
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %#v", resp["data"])
	}
	if data["version"] == nil {
		t.Fatalf("expected version in quick start response, got %#v", data)
	}
	if _, ok := data["commands"].(map[string]any); !ok {
		t.Fatalf("expected commands block, got %#v", data["commands"])
	}
}

func TestCobraFormatCount(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []map[string]any{
			{"id": "1", "name": "A"},
			{"id": "2", "name": "B"},
		},
	}
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("fixture", "list", "--count")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(raw) != "2" {
		t.Errorf("expected '2', got %q", raw)
	}
}

func TestCobraFormatQuiet(t *testing.T) {
	mock := NewMockClient()
	mock.GetResponse = &APIResponse{
		StatusCode: 200,
		Data:       map[string]any{"id": "42", "name": "Test Fixture"},
	}
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("fixture", "show", "42", "--quiet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("expected JSON object, got parse error: %v\noutput: %s", err, raw)
	}
	if _, hasOK := data["ok"]; hasOK {
		t.Error("quiet format should not include envelope 'ok' key")
	}
	if data["name"] != "Test Fixture" {
		t.Errorf("expected name 'Test Fixture', got %v", data["name"])
	}
}

func TestCobraFormatIDsOnly(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		Data: []map[string]any{
			{"id": "10", "name": "A"},
			{"id": "20", "name": "B"},
		},
	}
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("fixture", "list", "--ids-only")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	if len(lines) != 2 || lines[0] != "10" || lines[1] != "20" {
		t.Errorf("expected '10\\n20', got %q", raw)
	}
}

func TestCobraMutualExclusion(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("fixture", "list", "--quiet", "--count")
	if err == nil {
		t.Fatal("expected error for conflicting format flags")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Errorf("unexpected error message: %v", err)
	}
	assertExitCode(t, err, 1)
}

func TestResolveFormatAgent(t *testing.T) {
	defer resetTest()

	t.Run("--agent defaults to Quiet", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatQuiet {
			t.Errorf("expected FormatQuiet, got %v", f)
		}
	})

	t.Run("--agent --json resolves to JSON", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		cfgJSON = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatJSON {
			t.Errorf("expected FormatJSON, got %v", f)
		}
	})

	t.Run("--agent --styled is an error", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		cfgStyled = true
		_, err := resolveFormat()
		if err == nil {
			t.Fatal("expected error for --agent --styled")
		}
		if !strings.Contains(err.Error(), "cannot be used together") {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("--agent --markdown resolves to Markdown", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		cfgMarkdown = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatMarkdown {
			t.Errorf("expected FormatMarkdown, got %v", f)
		}
	})

	t.Run("--agent --ids-only resolves to IDs", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		cfgIDsOnly = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatIDs {
			t.Errorf("expected FormatIDs, got %v", f)
		}
	})

	t.Run("--agent --count resolves to Count", func(t *testing.T) {
		resetTest()
		cfgAgent = true
		cfgCount = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatCount {
			t.Errorf("expected FormatCount, got %v", f)
		}
	})
}

func TestResolveFormatStyledMarkdown(t *testing.T) {
	defer resetTest()

	t.Run("--styled resolves to Styled", func(t *testing.T) {
		resetTest()
		cfgStyled = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatStyled {
			t.Errorf("expected FormatStyled, got %v", f)
		}
	})

	t.Run("--markdown resolves to Markdown", func(t *testing.T) {
		resetTest()
		cfgMarkdown = true
		f, err := resolveFormat()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f != output.FormatMarkdown {
			t.Errorf("expected FormatMarkdown, got %v", f)
		}
	})

	t.Run("--styled --json is an error", func(t *testing.T) {
		resetTest()
		cfgStyled = true
		cfgJSON = true
		_, err := resolveFormat()
		if err == nil {
			t.Fatal("expected error for --styled --json")
		}
	})

	t.Run("--markdown --quiet is an error", func(t *testing.T) {
		resetTest()
		cfgMarkdown = true
		cfgQuiet = true
		_, err := resolveFormat()
		if err == nil {
			t.Fatal("expected error for --markdown --quiet")
		}
	})
}

func TestFormatFlagsNewRegistered(t *testing.T) {
	flags := []string{"agent", "styled", "markdown"}
	for _, name := range flags {
		if rootCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected --%s flag to be registered", name)
		}
	}
}

func TestSetupBlockedInMachineMode(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func()
	}{
		{"agent", func() { cfgAgent = true }},
		{"json", func() { cfgJSON = true }},
		{"quiet", func() { cfgQuiet = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer resetTest()
			resetTest()
			tc.set()
			err := runSetup(setupCmd, nil)
			if err == nil {
				t.Fatal("expected error when running setup in machine mode")
			}
			if !strings.Contains(err.Error(), "interactive terminal") {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSkillPrintsInMachineMode(t *testing.T) {
	defer resetTest()
	resetTest()
	cfgAgent = true

	var buf bytes.Buffer
	skillCmd.SetOut(&buf)
	defer skillCmd.SetOut(nil)

	err := runSkill(skillCmd, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected skill content to be printed in machine mode")
	}
}

// newFormatProfileStore saves one profile per name, for a non-paginated list.
func newFormatProfileStore(t *testing.T, names ...string) *profile.Store {
	t.Helper()
	store := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	for _, name := range names {
		if err := store.Create(&profile.Profile{Name: name, BaseURL: "https://mailpiece.example.com"}); err != nil {
			t.Fatalf("create profile %s: %v", name, err)
		}
	}
	return store
}

func TestLimitFlag(t *testing.T) {
	t.Run("--limit truncates paginated list", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data: []any{
				map[string]any{"id": "1", "name": "Fixture 1"},
				map[string]any{"id": "2", "name": "Fixture 2"},
				map[string]any{"id": "3", "name": "Fixture 3"},
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgLimit = 2
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Response == nil {
			t.Fatal("expected response")
		}
		data, ok := result.Response.Data.([]any)
		if !ok {
			t.Fatalf("expected []any data, got %T", result.Response.Data)
		}
		if len(data) != 2 {
			t.Errorf("expected 2 items after --limit, got %d", len(data))
		}
	})

	t.Run("--limit truncates non-paginated list", func(t *testing.T) {
		result := SetTestMode()
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		SetTestProfiles(newFormatProfileStore(t, "a1", "b2", "c3"))
		defer resetTest()

		cfgLimit = 1
		err := authListCmd.RunE(authListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, ok := result.Response.Data.([]any)
		if !ok {
			t.Fatalf("expected []any data, got %T", result.Response.Data)
		}
		if len(data) != 1 {
			t.Errorf("expected 1 item after --limit, got %d", len(data))
		}
	})

	t.Run("--limit with notice for non-paginated", func(t *testing.T) {
		result := SetTestMode()
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		SetTestProfiles(newFormatProfileStore(t, "a1", "b2", "c3"))
		defer resetTest()

		cfgLimit = 2
		err := authListCmd.RunE(authListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Response.Notice != "Showing 2 of 3 results" {
			t.Errorf("expected truncation notice, got %q", result.Response.Notice)
		}
	})

	t.Run("--limit with notice for paginated", func(t *testing.T) {
		mock := NewMockClient()
		items := make([]any, 20)
		for i := range items {
			items[i] = map[string]any{"id": string(rune('A' + i)), "name": "Fixture"}
		}
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data:       items,
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgLimit = 5
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Response.Notice != "Showing 5 of 20 results" {
			t.Errorf("expected truncation notice, got %q", result.Response.Notice)
		}
	})

	t.Run("no notice when limit not reached", func(t *testing.T) {
		result := SetTestMode()
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		SetTestProfiles(newFormatProfileStore(t, "a1"))
		defer resetTest()

		cfgLimit = 10
		err := authListCmd.RunE(authListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Response.Notice != "" {
			t.Errorf("expected no notice when data fits within limit, got %q", result.Response.Notice)
		}
	})

	t.Run("--limit and --all is an error", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgLimit = 5
		fixtureListPages.all = true
		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		fixtureListPages.all = false
		if err == nil {
			t.Fatal("expected error for --limit with --all")
		}
		if !strings.Contains(err.Error(), "cannot be used together") {
			t.Errorf("unexpected error message: %v", err)
		}
		assertExitCode(t, err, 1)
	})

	t.Run("no limit means no truncation", func(t *testing.T) {
		mock := NewMockClient()
		mock.GetWithPaginationResponse = &APIResponse{
			StatusCode: 200,
			Data: []any{
				map[string]any{"id": "1"},
				map[string]any{"id": "2"},
				map[string]any{"id": "3"},
			},
		}

		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := fixtureListCmd.RunE(fixtureListCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		data, ok := result.Response.Data.([]any)
		if !ok {
			t.Fatalf("expected []any data, got %T", result.Response.Data)
		}
		if len(data) != 3 {
			t.Errorf("expected 3 items with no limit, got %d", len(data))
		}
	})
}

func TestPaginationNoticeFollowsNextLink(t *testing.T) {
	mock := NewMockClient()
	mock.GetWithPaginationResponse = &APIResponse{
		StatusCode: 200,
		LinkNext:   "/acme/fixtures.json?page=2",
		Data: []any{
			map[string]any{"id": "1", "name": "Fixture 1"},
			map[string]any{"id": "2", "name": "Fixture 2"},
		},
	}
	result := SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	if err := fixtureListCmd.RunE(fixtureListCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := "Showing 2 results: there are more (use --page, or --all for the complete list)"; result.Response.Notice != want {
		t.Errorf("notice: want %q, got %q", want, result.Response.Notice)
	}
	pagination, ok := result.Response.Context["pagination"].(map[string]any)
	if !ok || pagination["has_next"] != true {
		t.Errorf("expected pagination context with has_next, got %#v", result.Response.Context)
	}
	var next string
	for _, crumb := range result.Response.Breadcrumbs {
		if crumb.Action == "next" {
			next = crumb.Cmd
		}
	}
	if next != "mailpiece fixture list --page 2" {
		t.Errorf("next breadcrumb: want 'mailpiece fixture list --page 2', got %q", next)
	}
}

func TestPaginationWithoutNextLinkHasNoNotice(t *testing.T) {
	mock := NewMockClient()
	mock.WithListData([]any{map[string]any{"id": "1", "name": "Fixture 1"}})
	result := SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	if err := fixtureListCmd.RunE(fixtureListCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Response.Notice != "" {
		t.Errorf("expected no notice, got %q", result.Response.Notice)
	}
	if _, ok := result.Response.Context["pagination"]; ok {
		t.Errorf("expected no pagination context, got %#v", result.Response.Context)
	}
}

func TestStyledListRendersBreadcrumbsAsNextSteps(t *testing.T) {
	mock := NewMockClient()
	mock.WithListData([]any{map[string]any{"id": "s1", "name": "North yard"}})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	if err := fixtureListCmd.RunE(fixtureListCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := TestOutput()
	if !strings.Contains(raw, "North yard") || !strings.Contains(raw, "Next steps:") || !strings.Contains(raw, "mailpiece fixture show ID  # View a fixture") {
		t.Fatalf("expected styled list with next steps, got:\n%s", raw)
	}
}

func TestMarkdownListRendersBreadcrumbsAsNextSteps(t *testing.T) {
	mock := NewMockClient()
	mock.WithListData([]any{map[string]any{"id": "s1", "name": "North yard"}})
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	SetTestFormat(output.FormatMarkdown)
	defer resetTest()

	if err := fixtureListCmd.RunE(fixtureListCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := TestOutput()
	if !strings.Contains(raw, "### Next steps") || !strings.Contains(raw, "- `mailpiece fixture show ID`: View a fixture") {
		t.Fatalf("expected markdown next steps, got:\n%s", raw)
	}
}

func TestLimitFlagRegistered(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("limit") == nil {
		t.Error("expected --limit flag to be registered")
	}
}

func TestCheckLimitAll(t *testing.T) {
	t.Run("no conflict when limit is 0", func(t *testing.T) {
		cfgLimit = 0
		if err := checkLimitAll(true); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("no conflict when all is false", func(t *testing.T) {
		cfgLimit = 5
		if err := checkLimitAll(false); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		cfgLimit = 0
	})

	t.Run("conflict when both set", func(t *testing.T) {
		cfgLimit = 5
		err := checkLimitAll(true)
		cfgLimit = 0
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAppendHumanSectionsLocation(t *testing.T) {
	styled := appendHumanSections("body", "", "/acme/fixtures/s1.json", nil, false)
	if !strings.Contains(styled, "Location: /acme/fixtures/s1.json") {
		t.Errorf("expected styled location, got %q", styled)
	}
	markdown := appendHumanSections("body", "", "/acme/fixtures/s1.json", nil, true)
	if !strings.Contains(markdown, "**Location:** `/acme/fixtures/s1.json`") {
		t.Errorf("expected markdown location, got %q", markdown)
	}
}

func TestTruncateDataHandlesTypedSlices(t *testing.T) {
	defer func() { cfgLimit = 0 }()
	cfgLimit = 2

	data, count := truncateData([]map[string]any{{"id": "1"}, {"id": "2"}, {"id": "3"}})
	if count != 3 {
		t.Errorf("original count: want 3, got %d", count)
	}
	if got := dataCount(data); got != 2 {
		t.Errorf("truncated count: want 2, got %d", got)
	}
	if _, count := truncateData(map[string]any{"id": "1"}); count != 0 {
		t.Errorf("a single object has no count, got %d", count)
	}
}
