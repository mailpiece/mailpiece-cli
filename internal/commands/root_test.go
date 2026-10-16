package commands

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestRequireAccountTakesTheTokensOnlyAccount(t *testing.T) {
	tests := []struct {
		name     string
		accounts []any
		exit     int
		account  string
	}{
		{name: "one account is taken", accounts: []any{map[string]any{"id": "a1", "name": "Acme", "slug": "/834206594"}}, exit: 0, account: "834206594"},
		{name: "no account is forbidden", accounts: []any{}, exit: 4},
		{name: "several accounts are ambiguous", accounts: []any{
			map[string]any{"id": "a1", "name": "Acme", "slug": "/834206594"},
			map[string]any{"id": "a2", "name": "Globex", "slug": "/123456789"},
		}, exit: 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockClient()
			mock.OnGet("/my/identity.json", &APIResponse{StatusCode: 200, Data: map[string]any{"id": "i1", "accounts": tt.accounts}})
			SetTestModeWithSDK(mock)
			SetTestConfig("token", "", "https://mailpiece.example.com")
			defer resetTest()

			err := requireAccount()
			assertExitCode(t, err, tt.exit)
			if cfg.Account != tt.account {
				t.Fatalf("account: want %q, got %q", tt.account, cfg.Account)
			}
			if tt.exit == 0 && accountSource != "identity" {
				t.Fatalf("account source: want identity, got %q", accountSource)
			}
			if tt.exit == 8 {
				e := output.AsError(err)
				if !strings.Contains(e.Hint, "834206594 (Acme)") || !strings.Contains(e.Hint, "123456789 (Globex)") {
					t.Fatalf("expected both accounts in the hint, got %q", e.Hint)
				}
			}
		})
	}
}

func TestRequireAuthWithoutToken(t *testing.T) {
	SetTestMode()
	SetTestConfig("", "acme", "https://mailpiece.example.com")
	defer resetTest()

	assertExitCode(t, requireAuth(), 3)
}

func TestInvalidAccountFlagIsAUsageError(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("fixture", "list", "--account", "../other")
	assertExitCode(t, err, 1)
	if len(mock.GetWithPaginationCalls) != 0 {
		t.Fatalf("expected no request, got %d", len(mock.GetWithPaginationCalls))
	}
}

func TestTakeAccountFromAddress(t *testing.T) {
	tests := []struct {
		name          string
		account       string
		apiURL        string
		wantAccount   string
		wantAPIURL    string
		wantSource    string
		wantErrNaming []string
	}{
		{name: "the number in the address is the account", apiURL: "https://mail.example.com/834206594", wantAccount: "834206594", wantAPIURL: "https://mail.example.com", wantSource: "address"},
		{name: "a handle in the address is the account", apiURL: "https://mail.example.com/my-newsletter/", wantAccount: "my-newsletter", wantAPIURL: "https://mail.example.com", wantSource: "address"},
		{name: "the same account set twice is fine", account: "834206594", apiURL: "https://mail.example.com/834206594", wantAccount: "834206594", wantAPIURL: "https://mail.example.com", wantSource: "flag"},
		{name: "another account set is refused", account: "acme", apiURL: "https://mail.example.com/834206594", wantAccount: "acme", wantAPIURL: "https://mail.example.com", wantSource: "flag", wantErrNaming: []string{"https://mail.example.com/834206594", "834206594", "acme"}},
		{name: "a trailing slash is no path", apiURL: "https://mail.example.com/", wantAPIURL: "https://mail.example.com/"},
		{name: "a bare address is left alone", account: "acme", apiURL: "https://mail.example.com", wantAccount: "acme", wantAPIURL: "https://mail.example.com", wantSource: "flag"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetTestMode()
			SetTestConfig("token", tt.account, tt.apiURL)
			defer resetTest()
			if tt.account != "" {
				accountSource = "flag"
			}

			err := takeAccountFromAddress()
			if len(tt.wantErrNaming) == 0 {
				assertExitCode(t, err, 0)
			} else {
				assertExitCode(t, err, 1)
				for _, word := range tt.wantErrNaming {
					if !strings.Contains(output.AsError(err).Message, word) {
						t.Errorf("expected the error to name %q, got %q", word, output.AsError(err).Message)
					}
				}
			}
			if cfg.Account != tt.wantAccount || cfg.APIURL != tt.wantAPIURL || accountSource != tt.wantSource {
				t.Errorf("got account %q at %q from %q; want %q at %q from %q", cfg.Account, cfg.APIURL, accountSource, tt.wantAccount, tt.wantAPIURL, tt.wantSource)
			}
			if configuredAPIURL != tt.apiURL {
				t.Errorf("configured address: want %q, got %q", tt.apiURL, configuredAPIURL)
			}
		})
	}
}

