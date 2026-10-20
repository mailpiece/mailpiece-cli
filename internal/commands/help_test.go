package commands

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestRenderRootHelp(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(rootCmd, &buf)
	out := buf.String()

	for _, want := range []string{"CORE COMMANDS", "show", "ACCOUNT & ADMIN", "token", "GETTING STARTED", "DISCOVER", "doctor", "FLAGS", "--profile", "EXAMPLES", "LEARN MORE", "Use `mailpiece commands` to see the full command catalog.", "implies --json"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected root help to contain %q, got:\n%s", want, out)
		}
	}
}

func TestRenderRootHelpGroupsCommands(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(rootCmd, &buf)
	out := buf.String()

	core := strings.Index(out, "CORE COMMANDS")
	admin := strings.Index(out, "ACCOUNT & ADMIN")
	tools := strings.Index(out, "GETTING STARTED")
	if core < 0 || admin < core || tools < admin {
		t.Fatalf("expected core, admin, then getting started groups, got:\n%s", out)
	}
	if section := out[core:admin]; !strings.Contains(section, "  show") || strings.Contains(section, "  token ") {
		t.Fatalf("expected show under core and token elsewhere, got:\n%s", section)
	}
	if section := out[admin:tools]; !strings.Contains(section, "  token ") {
		t.Fatalf("expected token under account & admin, got:\n%s", section)
	}
}

func TestRenderSubcommandHelpIncludesAliasesAndExamples(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(authListCmd, &buf)
	out := buf.String()

	if !strings.Contains(out, "ALIASES") || !strings.Contains(out, "ls") {
		t.Fatalf("expected alias section in help, got:\n%s", out)
	}
	if !strings.Contains(out, "EXAMPLES") {
		t.Fatalf("expected examples section in help, got:\n%s", out)
	}
}

func TestRenderSubcommandHelpIncludesSeeAlso(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(configShowCmd, &buf)
	out := buf.String()

	if !strings.Contains(out, "SEE ALSO") || !strings.Contains(out, "mailpiece config explain") {
		t.Fatalf("expected see also section in help, got:\n%s", out)
	}
	if !strings.Contains(out, "Use `mailpiece config show --help` for more information about this command.") {
		t.Fatalf("expected learn more line, got:\n%s", out)
	}
}

func TestRenderRootHelpOmitsCommonWorkflows(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(rootCmd, &buf)
	out := buf.String()

	if strings.Contains(out, "COMMON WORKFLOWS") {
		t.Fatalf("expected root help to omit common workflows, got:\n%s", out)
	}
}

func TestRenderRunnableParentCommandHelpPreservesDirectUsage(t *testing.T) {
	configureCLIUX()

	tests := []struct {
		name string
		cmd  *cobra.Command
		want string
	}{
		{name: "setup", cmd: setupCmd, want: "  mailpiece setup [flags]"},
		{name: "skill", cmd: skillCmd, want: "  mailpiece skill [flags]"},
		{name: "not runnable", cmd: tokenCmd, want: "  mailpiece token <command> [flags]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderHelp(tt.cmd, &buf)
			out := buf.String()

			if !strings.Contains(out, tt.want) {
				t.Fatalf("expected help to contain %q, got:\n%s", tt.want, out)
			}
		})
	}
}

func TestRenderCommandsHelpMentionsJSONCatalog(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(commandsCmd, &buf)
	out := buf.String()

	if !strings.Contains(out, "Use --json for a structured command catalog.") {
		t.Fatalf("expected commands help to mention --json catalog, got:\n%s", out)
	}
	if !strings.Contains(out, "EXAMPLES") || !strings.Contains(out, "$ mailpiece commands --json") {
		t.Fatalf("expected commands help examples to include --json, got:\n%s", out)
	}
}

func TestRenderSubcommandHelpDoesNotRepeatJQFlag(t *testing.T) {
	configureCLIUX()

	var buf bytes.Buffer
	renderHelp(tokenListCmd, &buf)
	out := buf.String()

	if strings.Contains(out, "--jq") {
		t.Fatalf("expected subcommand help to omit --jq, got:\n%s", out)
	}
}

func TestGenericAliases(t *testing.T) {
	configureCLIUX()

	tests := []struct {
		cmd   *cobra.Command
		alias string
	}{
		{tokenListCmd, "ls"},
		{authListCmd, "ls"},
		{identityShowCmd, "view"},
		{tokenDeleteCmd, "rm"},
	}
	for _, tt := range tests {
		found := false
		for _, alias := range tt.cmd.Aliases {
			if alias == tt.alias {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: expected alias %q, got %v", tt.cmd.CommandPath(), tt.alias, tt.cmd.Aliases)
		}
	}
}

func TestAppendAliasIsIdempotent(t *testing.T) {
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}}
	applyGenericAliases(cmd)
	applyGenericAliases(cmd)
	if len(cmd.Aliases) != 1 {
		t.Fatalf("expected one alias, got %v", cmd.Aliases)
	}
}

func TestAliasesRunTheirCommands(t *testing.T) {
	configureCLIUX()

	t.Run("ls lists", func(t *testing.T) {
		mock := NewMockClient()
		mock.OnGet("/my/access_tokens.json", &APIResponse{StatusCode: 200, Data: []any{map[string]any{"id": "tok1", "description": "Laptop"}}})
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		_, err := runCobraWithArgs("token", "ls")
		assertExitCode(t, err, 0)
		if lastResult.Response == nil || lastResult.Response.Summary != "1 access tokens" {
			t.Fatalf("expected the token list, got %#v", lastResult.Response)
		}
	})

	t.Run("view shows", func(t *testing.T) {
		mock := NewMockClient()
		mock.OnGet("/my/identity.json", &APIResponse{StatusCode: 200, Data: map[string]any{"id": "i1", "accounts": []any{}}})
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		_, err := runCobraWithArgs("identity", "view")
		assertExitCode(t, err, 0)
		data := responseDataMap(t, lastResult)
		if data["id"] != "i1" {
			t.Fatalf("expected the identity, got %#v", data)
		}
	})

	t.Run("rm deletes", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		_, err := runCobraWithArgs("token", "rm", "tok1")
		assertExitCode(t, err, 0)
		if len(mock.DeleteCalls) != 1 || mock.DeleteCalls[0].Path != "/my/access_tokens/tok1.json" {
			t.Fatalf("expected DELETE /my/access_tokens/tok1.json, got %#v", mock.DeleteCalls)
		}
	})
}

func TestAgentHelpIsJSON(t *testing.T) {
	SetTestMode()
	defer resetTest()

	var buf bytes.Buffer
	oldWriter := outWriter
	outWriter = &buf
	defer func() { outWriter = oldWriter }()
	agentHelp(tokenListCmd, nil)

	var info map[string]any
	if err := json.Unmarshal(buf.Bytes(), &info); err != nil {
		t.Fatalf("expected JSON help, got %v:\n%s", err, buf.String())
	}
	if info["name"] != "mailpiece token list" {
		t.Fatalf("expected command path, got %#v", info["name"])
	}
	names := map[string]bool{}
	for _, f := range info["flags"].([]any) {
		names[f.(map[string]any)["name"].(string)] = true
	}
	for _, want := range []string{"--json", "--jq", "--profile"} {
		if !names[want] {
			t.Errorf("expected flag %s in agent help, got %v", want, names)
		}
	}
}
