package commands

import (
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/spf13/cobra"
)

func TestCommandsStyledOutputRendersHumanCatalog(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := TestOutput()
	for _, heading := range []string{"CORE COMMANDS", "ACCOUNT & ADMIN", "SETUP & TOOLS"} {
		if !strings.Contains(raw, heading) {
			t.Fatalf("expected styled catalog heading %q, got:\n%s", heading, raw)
		}
	}
	if !strings.Contains(raw, "auth") || !strings.Contains(raw, "show") || !strings.Contains(raw, "token") {
		t.Fatalf("expected styled catalog to include commands, got:\n%s", raw)
	}
	if strings.Contains(raw, "list, login, logout") {
		t.Fatalf("expected unfiltered styled catalog to omit action lists, got:\n%s", raw)
	}
}

func TestCommandsFilterRendersMatchingHumanCatalog(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{"auth"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := TestOutput()
	if !strings.Contains(raw, "auth") {
		t.Fatalf("expected filtered catalog to include auth, got:\n%s", raw)
	}
	if strings.Contains(raw, "completion") {
		t.Fatalf("expected filtered catalog to omit non-matching completion command, got:\n%s", raw)
	}
	if !strings.Contains(raw, "email, list, login, logout, status, switch") {
		t.Fatalf("expected filtered catalog to include action list, got:\n%s", raw)
	}
}

func TestCommandsFilterFindsToken(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{"token"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	raw := TestOutput()
	if !strings.Contains(raw, "token") || !strings.Contains(raw, "create, delete") {
		t.Fatalf("expected filtered catalog to include token actions, got:\n%s", raw)
	}
	if strings.Contains(raw, "No commands match") {
		t.Fatalf("expected token to be discoverable, got:\n%s", raw)
	}
}

func TestCommandsFilterWithNoMatch(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{"zzqx"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if raw := TestOutput(); !strings.Contains(raw, `No commands match "zzqx"`) {
		t.Fatalf("expected no-match line, got:\n%s", raw)
	}
}

func TestCommandsJSONOutputReturnsStructuredCatalog(t *testing.T) {
	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Response == nil || !result.Response.OK {
		t.Fatalf("expected OK JSON response, got %#v", result.Response)
	}

	items, ok := result.Response.Data.([]any)
	if !ok {
		t.Fatalf("expected command catalog slice, got %#v", result.Response.Data)
	}
	if len(items) == 0 {
		t.Fatal("expected command catalog entries")
	}

	categories := map[string]any{}
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		categories[entry["name"].(string)] = entry["category"]
	}
	for name, want := range map[string]string{
		"mailpiece commands": "utilities",
		"mailpiece show":     "core",
		"mailpiece token":    "admin",
	} {
		if categories[name] != want {
			t.Fatalf("expected %s category %s, got %#v", name, want, categories[name])
		}
	}
}

func TestCommandsJSONFilterKeepsParentsOfMatches(t *testing.T) {
	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	defer resetTest()

	if err := commandsCmd.RunE(commandsCmd, []string{"personal access tokens, newest"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items := result.Response.Data.([]any)
	if len(items) != 1 {
		t.Fatalf("expected only the token command, got %#v", items)
	}
	token := items[0].(map[string]any)
	if token["name"] != "mailpiece token" {
		t.Fatalf("expected mailpiece token, got %#v", token["name"])
	}
	subs := token["subcommands"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["name"] != "mailpiece token list" {
		t.Fatalf("expected only list under token, got %#v", subs)
	}
}

func TestCommandGroupDefaultsToUtilities(t *testing.T) {
	tests := []struct {
		annotations map[string]string
		want        string
	}{
		{nil, "utilities"},
		{map[string]string{"group": "core"}, "core"},
		{map[string]string{"group": "admin"}, "admin"},
		{map[string]string{"group": "unknown"}, "utilities"},
	}
	for _, tt := range tests {
		cmd := &cobra.Command{Use: "x", Annotations: tt.annotations}
		if got := commandGroup(cmd); got != tt.want {
			t.Errorf("commandGroup(%v) = %q, want %q", tt.annotations, got, tt.want)
		}
	}
}
