package commands

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
)

// resetSignupFlags clears cobra flag values between test runs to prevent cross-contamination.
func resetSignupFlags() {
	signupStartCmd.Flags().Set("email", "")
	signupVerifyCmd.Flags().Set("code", "")
	signupVerifyCmd.Flags().Set("pending-token", "")
	signupCompleteCmd.Flags().Set("name", "")
	signupCompleteCmd.Flags().Set("account", "")
	signupCompleteCmd.Flags().Set("write", "false")
}

// setSignupSaveStores points saved tokens and profiles at temporary stores.
func setSignupSaveStores(t *testing.T) {
	t.Helper()
	config.SetTestConfigDir(t.TempDir())
	t.Cleanup(config.ResetTestConfigDir)
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	SetTestCreds(newTestCredStore(t, t.TempDir()))
	SetTestProfiles(profileStore)
}

func TestSignupStart(t *testing.T) {
	server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
	defer server.Close()

	t.Run("returns signed cookie value and points to signup verify", func(t *testing.T) {
		resetSignupFlags()
		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		signupStartCmd.Flags().Set("email", "test@example.com")
		err := signupStartCmd.RunE(signupStartCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["pending_authentication_token"] != "signed-pending-token-value" {
			t.Errorf("expected signed cookie value, got %v", data["pending_authentication_token"])
		}
		if data["code"] != "TEST01" {
			t.Errorf("expected code 'TEST01', got %v", data["code"])
		}

		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || !strings.HasPrefix(crumbs[0].Cmd, "mailpiece signup verify ") {
			t.Errorf("expected the signup verify breadcrumb, got %#v", crumbs)
		}
	})
}

func TestSignupVerify(t *testing.T) {
	t.Run("returns the session cookie and the accounts of an existing user", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_test",
			accounts: []any{
				map[string]any{"id": "acct-1", "name": "Test Account", "slug": "/123456"},
				map[string]any{"id": "acct-2", "name": "Other Account", "slug": "/789012"},
			},
		})
		defer server.Close()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		signupVerifyCmd.Flags().Set("code", "VALID1")
		signupVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := signupVerifyCmd.RunE(signupVerifyCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["session_token"] != "signed-session-token-value" {
			t.Errorf("expected signed session cookie, got %v", data["session_token"])
		}
		if data["requires_signup_completion"] != false {
			t.Errorf("expected requires_signup_completion=false, got %v", data["requires_signup_completion"])
		}

		accounts, _ := data["accounts"].([]any)
		if len(accounts) != 2 {
			t.Fatalf("expected 2 accounts, got %#v", data["accounts"])
		}
		for _, acc := range accounts {
			if slug := acc.(map[string]any)["slug"].(string); strings.HasPrefix(slug, "/") {
				t.Errorf("expected slug without leading slash, got %q", slug)
			}
		}

		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || crumbs[0].Cmd != "mailpiece signup complete --account <SLUG>" {
			t.Errorf("expected the signup complete --account breadcrumb, got %#v", crumbs)
		}
	})

	t.Run("points a new user to signup complete --name", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			requiresCompletion: true,
			accessToken:        "mailpiece_test",
		})
		defer server.Close()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		signupVerifyCmd.Flags().Set("code", "VALID1")
		signupVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := signupVerifyCmd.RunE(signupVerifyCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["requires_signup_completion"] != true {
			t.Errorf("expected requires_signup_completion=true, got %v", data["requires_signup_completion"])
		}
		if _, ok := data["accounts"]; ok {
			t.Error("expected no accounts for a new user")
		}
		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || crumbs[0].Cmd != `mailpiece signup complete --name "<NAME>"` {
			t.Errorf("expected the signup complete --name breadcrumb, got %#v", crumbs)
		}
	})
}

