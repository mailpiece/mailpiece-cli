package commands

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/spf13/cobra"
)

func TestUsageHelpCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want string
	}{
		{name: "nil falls back to root", cmd: "", want: "mailpiece --help"},
		{name: "subcommand uses command path", cmd: "auth login", want: "mailpiece auth login --help"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cmd *cobra.Command
			if tt.cmd != "" {
				parts := strings.Split(tt.cmd, " ")
				found, _, err := rootCmd.Find(parts)
				if err != nil {
					t.Fatalf("find command: %v", err)
				}
				cmd = found
			}

			if got := usageHelpCommand(cmd); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestPrintHumanErrorUsesCommandSpecificHelp(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
	}()

	cmd, _, err := rootCmd.Find([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("find command: %v", err)
	}

	printHumanError(cmd, &output.Error{Code: output.CodeUsage, Message: "accepts 1 arg(s), received 0"})
	_ = w.Close()

	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	out := string(body)
	if !strings.Contains(out, "Run `mailpiece auth login --help` for usage.") {
		t.Fatalf("expected command-specific usage hint, got:\n%s", out)
	}
	if strings.Contains(out, "Run `mailpiece --help` for usage.") {
		t.Fatalf("expected root usage hint to be omitted, got:\n%s", out)
	}
}

func TestRequireAccountWithoutAccount(t *testing.T) {
	SetTestMode()
	SetTestConfig("token", "", "https://mailpiece.example.com")
	defer resetTest()

	err := requireAccount()
	assertExitCode(t, err, 1)
	if want := "No account configured. Set --profile flag, MAILPIECE_PROFILE, or run 'mailpiece setup'"; output.AsError(err).Message != want {
		t.Errorf("message = %q, want %q", output.AsError(err).Message, want)
	}
}

func TestRequireAuthWithoutToken(t *testing.T) {
	SetTestMode()
	SetTestConfig("", "acme", "https://mailpiece.example.com")
	defer resetTest()

	assertExitCode(t, requireAuth(), 3)
}

func TestInvalidAccountIsAUsageError(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "../other", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("fixture", "list")
	assertExitCode(t, err, 1)
	if len(mock.GetWithPaginationCalls) != 0 {
		t.Fatalf("expected no request, got %d", len(mock.GetWithPaginationCalls))
	}
}

func TestUnknownSubcommandIsAUsageErrorWithSuggestions(t *testing.T) {
	configureCLIUX()
	SetTestMode()
	defer resetTest()

	rootCmd.SetArgs([]string{"post", "lsit"})
	defer rootCmd.SetArgs(nil)
	_, err := executeRoot()
	e := output.AsError(err)
	if err == nil || e.Code != output.CodeUsage || e.ExitCode() != 1 {
		t.Fatalf("expected a usage error exiting 1, got %v", err)
	}
	if e.Message != `unknown command "lsit" for "mailpiece post"` {
		t.Errorf("message = %q", e.Message)
	}
	if !strings.Contains(e.Hint, "Did you mean: list") {
		t.Errorf("expected a suggestion of list, got %q", e.Hint)
	}

	rootCmd.SetArgs([]string{"post", "zzzzzz"})
	_, err = executeRoot()
	if e := output.AsError(err); err == nil || e.Hint != "Run: mailpiece post --help" {
		t.Errorf("expected the help hint with nothing close, got %v", err)
	}
}

func TestBareGroupStillShowsHelp(t *testing.T) {
	configureCLIUX()
	SetTestMode()
	defer resetTest()

	for _, args := range [][]string{{"post"}, {"post", "--help"}} {
		rootCmd.SetArgs(args)
		if _, err := executeRoot(); err != nil {
			t.Errorf("%v: expected help and no error, got %v", args, err)
		}
	}
	rootCmd.SetArgs(nil)
}