func TestAddressWithAccountIsShownResolved(t *testing.T) {
	SetTestMode()
	SetTestConfig("token", "", "https://mail.example.com/834206594")
	defer resetTest()
	assertExitCode(t, takeAccountFromAddress(), 0)

	show := configShowData()
	if show["api_url"] != "https://mail.example.com" || show["api_url_configured"] != "https://mail.example.com/834206594" {
		t.Errorf("config show: got api_url %v (configured %v)", show["api_url"], show["api_url_configured"])
	}
	if show["account"] != "834206594" || show["account_source"] != "address" {
		t.Errorf("config show: got account %v from %v", show["account"], show["account_source"])
	}

	account := configExplainData()["account"].(configExplainField)
	if account.Value != "834206594" || account.Source != "API URL https://mail.example.com/834206594" {
		t.Errorf("config explain: got account %v from %q", account.Value, account.Source)
	}

	eff := resolveDoctorEffectiveConfig()
	if eff.APIURL != "https://mail.example.com" || eff.Account != "834206594" || eff.AccountSource != "API URL https://mail.example.com/834206594" {
		t.Errorf("doctor: got %q at %q from %q", eff.Account, eff.APIURL, eff.AccountSource)
	}
}

func TestRequireAccountSaysWhatFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(`{"status":406,"error":"Not Acceptable"}`))
	}))
	defer server.Close()

	SetTestMode()
	SetTestSDK(server.URL)
	SetTestConfig("secret-token", "", server.URL)
	defer resetTest()

	err := requireAccount()
	assertExitCode(t, err, 7)
	e := output.AsError(err)
	if e.Message != "Could not find the token's account: Not Acceptable" {
		t.Errorf("message = %q", e.Message)
	}
	want := "The API answered HTTP 406 to GET " + server.URL + "/my/identity.json. Check the Mailpiece address and the token."
	if e.Hint != want {
		t.Errorf("hint = %q, want %q", e.Hint, want)
	}
	if strings.Contains(e.Message+e.Hint, "secret-token") || strings.Contains(e.Message+e.Hint, "test-token") {
		t.Errorf("the error prints the token: %q / %q", e.Message, e.Hint)
	}
}

func TestRequireAccountHintsAtTheAddressWhenTheAPIGivesNone(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	SetTestMode()
	SetTestSDK(server.URL)
	SetTestConfig("secret-token", "", server.URL)
	defer resetTest()

	err := requireAccount()
	assertExitCode(t, err, 2)
	e := output.AsError(err)
	if !strings.HasPrefix(e.Message, "Could not find the token's account: ") {
		t.Errorf("message = %q", e.Message)
	}
	if e.Hint != "Check the Mailpiece address ("+server.URL+") and the token" {
		t.Errorf("hint = %q", e.Hint)
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

func TestVerboseCounts(t *testing.T) {
	defer ResetTestMode()

	tests := []struct {
		args []string
		want int
	}{
		{[]string{"--verbose"}, 1},
		{[]string{"-v"}, 1},
		{[]string{"-vv"}, 2},
		{[]string{"-v", "--verbose"}, 2},
	}
	for _, tt := range tests {
		cfgVerbose = 0
		if err := rootCmd.PersistentFlags().Parse(tt.args); err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if cfgVerbose != tt.want {
			t.Errorf("%v: verbose = %d, want %d", tt.args, cfgVerbose, tt.want)
		}
	}
}