func TestSignupComplete(t *testing.T) {
	t.Run("new user completes signup and generates token", func(t *testing.T) {
		resetSignupFlags()
		signups := &tokenRequests{}
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			requiresCompletion: true,
			accessToken:        "mailpiece_new_user_token",
			signups:            signups,
			requests:           requests,
		})
		defer server.Close()

		result := SetTestMode()
		setSignupSaveStores(t)
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		signupCompleteCmd.Flags().Set("name", "New User")
		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		if len(signups.bodies) != 1 {
			t.Fatalf("expected one signup completion, got %d", len(signups.bodies))
		}
		signup, _ := signups.bodies[0]["signup"].(map[string]any)
		if signup["full_name"] != "New User" {
			t.Errorf("unexpected signup completion body: %#v", signups.bodies[0])
		}
		if len(requests.paths) != 1 || requests.paths[0] != "/123456/my/access_tokens.json" {
			t.Errorf("expected a token request for the new account, got %v", requests.paths)
		}

		data := result.Response.Data.(map[string]any)
		if data["account"] != "123456" {
			t.Errorf("expected account '123456', got %v", data["account"])
		}
		if data["is_new_user"] != true {
			t.Errorf("expected is_new_user=true, got %v", data["is_new_user"])
		}
		if _, ok := data["token"]; ok {
			t.Error("the generated token must not be printed")
		}

		loaded, err := credsLoadProfileToken("123456")
		if err != nil || loaded != "mailpiece_new_user_token" {
			t.Fatalf("expected the generated token under profile 123456, got %q (%v)", loaded, err)
		}
	})

	t.Run("existing user generates token for the account", func(t *testing.T) {
		resetSignupFlags()
		signups := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_generated_token",
			signups:     signups,
		})
		defer server.Close()

		result := SetTestMode()
		setSignupSaveStores(t)
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		signupCompleteCmd.Flags().Set("account", "/123456")
		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		if len(signups.bodies) != 0 {
			t.Error("an existing user must not complete a signup")
		}
		data := result.Response.Data.(map[string]any)
		if data["account"] != "123456" {
			t.Errorf("expected normalized account '123456', got %v", data["account"])
		}
		if _, ok := data["is_new_user"]; ok {
			t.Error("expected no is_new_user for an existing user")
		}
		if data["permission"] != "read" {
			t.Errorf("expected permission 'read', got %v", data["permission"])
		}
	})

	t.Run("shows the app's refusal of a name", func(t *testing.T) {
		resetSignupFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			requiresCompletion: true,
			accessToken:        "mailpiece_test",
			requests:           requests,
		})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		signupCompleteCmd.Flags().Set("name", " ")
		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		if err == nil || !strings.Contains(err.Error(), "Full name can't be blank") {
			t.Fatalf("expected the app's refusal, got %v", err)
		}
		if len(requests.paths) != 0 {
			t.Errorf("expected no token request after a refused signup, got %v", requests.paths)
		}
	})

	t.Run("rejects both name and account flags", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		signupCompleteCmd.Flags().Set("name", "Test User")
		signupCompleteCmd.Flags().Set("account", "123456")
		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		assertExitCode(t, err, 1)
	})

	t.Run("rejects neither name nor account flags", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		assertExitCode(t, err, 1)
	})

	t.Run("rejects empty session token", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("")
		defer restoreStdin()

		signupCompleteCmd.Flags().Set("account", "123456")
		err := signupCompleteCmd.RunE(signupCompleteCmd, []string{})
		assertExitCode(t, err, 1)
	})
}

func TestSignupHumanOutputRedactsSecrets(t *testing.T) {
	t.Run("signup start hides pending token in styled output", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestFormat(output.FormatStyled)
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		signupStartCmd.Flags().Set("email", "test@example.com")
		err := signupStartCmd.RunE(signupStartCmd, []string{})
		assertExitCode(t, err, 0)

		raw := TestOutput()
		if strings.Contains(raw, "signed-pending-token-value") {
			t.Fatalf("expected styled output to hide pending token, got:\n%s", raw)
		}
		if !strings.Contains(raw, "TEST01") {
			t.Fatalf("expected styled output to include non-secret dev code, got:\n%s", raw)
		}
	})

	t.Run("signup verify hides session token in styled output", func(t *testing.T) {
		resetSignupFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestFormat(output.FormatStyled)
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		signupVerifyCmd.Flags().Set("code", "VALID1")
		signupVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := signupVerifyCmd.RunE(signupVerifyCmd, []string{})
		assertExitCode(t, err, 0)

		raw := TestOutput()
		if strings.Contains(raw, "signed-session-token-value") {
			t.Fatalf("expected styled output to hide session token, got:\n%s", raw)
		}
		if !strings.Contains(raw, "requires_signup_completion") {
			t.Fatalf("expected styled output to retain non-secret fields, got:\n%s", raw)
		}
	})
}

func TestSignupWizardRequiresATerminal(t *testing.T) {
	SetTestMode()
	SetTestConfig("", "", testAPIURL)
	cfgAgent = true
	defer ResetTestMode()

	err := signupCmd.RunE(signupCmd, []string{})
	assertExitCode(t, err, 1)
}
