package commands

import (
	"encoding/json"
	stderrors "errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"gopkg.in/yaml.v3"
)

func TestSetupCommandDescription(t *testing.T) {
	t.Run("long description names the personal access token", func(t *testing.T) {
		if !strings.Contains(setupCmd.Long, "personal access token") {
			t.Error("expected setup command long description to mention the personal access token")
		}
	})
}

func TestSetupAccountProfileClearsStaleAliasRouting(t *testing.T) {
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "acct",
		BaseURL: testAPIURL,
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"old-account"`)},
	}); err != nil {
		t.Fatalf("create stale alias: %v", err)
	}
	SetTestProfiles(profileStore)
	defer resetTest()

	if err := ensureProfileForAccount("acct", "acct", testAPIURL); err != nil {
		t.Fatalf("save setup profile: %v", err)
	}
	p, err := profileStore.Get("acct")
	if err != nil {
		t.Fatalf("get setup profile: %v", err)
	}
	if account := profileAccount("acct", p); account != "acct" {
		t.Fatalf("setup account: want acct, got %q", account)
	}
}

func TestLoginProfileName(t *testing.T) {
	defer resetTest()

	if got := loginProfileName("834206594"); got != "834206594" {
		t.Fatalf("expected the account's name, got %q", got)
	}
	t.Setenv("MAILPIECE_PROFILE", "shared")
	if got := loginProfileName("834206594"); got != "shared" {
		t.Fatalf("expected MAILPIECE_PROFILE, got %q", got)
	}
	cfgProfile = "walter"
	if got := loginProfileName("834206594"); got != "walter" {
		t.Fatalf("expected --profile, got %q", got)
	}
}

func TestValidateToken(t *testing.T) {
	t.Run("returns accounts on successful validation", func(t *testing.T) {
		mock := NewMockClient()
		mock.OnGet("/my/identity.json", &APIResponse{StatusCode: 200, Data: map[string]any{
			"id": "identity-1",
			"accounts": []any{
				map[string]any{"id": "abc123", "name": "Test Account", "slug": "/123456"},
			},
		}})
		SetTestModeWithSDK(mock)
		defer resetTest()

		identity, err := validateToken(setupCmd, testHTTPServer.URL, "pasted-token")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(identity.Accounts) != 1 {
			t.Fatalf("expected 1 account, got %d", len(identity.Accounts))
		}
		if slug := mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug); slug != "123456" {
			t.Fatalf("expected slug 123456, got %q", slug)
		}
		if len(mock.GetCalls) != 1 || mock.GetCalls[0].Path != "/my/identity.json" {
			t.Fatalf("expected GET /my/identity.json, got %#v", mock.GetCalls)
		}
	})

	t.Run("an invalid token is an auth error", func(t *testing.T) {
		mock := NewMockClient().WithAuthError()
		SetTestModeWithSDK(mock)
		defer resetTest()

		_, err := validateToken(setupCmd, testHTTPServer.URL, "bad-token")
		sdkErr, ok := stderrors.AsType[*mailpiece.Error](err)
		if !ok || sdkErr.Code != mailpiece.CodeAuth {
			t.Fatalf("expected an SDK auth error, got %v", err)
		}
	})

	t.Run("plain http to a remote host is refused", func(t *testing.T) {
		SetTestModeWithSDK(NewMockClient())
		defer resetTest()

		_, err := validateToken(setupCmd, "http://mailpiece.example.com", "token")
		assertExitCode(t, err, 1)
	})
}

func TestValidateAPIURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"accepts HTTPS URL", "https://mailpiece.example.com", ""},
		{"accepts HTTPS with port", "https://mailpiece.example.com:8443", ""},
		{"accepts http://localhost", "http://localhost:3000", ""},
		{"accepts http://127.0.0.1", "http://127.0.0.1:3000", ""},
		{"accepts http://[::1]", "http://[::1]:3000", ""},
		{"accepts http://*.localhost", "http://app.localhost:3000", ""},
		{"rejects empty", "", "URL is required"},
		{"rejects no scheme", "mailpiece.example.com", "URL must start with http:// or https://"},
		{"rejects no host", "https://", "URL must include a hostname"},
		{"rejects http with remote host", "http://mailpiece.example.com", "non-localhost URLs must use https://"},
		{"rejects http with IP", "http://10.0.0.1:3000", "non-localhost URLs must use https://"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAPIURL(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErr)
				} else if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			}
		})
	}
}

// Local is setup's "Local (.mailpiece.yaml in current directory)" choice.
func TestSaveLocal(t *testing.T) {
	t.Run("saves config to local .mailpiece.yaml file", func(t *testing.T) {
		tempDir := t.TempDir()

		config.SetTestWorkingDir(tempDir)
		defer config.ResetTestWorkingDir()

		cfg := &config.Config{
			Token:   "test-token",
			Account: "123456",
			APIURL:  "https://custom.mailpiece.example.com",
		}

		err := cfg.SaveLocal()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		configPath := filepath.Join(tempDir, ".mailpiece.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("config file not created: %v", err)
		}

		var savedConfig config.Config
		if err := yaml.Unmarshal(data, &savedConfig); err != nil {
			t.Fatalf("failed to parse config: %v", err)
		}

		if savedConfig.Token != "test-token" {
			t.Errorf("expected token 'test-token', got '%s'", savedConfig.Token)
		}
		if savedConfig.Account != "123456" {
			t.Errorf("expected account '123456', got '%s'", savedConfig.Account)
		}
		if savedConfig.APIURL != "https://custom.mailpiece.example.com" {
			t.Errorf("expected api_url 'https://custom.mailpiece.example.com', got '%s'", savedConfig.APIURL)
		}
		if config.LocalConfigPath() != configPath {
			t.Errorf("expected the local config to be found at %s, got %q", configPath, config.LocalConfigPath())
		}
	})

	t.Run("file has correct permissions", func(t *testing.T) {
		tempDir := t.TempDir()

		config.SetTestWorkingDir(tempDir)
		defer config.ResetTestWorkingDir()

		cfg := &config.Config{
			Token: "test-token",
		}

		err := cfg.SaveLocal()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		configPath := filepath.Join(tempDir, ".mailpiece.yaml")
		info, err := os.Stat(configPath)
		if err != nil {
			t.Fatalf("failed to stat config file: %v", err)
		}

		perm := info.Mode().Perm()
		if perm != 0600 {
			t.Errorf("expected file permissions 0600, got %o", perm)
		}
	})
}

// Global is setup's "Global (~/.config/mailpiece/config.yaml)" choice, which
// saves the login under a profile named after the account.
func TestSaveLoginGlobal(t *testing.T) {
	t.Run("saves the token under a default profile", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("token: old-token\naccount: old\n"), 0o600); err != nil {
			t.Fatalf("write global config: %v", err)
		}

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		SetTestMode()
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		defer resetTest()

		profileName := loginProfileName("834206594")
		storage, err := saveLogin(profileName, "834206594", "https://mailpiece.example.com", "new-token")
		if err != nil {
			t.Fatalf("saveLogin: %v", err)
		}
		if storage != "file" {
			t.Fatalf("storage: want file, got %q", storage)
		}

		allProfiles, defaultName, err := profileStore.List()
		if err != nil {
			t.Fatalf("list profiles: %v", err)
		}
		if defaultName != profileName {
			t.Fatalf("default profile: want %q, got %q", profileName, defaultName)
		}
		if account := profileAccount(profileName, allProfiles[profileName]); account != "834206594" {
			t.Fatalf("profile account: want 834206594, got %q", account)
		}
		if token, err := credsLoadProfileToken(profileName); err != nil || token != "new-token" {
			t.Fatalf("stored token: want new-token, got %q (%v)", token, err)
		}
		if global := config.LoadGlobal(); global.Token != "" {
			t.Fatalf("expected the old token cleared from the global config, got %q", global.Token)
		}
	})

	t.Run("falls back to the global config without a credential store", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		SetTestMode()
		defer resetTest()

		storage, err := saveLogin("834206594", "834206594", "https://mailpiece.example.com", "new-token")
		if err != nil {
			t.Fatalf("saveLogin: %v", err)
		}
		if storage != "config" {
			t.Fatalf("storage: want config, got %q", storage)
		}
		global := config.LoadGlobal()
		if global.Token != "new-token" || global.Account != "834206594" || global.APIURL != "https://mailpiece.example.com" {
			t.Fatalf("expected the login in the global config, got %#v", global)
		}
		if path := globalConfigPathForDoctor(); path != filepath.Join(configDir, "config.yaml") {
			t.Fatalf("expected setup to report %s, got %q", filepath.Join(configDir, "config.yaml"), path)
		}
	})
}

func TestExistingConfigLocation(t *testing.T) {
	t.Run("local config", func(t *testing.T) {
		workDir := t.TempDir()
		config.SetTestWorkingDir(workDir)
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestWorkingDir()
		defer config.ResetTestConfigDir()
		if err := os.WriteFile(filepath.Join(workDir, config.LocalConfigFile), []byte("account: acme\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		want := "local config (" + filepath.Join(workDir, config.LocalConfigFile) + ")"
		if got := existingConfigLocation(); got != want {
			t.Fatalf("want %q, got %q", want, got)
		}
	})

	t.Run("global config", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestWorkingDir(t.TempDir())
		config.SetTestConfigDir(configDir)
		defer config.ResetTestWorkingDir()
		defer config.ResetTestConfigDir()
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("account: acme\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		if got := existingConfigLocation(); got != "global config" {
			t.Fatalf("want global config, got %q", got)
		}
	})

	t.Run("saved profile", func(t *testing.T) {
		config.SetTestWorkingDir(t.TempDir())
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestWorkingDir()
		defer config.ResetTestConfigDir()
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testAPIURL}); err != nil {
			t.Fatal(err)
		}
		SetTestProfiles(profileStore)
		defer resetTest()

		if got := existingConfigLocation(); got != "saved profile" {
			t.Fatalf("want saved profile, got %q", got)
		}
	})

	t.Run("nothing", func(t *testing.T) {
		config.SetTestWorkingDir(t.TempDir())
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestWorkingDir()
		defer config.ResetTestConfigDir()
		defer resetTest()

		if got := existingConfigLocation(); got != "" {
			t.Fatalf("want nothing, got %q", got)
		}
	})
}

func TestSetupRefusesJQ(t *testing.T) {
	defer resetTest()
	resetTest()
	cfgJQ = ".data"

	err := runSetup(setupCmd, nil)
	assertExitCode(t, err, 1)
	if !errors.IsJQError(err) {
		t.Fatalf("expected a jq error, got %v", err)
	}
}
