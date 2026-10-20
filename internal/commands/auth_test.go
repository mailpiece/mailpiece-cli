package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/credstore"
	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"gopkg.in/yaml.v3"
)

// newTestCredStore returns the CLI's own credential store, forced onto its
// file fallback in dir.
func newTestCredStore(t *testing.T, dir string) *credstore.Store {
	t.Helper()
	t.Setenv("MAILPIECE_NO_KEYRING", "1")
	return credstore.NewStore(credstore.StoreOptions{
		ServiceName:   "mailpiece",
		DisableEnvVar: "MAILPIECE_NO_KEYRING",
		FallbackDir:   dir,
	})
}

// saveTestToken saves a token under a profile the way the CLI does.
func saveTestToken(t *testing.T, store *credstore.Store, profileName, token string) {
	t.Helper()
	data, _ := json.Marshal(token)
	if err := store.Save("profile:"+profileName, data); err != nil {
		t.Fatalf("save token for %s: %v", profileName, err)
	}
}

// profileAccount returns the account a saved profile is bound to.
func profileAccount(name string, p *profile.Profile) string {
	binding, err := resolveProfileAccountBinding(name, p)
	if err != nil {
		return ""
	}
	return binding.Account
}

func TestAuthLogin(t *testing.T) {
	t.Run("saves token to config file", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestConfig("", "test-account", "https://mailpiece.example.com")
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"test-token-123"})
		assertExitCode(t, err, 0)

		if !result.Response.OK {
			t.Error("expected success response")
		}
		if data := responseDataMap(t, result); data["storage"] != "config" {
			t.Errorf("expected storage 'config', got %v", data["storage"])
		}

		// Verify config file was created with correct account
		configPath := filepath.Join(tempDir, "config.yaml")
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("config file not created: %v", err)
		}

		var savedConfig config.Config
		if err := yaml.Unmarshal(data, &savedConfig); err != nil {
			t.Fatalf("failed to parse config: %v", err)
		}

		if savedConfig.Token != "test-token-123" {
			t.Errorf("expected token 'test-token-123', got '%s'", savedConfig.Token)
		}
		if savedConfig.Account != "test-account" {
			t.Errorf("expected account 'test-account', got '%s'", savedConfig.Account)
		}
	})

	t.Run("saves token to credstore under profile-scoped key", func(t *testing.T) {
		configDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))

		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"cred-token-456"})
		assertExitCode(t, err, 0)

		data := responseDataMap(t, result)
		if data["storage"] != "file" {
			t.Errorf("expected storage 'file', got %v", data["storage"])
		}
		if data["profile"] != "acme" || data["account"] != "acme" {
			t.Errorf("expected profile and account 'acme', got %v and %v", data["profile"], data["account"])
		}

		// Token should be stored under "profile:acme"
		loaded, err := store.Load("profile:acme")
		if err != nil {
			t.Fatalf("expected token in credstore under 'profile:acme': %v", err)
		}
		var tokenStr string
		if err := json.Unmarshal(loaded, &tokenStr); err != nil {
			t.Fatalf("expected JSON-encoded token, got %q: %v", string(loaded), err)
		}
		if tokenStr != "cred-token-456" {
			t.Errorf("expected 'cred-token-456', got '%s'", tokenStr)
		}

		// Token should NOT be in YAML config
		configPath := filepath.Join(configDir, "config.yaml")
		if data, err := os.ReadFile(configPath); err == nil {
			var savedConfig config.Config
			yaml.Unmarshal(data, &savedConfig)
			if savedConfig.Token != "" {
				t.Errorf("expected empty token in YAML, got '%s'", savedConfig.Token)
			}
		}

		// Profile should exist in profile store, as the default
		p, err := profileStore.Get("acme")
		if err != nil {
			t.Fatalf("expected profile 'acme' in store: %v", err)
		}
		if p.BaseURL != "https://mailpiece.example.com" {
			t.Errorf("expected base_url 'https://mailpiece.example.com', got '%s'", p.BaseURL)
		}
		if _, defaultName, _ := profileStore.List(); defaultName != "acme" {
			t.Errorf("expected default profile 'acme', got '%s'", defaultName)
		}
	})

	t.Run("clears a token left in the global config once the credstore holds it", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		if err := (&config.Config{Token: "old-yaml-token", Account: "acme"}).Save(); err != nil {
			t.Fatalf("save global config: %v", err)
		}

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()

		if err := authLoginCmd.RunE(authLoginCmd, []string{"new-token"}); err != nil {
			t.Fatalf("login: %v", err)
		}
		globalCfg := config.LoadGlobal()
		if globalCfg.Token != "" {
			t.Errorf("expected global token cleared, got %q", globalCfg.Token)
		}
		if globalCfg.Account != "acme" {
			t.Errorf("expected global account kept, got %q", globalCfg.Account)
		}
	})

	t.Run("saves an alias separately from its account", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "1", "https://mailpiece.example.com")
		activeProfile = "walter"
		defer resetTest()

		if err := authLoginCmd.RunE(authLoginCmd, []string{"walter-token"}); err != nil {
			t.Fatalf("login: %v", err)
		}

		p, err := profileStore.Get("walter")
		if err != nil {
			t.Fatalf("get walter profile: %v", err)
		}
		binding, err := resolveProfileAccountBinding("walter", p)
		if err != nil {
			t.Fatalf("resolve walter binding: %v", err)
		}
		if binding.Account != "1" || !binding.Explicit {
			t.Errorf("profile account: want explicit 1, got %q (explicit %v)", binding.Account, binding.Explicit)
		}
		if _, err := store.Load("profile:walter"); err != nil {
			t.Fatalf("load aliased credential: %v", err)
		}
		if _, err := store.Load("profile:1"); err == nil {
			t.Fatal("alias login saved a credential under the account's name")
		}
	})

	t.Run("rejects invalid aliases before saving credentials", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "1", "https://mailpiece.example.com")
		activeProfile = "walter.agent"
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"agent-token"})
		assertExitCode(t, err, 1)
		if _, err := store.Load("profile:walter.agent"); err == nil {
			t.Fatal("credential was saved for an invalid profile")
		}
	})

	t.Run("invalid selector saves nothing", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "existing", BaseURL: testAPIURL}); err != nil {
			t.Fatalf("create existing profile: %v", err)
		}

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "existing", testAPIURL)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "login", "replacement-token", "--profile", "walter.agent", "--account", "1")
		if err == nil {
			t.Fatal("expected invalid profile error")
		}
		if _, err := store.Load("profile:walter.agent"); err == nil {
			t.Fatal("credential was saved for an invalid profile")
		}
		if _, err := profileStore.Get("walter.agent"); err == nil {
			t.Fatal("profile was saved for an invalid name")
		}
	})

	t.Run("rolls back when the credential store cannot save", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		blockedCredDir := filepath.Join(t.TempDir(), "blocked")
		if err := os.WriteFile(blockedCredDir, []byte("not a directory"), 0600); err != nil {
			t.Fatalf("create blocked credential path: %v", err)
		}
		store := newTestCredStore(t, blockedCredDir)
		profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))

		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "1", testAPIURL)
		activeProfile = "agent"
		defer resetTest()

		if err := authLoginCmd.RunE(authLoginCmd, []string{"replacement-token"}); err == nil {
			t.Fatal("expected the login to fail")
		}
		if _, err := profileStore.Get("agent"); err == nil {
			t.Error("expected the profile rolled back")
		}
		if globalCfg := config.LoadGlobal(); globalCfg.Token != "" || globalCfg.Account != "" {
			t.Errorf("expected the global config untouched, got %q and %q", globalCfg.Token, globalCfg.Account)
		}
	})

	t.Run("rejects unsafe account identifiers before saving", func(t *testing.T) {
		for _, account := range []string{"../other", "other?admin=1", "other%2Fadmin", " other"} {
			t.Run(account, func(t *testing.T) {
				store := newTestCredStore(t, t.TempDir())
				profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
				SetTestModeWithSDK(NewMockClient())
				SetTestCreds(store)
				SetTestProfiles(profileStore)
				SetTestConfig("", account, testAPIURL)
				activeProfile = "agent"
				defer resetTest()

				err := authLoginCmd.RunE(authLoginCmd, []string{"token"})
				assertExitCode(t, err, 1)
				if _, err := store.Load("profile:agent"); err == nil {
					t.Fatal("credential was saved for unsafe account")
				}
				if _, err := profileStore.Get("agent"); err == nil {
					t.Fatal("profile was saved for unsafe account")
				}
			})
		}
	})

	t.Run("does not save credentials when profile creation fails", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		blockedParent := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(blockedParent, []byte("blocked"), 0o600); err != nil {
			t.Fatalf("create blocking file: %v", err)
		}
		profileStore := profile.NewStore(filepath.Join(blockedParent, "config.json"))

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "1", "https://mailpiece.example.com")
		activeProfile = "agent"
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"agent-token"})
		if err == nil {
			t.Fatal("expected profile creation error")
		}
		if _, err := store.Load("profile:agent"); err == nil {
			t.Fatal("credential was saved without a profile")
		}
		if config.Exists() {
			t.Fatal("token was written to the global config after a profile error")
		}
	})

	t.Run("requires profile to be configured", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"some-token"})
		assertExitCode(t, err, 1)
	})

	t.Run("reads the token from stdin with --with-token", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()
		setWithToken(t, "stdin-token\n")

		err := authLoginCmd.RunE(authLoginCmd, nil)
		assertExitCode(t, err, 0)
		if saved := config.LoadGlobal(); saved.Token != "stdin-token" {
			t.Errorf("expected token 'stdin-token', got %q", saved.Token)
		}
	})

	t.Run("refuses a TOKEN argument with --with-token", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()
		setWithToken(t, "stdin-token\n")

		err := authLoginCmd.RunE(authLoginCmd, []string{"argument-token"})
		assertExitCode(t, err, 1)
	})

	t.Run("refuses an empty or multi-line token on stdin", func(t *testing.T) {
		for _, in := range []string{"", "\n", "one\ntwo\n"} {
			mock := NewMockClient()
			SetTestModeWithSDK(mock)
			SetTestConfig("", "acme", testAPIURL)
			setWithToken(t, in)

			err := authLoginCmd.RunE(authLoginCmd, nil)
			assertExitCode(t, err, 1)
			resetTest()
		}
	})

	t.Run("requires a token", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, nil)
		assertExitCode(t, err, 1)
	})

	t.Run("refuses an account with no profile selected or configured", func(t *testing.T) {
		config.SetTestConfigDir(t.TempDir())
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		SetTestModeWithSDK(NewMockClient())
		SetTestProfiles(profile.NewStore(filepath.Join(t.TempDir(), "config.json")))
		defer resetTest()
		t.Cleanup(func() { authLoginAccount = "" })

		_, err := runCobraWithArgs("auth", "login", "some-token", "--account", "834206594")
		if err == nil || !strings.Contains(err.Error(), "No profile configured") {
			t.Fatalf("expected no profile configured, got %v", err)
		}
		if config.Exists() {
			t.Error("expected nothing saved")
		}
	})

	t.Run("requires an account when only a profile is named", func(t *testing.T) {
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "", "https://mailpiece.example.com")
		activeProfile = "agent"
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"some-token"})
		assertExitCode(t, err, 1)
	})

	t.Run("preserves existing config values", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		// Create existing config with account
		existingConfig := &config.Config{
			Account: "existing-account",
			APIURL:  "https://custom.api.com",
		}
		existingData, _ := yaml.Marshal(existingConfig)
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), existingData, 0600)

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "existing-account", "https://custom.api.com")
		defer resetTest()

		err := authLoginCmd.RunE(authLoginCmd, []string{"new-token"})
		assertExitCode(t, err, 0)

		// Verify existing values preserved
		data, _ := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
		var savedConfig config.Config
		yaml.Unmarshal(data, &savedConfig)

		if savedConfig.Token != "new-token" {
			t.Errorf("expected token 'new-token', got '%s'", savedConfig.Token)
		}
		if savedConfig.Account != "existing-account" {
			t.Errorf("expected account 'existing-account', got '%s'", savedConfig.Account)
		}
		if savedConfig.APIURL != "https://custom.api.com" {
			t.Errorf("expected api_url 'https://custom.api.com', got '%s'", savedConfig.APIURL)
		}
	})
}

func TestAuthLoginCreatesProfilesFromExplicitSelectors(t *testing.T) {
	for _, tt := range []struct {
		name            string
		profileName     string
		profileArgs     []string
		envProfile      string
		account         string
		expectedAccount string
	}{
		{name: "flag alias", profileName: "agent", profileArgs: []string{"--profile", "agent"}, account: "1", expectedAccount: "1"},
		{name: "environment alias", profileName: "agent", envProfile: "agent", account: "1", expectedAccount: "1"},
		{name: "profile names the account", profileName: "new-account", profileArgs: []string{"--profile", "new-account"}, expectedAccount: "new-account"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			config.SetTestConfigDir(configDir)
			config.SetTestWorkingDir(t.TempDir())
			defer config.ResetTestConfigDir()
			defer config.ResetTestWorkingDir()

			if tt.envProfile != "" {
				t.Setenv("MAILPIECE_PROFILE", tt.envProfile)
			}
			store := newTestCredStore(t, t.TempDir())
			profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
			if err := profileStore.Create(&profile.Profile{Name: "existing", BaseURL: "https://mailpiece.example.com"}); err != nil {
				t.Fatalf("create existing profile: %v", err)
			}

			mock := NewMockClient()
			SetTestModeWithSDK(mock)
			SetTestCreds(store)
			SetTestProfiles(profileStore)
			SetTestConfig("", "existing", "https://mailpiece.example.com")
			defer resetTest()

			args := make([]string, 0, 5+len(tt.profileArgs))
			args = append(args, "auth", "login", "agent-token")
			if tt.account != "" {
				args = append(args, "--account", tt.account)
			}
			args = append(args, tt.profileArgs...)
			if _, err := runCobraWithArgs(args...); err != nil {
				t.Fatalf("login with %s selector: %v", tt.name, err)
			}

			p, err := profileStore.Get(tt.profileName)
			if err != nil {
				t.Fatalf("get %s profile: %v", tt.profileName, err)
			}
			if account := profileAccount(tt.profileName, p); account != tt.expectedAccount {
				t.Errorf("account: want %q, got %q", tt.expectedAccount, account)
			}
			if _, err := store.Load("profile:" + tt.profileName); err != nil {
				t.Fatalf("load %s credential: %v", tt.profileName, err)
			}
		})
	}
}

func TestAuthLogout(t *testing.T) {
	t.Run("removes profile-scoped token from credstore", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})

		// Save a token under profile-scoped key
		saveTestToken(t, store, "acme", "my-token")

		// Save config
		cfg := &config.Config{Account: "acme"}
		cfgData, _ := yaml.Marshal(cfg)
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), cfgData, 0600)

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("my-token", "acme", "https://mailpiece.example.com")
		activeProfile = "acme"
		defer resetTest()

		// Reset the --all flag
		authLogoutCmd.Flags().Set("all", "false")
		err := authLogoutCmd.RunE(authLogoutCmd, []string{})
		assertExitCode(t, err, 0)

		// Verify token removed from credstore
		if _, err := store.Load("profile:acme"); err == nil {
			t.Error("expected token to be removed from credstore")
		}

		// Verify profile removed from store
		if _, err := profileStore.Get("acme"); err == nil {
			t.Error("expected profile to be removed from store")
		}

		// The default profile's account is cleared from the global config
		if account := config.LoadGlobal().Account; account != "" {
			t.Errorf("expected global account cleared, got %q", account)
		}
	})

	t.Run("keeps the global account when logging out of another profile", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testAPIURL})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: testAPIURL})
		saveTestToken(t, store, "other", "other-token")
		if err := (&config.Config{Account: "acme"}).Save(); err != nil {
			t.Fatalf("save global config: %v", err)
		}

		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("other-token", "other", testAPIURL)
		activeProfile = "other"
		defer resetTest()

		authLogoutCmd.Flags().Set("all", "false")
		if err := authLogoutCmd.RunE(authLogoutCmd, nil); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if account := config.LoadGlobal().Account; account != "acme" {
			t.Errorf("expected global account 'acme' kept, got %q", account)
		}
		if _, err := profileStore.Get("acme"); err != nil {
			t.Errorf("expected profile 'acme' kept: %v", err)
		}
	})

	t.Run("clears a token left in the global config", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		if err := (&config.Config{Token: "yaml-token", Account: "acme"}).Save(); err != nil {
			t.Fatalf("save global config: %v", err)
		}

		SetTestModeWithSDK(NewMockClient())
		SetTestConfig("yaml-token", "acme", testAPIURL)
		defer resetTest()

		authLogoutCmd.Flags().Set("all", "false")
		if err := authLogoutCmd.RunE(authLogoutCmd, nil); err != nil {
			t.Fatalf("logout: %v", err)
		}
		if token := config.LoadGlobal().Token; token != "" {
			t.Errorf("expected global token cleared, got %q", token)
		}
	})

	t.Run("logout --all removes all profiles", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: "https://mailpiece.example.com"})

		// Save tokens for two profiles
		saveTestToken(t, store, "acme", "token1")
		saveTestToken(t, store, "other", "token2")

		// Config
		cfg := &config.Config{Account: "acme"}
		cfgData, _ := yaml.Marshal(cfg)
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), cfgData, 0600)

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("token1", "acme", "https://mailpiece.example.com")
		defer resetTest()

		authLogoutCmd.Flags().Set("all", "true")
		defer authLogoutCmd.Flags().Set("all", "false")
		err := authLogoutCmd.RunE(authLogoutCmd, []string{})
		assertExitCode(t, err, 0)

		// Both tokens should be gone
		if _, err := store.Load("profile:acme"); err == nil {
			t.Error("expected acme token removed")
		}
		if _, err := store.Load("profile:other"); err == nil {
			t.Error("expected other token removed")
		}
		for _, name := range []string{"acme", "other"} {
			if _, err := profileStore.Get(name); err == nil {
				t.Errorf("expected profile %q removed", name)
			}
		}
		if config.Exists() {
			t.Error("expected global config removed")
		}
	})

	t.Run("succeeds even if no config file exists", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestConfig("", "some-profile", "https://mailpiece.example.com")
		activeProfile = "some-profile"
		defer resetTest()

		authLogoutCmd.Flags().Set("all", "false")
		err := authLogoutCmd.RunE(authLogoutCmd, []string{})
		assertExitCode(t, err, 0)
	})

	t.Run("refuses with no profile and no account", func(t *testing.T) {
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()

		SetTestModeWithSDK(NewMockClient())
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		authLogoutCmd.Flags().Set("all", "false")
		err := authLogoutCmd.RunE(authLogoutCmd, []string{})
		assertExitCode(t, err, 1)
	})
}

func TestAuthLogoutReportsCredentialDeletionFailure(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	credDir := filepath.Join(t.TempDir(), "credentials")
	store := newTestCredStore(t, credDir)
	saveTestToken(t, store, "agent", "agent-token")
	backupDir := credDir + "-backup"
	if err := os.Rename(credDir, backupDir); err != nil {
		t.Fatalf("move credential directory: %v", err)
	}
	if err := os.WriteFile(credDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("block credential directory: %v", err)
	}
	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{Name: "agent", BaseURL: testAPIURL}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	SetTestModeWithSDK(NewMockClient())
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	SetTestConfig("agent-token", "agent", testAPIURL)
	activeProfile = "agent"
	authLogoutCmd.Flags().Set("all", "false")
	err := authLogoutCmd.RunE(authLogoutCmd, nil)
	if removeErr := os.Remove(credDir); removeErr != nil {
		t.Fatalf("unblock credential directory: %v", removeErr)
	}
	if renameErr := os.Rename(backupDir, credDir); renameErr != nil {
		t.Fatalf("restore credential directory: %v", renameErr)
	}
	defer resetTest()
	assertExitCode(t, err, 7)
	if _, loadErr := store.Load("profile:agent"); loadErr != nil {
		t.Fatalf("credential should remain after failed deletion: %v", loadErr)
	}
}

func TestAuthLogoutUnknownExplicitProfilePreservesActiveProfile(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "active", "active-token")
	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{Name: "active", BaseURL: testAPIURL}); err != nil {
		t.Fatalf("create active profile: %v", err)
	}
	if err := (&config.Config{Account: "active", APIURL: testAPIURL}).Save(); err != nil {
		t.Fatalf("save global config: %v", err)
	}
	SetTestModeWithSDK(NewMockClient())
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	SetTestConfig("active-token", "active", testAPIURL)
	defer resetTest()

	authLogoutCmd.Flags().Set("all", "false")
	if _, err := runCobraWithArgs("auth", "logout", "--profile", "typo"); err != nil {
		t.Fatalf("logout unknown profile: %v", err)
	}
	if _, err := store.Load("profile:active"); err != nil {
		t.Fatalf("active credential was deleted: %v", err)
	}
	if _, err := profileStore.Get("active"); err != nil {
		t.Fatalf("active profile was deleted: %v", err)
	}
	if account := config.LoadGlobal().Account; account != "active" {
		t.Fatalf("global account changed to %q", account)
	}
}

func TestAuthLogoutAllReportsCredentialDeletionFailure(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	credDir := filepath.Join(t.TempDir(), "credentials")
	store := newTestCredStore(t, credDir)
	saveTestToken(t, store, "agent", "agent-token")
	backupDir := credDir + "-backup"
	if err := os.Rename(credDir, backupDir); err != nil {
		t.Fatalf("move credential directory: %v", err)
	}
	if err := os.WriteFile(credDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("block credential directory: %v", err)
	}
	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{Name: "agent", BaseURL: testAPIURL}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	err := authLogoutAll()
	if removeErr := os.Remove(credDir); removeErr != nil {
		t.Fatalf("unblock credential directory: %v", removeErr)
	}
	if renameErr := os.Rename(backupDir, credDir); renameErr != nil {
		t.Fatalf("restore credential directory: %v", renameErr)
	}
	defer resetTest()
	if err == nil {
		t.Fatal("expected incomplete logout error")
	}
	if _, loadErr := store.Load("profile:agent"); loadErr != nil {
		t.Fatalf("credential should remain after failed deletion: %v", loadErr)
	}
}

func TestAuthLogoutAllCleansInvalidProfileMetadata(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "agent", "agent-token")
	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "agent",
		BaseURL: testAPIURL,
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../invalid"`)},
	}); err != nil {
		t.Fatalf("create invalid profile: %v", err)
	}
	SetTestModeWithSDK(NewMockClient())
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	defer resetTest()

	if err := authLogoutAll(); err != nil {
		t.Fatalf("logout all: %v", err)
	}
	if _, err := store.Load("profile:agent"); err == nil {
		t.Fatal("invalid profile credential still exists")
	}
	if _, err := profileStore.Get("agent"); err == nil {
		t.Fatal("invalid profile still exists")
	}
}

func TestAuthLogoutCleansDefaultProfileWithInvalidMetadata(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "agent", "agent-token")
	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "agent",
		BaseURL: testAPIURL,
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../invalid"`)},
	}); err != nil {
		t.Fatalf("create invalid profile: %v", err)
	}
	if err := profileStore.SetDefault("agent"); err != nil {
		t.Fatalf("set default: %v", err)
	}
	SetTestModeWithSDK(NewMockClient())
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	defer resetTest()
	authLogoutCmd.Flags().Set("all", "false")

	if _, err := runCobraWithArgs("auth", "logout"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if _, err := store.Load("profile:agent"); err == nil {
		t.Fatal("invalid profile credential still exists")
	}
	if _, err := profileStore.Get("agent"); err == nil {
		t.Fatal("invalid profile still exists")
	}
}

func TestAuthLogoutAliasClearsActiveAccount(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	globalCfg := &config.Config{Account: "1", APIURL: "https://mailpiece.example.com"}
	if err := globalCfg.Save(); err != nil {
		t.Fatalf("save global config: %v", err)
	}

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "walter", "alias-token")

	profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "walter",
		BaseURL: "https://mailpiece.example.com",
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
	}); err != nil {
		t.Fatalf("create alias profile: %v", err)
	}

	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	SetTestConfig("", "1", "https://mailpiece.example.com")
	defer resetTest()

	if err := resolveProfile(); err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	resolveToken()
	if cfg.Token != "alias-token" {
		t.Fatalf("token before logout: want alias-token, got %q", cfg.Token)
	}
	authLogoutCmd.Flags().Set("all", "false")
	if err := authLogoutCmd.RunE(authLogoutCmd, nil); err != nil {
		t.Fatalf("logout alias: %v", err)
	}

	// Simulate the next process invocation: nothing selects an account any more.
	cfg = config.Load()
	activeProfile = ""
	cfgProfile = ""
	if err := resolveProfile(); err != nil {
		t.Fatalf("resolve after logout: %v", err)
	}
	resolveToken()
	if cfg.Account != "" {
		t.Errorf("account after logout: want empty, got %q", cfg.Account)
	}
	if cfg.Token != "" {
		t.Errorf("token after logout: want empty, got %q", cfg.Token)
	}
}

func TestAuthStatus(t *testing.T) {
	t.Run("shows authenticated status when token exists", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		// Create config with token
		configData := "token: test-token\naccount: test-account"
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configData), 0600)

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "status")
		assertExitCode(t, err, 0)

		if !lastResult.Response.OK {
			t.Error("expected success response")
		}

		data := responseDataMap(t, lastResult)
		if data["authenticated"] != true {
			t.Errorf("expected authenticated=true, got %v", data["authenticated"])
		}
		if data["token_source"] != "config" {
			t.Errorf("expected token_source='config', got %v", data["token_source"])
		}
		if data["account"] != "test-account" {
			t.Errorf("expected account='test-account', got %v", data["account"])
		}
		if _, ok := data["api_url"]; ok {
			t.Errorf("expected no api_url for the default URL, got %v", data["api_url"])
		}
	})

	t.Run("shows unauthenticated status when no token", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "status")
		assertExitCode(t, err, 0)

		data := responseDataMap(t, lastResult)
		if data["authenticated"] != false {
			t.Errorf("expected authenticated=false, got %v", data["authenticated"])
		}
		if _, ok := data["token_source"]; ok {
			t.Errorf("expected no token_source without a token, got %v", data["token_source"])
		}
		crumbs := lastResult.Response.Breadcrumbs
		if len(crumbs) != 1 || crumbs[0].Action != "login" {
			t.Errorf("expected a single login breadcrumb, got %#v", crumbs)
		}
	})

	t.Run("names where the token came from", func(t *testing.T) {
		for _, tt := range []struct {
			name        string
			env         map[string]string
			args        []string
			tokenSource string
		}{
			{name: "environment", env: map[string]string{"MAILPIECE_TOKEN": "env-token", "MAILPIECE_ACCOUNT": "acme"}, tokenSource: "env"},
			{name: "token flag", env: map[string]string{"MAILPIECE_ACCOUNT": "acme"}, args: []string{"--token", "flag-token"}, tokenSource: "flag"},
		} {
			t.Run(tt.name, func(t *testing.T) {
				config.SetTestConfigDir(t.TempDir())
				config.SetTestWorkingDir(t.TempDir())
				defer config.ResetTestConfigDir()
				defer config.ResetTestWorkingDir()
				for k, v := range tt.env {
					t.Setenv(k, v)
				}

				SetTestModeWithSDK(NewMockClient())
				defer resetTest()

				_, err := runCobraWithArgs(append([]string{"auth", "status"}, tt.args...)...)
				assertExitCode(t, err, 0)

				data := responseDataMap(t, lastResult)
				if data["token_source"] != tt.tokenSource {
					t.Errorf("expected token_source=%q, got %v", tt.tokenSource, data["token_source"])
				}
				if data["account"] != "acme" {
					t.Errorf("expected account='acme', got %v", data["account"])
				}
			})
		}
	})

	t.Run("names the profile a token came from", func(t *testing.T) {
		config.SetTestConfigDir(t.TempDir())
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "acme-token")
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testAPIURL})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: testAPIURL})

		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "status")
		assertExitCode(t, err, 0)

		data := responseDataMap(t, lastResult)
		if data["profile"] != "acme" {
			t.Errorf("expected profile='acme', got %v", data["profile"])
		}
		if data["token_source"] != "credentials file" {
			t.Errorf("expected token_source='credentials file', got %v", data["token_source"])
		}
		var listed bool
		for _, crumb := range lastResult.Response.Breadcrumbs {
			if crumb.Cmd == "mailpiece auth list" {
				listed = true
			}
		}
		if !listed {
			t.Errorf("expected an auth list breadcrumb with several profiles, got %#v", lastResult.Response.Breadcrumbs)
		}
	})

	t.Run("shows using_keyring field when credstore is set", func(t *testing.T) {
		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestConfig("token", "account", "https://api.example.com")

		// A file-based credstore (the env var disables the keyring probe)
		store := newTestCredStore(t, t.TempDir())
		SetTestCreds(store)
		defer resetTest()

		err := authStatusCmd.RunE(authStatusCmd, []string{})
		assertExitCode(t, err, 0)

		data := responseDataMap(t, result)
		if data["using_keyring"] != false {
			t.Errorf("expected using_keyring=false for the file store, got %v", data["using_keyring"])
		}
	})

	t.Run("shows custom api_url when configured", func(t *testing.T) {
		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		// Create config with custom API URL
		configData := "token: test-token\napi_url: https://custom.mailpiece.dev"
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), []byte(configData), 0600)

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "status")
		assertExitCode(t, err, 0)

		data := responseDataMap(t, lastResult)
		if data["api_url"] != "https://custom.mailpiece.dev" {
			t.Errorf("expected api_url='https://custom.mailpiece.dev', got %v", data["api_url"])
		}
	})
}

func TestAuthList(t *testing.T) {
	t.Run("lists authenticated profiles", func(t *testing.T) {
		profileDir := t.TempDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		profileStore.Create(&profile.Profile{
			Name:    "other",
			BaseURL: "https://staging.mailpiece.dev",
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
		})

		// Save a token for one profile only
		saveTestToken(t, store, "acme", "token1")

		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		defer resetTest()

		err := authListCmd.RunE(authListCmd, []string{})
		assertExitCode(t, err, 0)

		profiles, ok := result.Response.Data.([]any)
		if !ok {
			t.Fatal("expected array response data")
		}
		if len(profiles) != 2 {
			t.Fatalf("expected 2 profiles, got %d", len(profiles))
		}
		if result.Response.Summary != "2 profile(s)" {
			t.Errorf("expected summary '2 profile(s)', got %q", result.Response.Summary)
		}

		// acme is the default since it was created first
		for _, p := range profiles {
			entry := p.(map[string]any)
			switch entry["profile"] {
			case "acme":
				if entry["active"] != true || entry["has_token"] != true || entry["account"] != "acme" {
					t.Errorf("unexpected acme entry: %#v", entry)
				}
			case "other":
				if entry["active"] != false || entry["has_token"] != false || entry["account"] != "1" {
					t.Errorf("unexpected other entry: %#v", entry)
				}
				if entry["base_url"] != "https://staging.mailpiece.dev" {
					t.Errorf("expected other's base_url, got %v", entry["base_url"])
				}
			default:
				t.Errorf("unexpected profile %v", entry["profile"])
			}
		}
	})

	t.Run("shows empty list when no profiles", func(t *testing.T) {
		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		defer resetTest()

		err := authListCmd.RunE(authListCmd, []string{})
		assertExitCode(t, err, 0)

		profiles, ok := result.Response.Data.([]any)
		if !ok {
			t.Fatal("expected array response data")
		}
		if len(profiles) != 0 {
			t.Errorf("expected 0 profiles, got %d", len(profiles))
		}
		if result.Response.Summary != "No profiles configured" {
			t.Errorf("expected summary 'No profiles configured', got %q", result.Response.Summary)
		}
	})

	t.Run("refuses a profile with invalid account metadata", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "broken",
			BaseURL: testAPIURL,
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../invalid"`)},
		})

		SetTestModeWithSDK(NewMockClient())
		SetTestProfiles(profileStore)
		defer resetTest()

		err := authListCmd.RunE(authListCmd, []string{})
		assertExitCode(t, err, 1)
	})

	t.Run("renders styled output with next steps", func(t *testing.T) {
		profileDir := t.TempDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		saveTestToken(t, store, "acme", "token1")

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestFormat(output.FormatStyled)
		defer resetTest()

		err := authListCmd.RunE(authListCmd, []string{})
		assertExitCode(t, err, 0)

		raw := TestOutput()
		if !strings.Contains(raw, "Profile") {
			t.Fatalf("expected styled table output, got:\n%s", raw)
		}
		if !strings.Contains(raw, "Next steps:") {
			t.Fatalf("expected next steps section, got:\n%s", raw)
		}
	})
}

func TestAuthSwitch(t *testing.T) {
	t.Run("switches active profile", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: "https://staging.mailpiece.dev"})

		// Save token for the target profile
		saveTestToken(t, store, "other", "other-token")

		initialCfg := &config.Config{Account: "acme"}
		cfgData, _ := yaml.Marshal(initialCfg)
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), cfgData, 0600)

		mock := NewMockClient()
		result := SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("acme-token", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := authSwitchCmd.RunE(authSwitchCmd, []string{"other"})
		assertExitCode(t, err, 0)

		data := responseDataMap(t, result)
		if data["profile"] != "other" || data["account"] != "other" {
			t.Errorf("expected profile and account 'other', got %v and %v", data["profile"], data["account"])
		}

		// Verify config was updated
		raw, _ := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
		var savedConfig config.Config
		yaml.Unmarshal(raw, &savedConfig)

		if savedConfig.Account != "other" {
			t.Errorf("expected account 'other', got '%s'", savedConfig.Account)
		}
		if savedConfig.APIURL != "https://staging.mailpiece.dev" {
			t.Errorf("expected persisted target API URL, got %q", savedConfig.APIURL)
		}
		if cfg.APIURL != "https://staging.mailpiece.dev" {
			t.Errorf("expected target API URL to be applied, got %q", cfg.APIURL)
		}
		if cfg.Token != "other-token" {
			t.Errorf("expected target token to be applied, got %q", cfg.Token)
		}
		if activeProfile != "other" {
			t.Errorf("expected active profile 'other', got %q", activeProfile)
		}
		targetProfile, err := profileStore.Get("other")
		if err != nil {
			t.Fatalf("get target profile: %v", err)
		}
		if targetProfile.BaseURL != "https://staging.mailpiece.dev" {
			t.Errorf("expected target API URL to remain unchanged, got %q", targetProfile.BaseURL)
		}

		// Verify profile store default was updated
		_, defaultName, _ := profileStore.List()
		if defaultName != "other" {
			t.Errorf("expected default profile 'other', got '%s'", defaultName)
		}
	})

	for _, tt := range []struct {
		name      string
		envURL    string
		flagURL   string
		effective string
	}{
		{name: "preserves environment API URL override", envURL: "https://env.example.com", effective: "https://env.example.com"},
		{name: "preserves flag API URL override", envURL: "https://env.example.com", flagURL: "https://flag.example.com", effective: "https://flag.example.com"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			configDir := t.TempDir()
			config.SetTestConfigDir(configDir)
			defer config.ResetTestConfigDir()
			t.Setenv("MAILPIECE_API_URL", tt.envURL)

			store := newTestCredStore(t, t.TempDir())
			profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
			if err := profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"}); err != nil {
				t.Fatalf("create acme profile: %v", err)
			}
			if err := profileStore.Create(&profile.Profile{Name: "other", BaseURL: "https://profile.example.com"}); err != nil {
				t.Fatalf("create other profile: %v", err)
			}
			saveTestToken(t, store, "other", "other-token")

			SetTestModeWithSDK(NewMockClient())
			SetTestCreds(store)
			SetTestProfiles(profileStore)
			SetTestConfig("acme-token", "acme", tt.effective)
			cfgAPIURL = tt.flagURL
			defer resetTest()

			if err := authSwitchCmd.RunE(authSwitchCmd, []string{"other"}); err != nil {
				t.Fatalf("switch profile: %v", err)
			}
			if cfg.APIURL != tt.effective {
				t.Errorf("effective API URL: want %q, got %q", tt.effective, cfg.APIURL)
			}
			data, err := os.ReadFile(filepath.Join(configDir, "config.yaml"))
			if err != nil {
				t.Fatalf("read global config: %v", err)
			}
			var savedConfig config.Config
			if err := yaml.Unmarshal(data, &savedConfig); err != nil {
				t.Fatalf("parse global config: %v", err)
			}
			if savedConfig.APIURL != "https://profile.example.com" {
				t.Errorf("persisted API URL: want profile URL, got %q", savedConfig.APIURL)
			}
		})
	}

	t.Run("empty profile credential is rejected", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "agent", "")
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "agent", BaseURL: testAPIURL}); err != nil {
			t.Fatalf("create profile: %v", err)
		}
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "agent", testAPIURL)
		defer resetTest()

		err := authSwitchCmd.RunE(authSwitchCmd, []string{"agent"})
		assertExitCode(t, err, 7)
	})

	t.Run("alias requires its own credential", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "existing", "existing-token")
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "existing", BaseURL: testAPIURL}); err != nil {
			t.Fatalf("create existing profile: %v", err)
		}
		if err := profileStore.Create(&profile.Profile{
			Name:    "agent",
			BaseURL: testAPIURL,
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"existing"`)},
		}); err != nil {
			t.Fatalf("create alias: %v", err)
		}
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("existing-token", "existing", testAPIURL)
		defer resetTest()

		if err := authSwitchCmd.RunE(authSwitchCmd, []string{"agent"}); err == nil {
			t.Fatal("expected missing alias credential error")
		}
		_, defaultName, err := profileStore.List()
		if err != nil {
			t.Fatalf("list profiles: %v", err)
		}
		if defaultName != "existing" {
			t.Fatalf("default profile changed to %q", defaultName)
		}
	})

	t.Run("refuses a target with invalid account metadata", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "broken", "broken-token")
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "existing", BaseURL: testAPIURL}); err != nil {
			t.Fatalf("create existing profile: %v", err)
		}
		if err := profileStore.Create(&profile.Profile{
			Name:    "broken",
			BaseURL: testAPIURL,
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../invalid"`)},
		}); err != nil {
			t.Fatalf("create broken profile: %v", err)
		}
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "existing", testAPIURL)
		defer resetTest()

		err := authSwitchCmd.RunE(authSwitchCmd, []string{"broken"})
		assertExitCode(t, err, 1)
		if _, defaultName, _ := profileStore.List(); defaultName != "existing" {
			t.Fatalf("default profile changed to %q", defaultName)
		}
	})

	t.Run("recovers from an invalid default profile", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(configDir, "config.json"))
		if err := profileStore.Create(&profile.Profile{
			Name:    "broken",
			BaseURL: testAPIURL,
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../invalid"`)},
		}); err != nil {
			t.Fatalf("create broken profile: %v", err)
		}
		if err := profileStore.Create(&profile.Profile{Name: "good", BaseURL: testAPIURL}); err != nil {
			t.Fatalf("create good profile: %v", err)
		}
		saveTestToken(t, store, "good", "good-token")
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "broken", testAPIURL)
		defer resetTest()

		if _, err := runCobraWithArgs("auth", "switch", "good"); err != nil {
			t.Fatalf("switch from invalid default: %v", err)
		}
		_, defaultName, err := profileStore.List()
		if err != nil {
			t.Fatalf("list profiles: %v", err)
		}
		if defaultName != "good" {
			t.Fatalf("default profile: want good, got %q", defaultName)
		}
	})

	t.Run("global config failure restores previous default", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		if err := os.Mkdir(filepath.Join(configDir, "config.yaml"), 0700); err != nil {
			t.Fatalf("block global config: %v", err)
		}
		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		for _, name := range []string{"acme", "other"} {
			if err := profileStore.Create(&profile.Profile{Name: name, BaseURL: testAPIURL}); err != nil {
				t.Fatalf("create profile %s: %v", name, err)
			}
			saveTestToken(t, store, name, name+"-token")
		}
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("acme-token", "acme", testAPIURL)
		defer resetTest()

		if err := authSwitchCmd.RunE(authSwitchCmd, []string{"other"}); err == nil {
			t.Fatal("expected global config error")
		}
		_, defaultName, err := profileStore.List()
		if err != nil {
			t.Fatalf("list profiles: %v", err)
		}
		if defaultName != "acme" {
			t.Fatalf("default profile: want acme, got %q", defaultName)
		}
		if _, err := profileStore.Get("other"); err != nil {
			t.Fatalf("target profile lost in the rollback: %v", err)
		}
		if cfg.Account != "acme" || cfg.Token != "acme-token" {
			t.Fatalf("effective config changed to %q with %q", cfg.Account, cfg.Token)
		}
	})

	t.Run("reconstructed profile inherits the effective API URL", func(t *testing.T) {
		configDir := t.TempDir()
		config.SetTestConfigDir(configDir)
		defer config.ResetTestConfigDir()
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "other", "other-token")
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		if err := profileStore.Create(&profile.Profile{Name: "current", BaseURL: "https://self-hosted.example.com"}); err != nil {
			t.Fatalf("create current profile: %v", err)
		}
		SetTestModeWithSDK(NewMockClient())
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("current-token", "current", "https://self-hosted.example.com")
		defer resetTest()

		if err := authSwitchCmd.RunE(authSwitchCmd, []string{"other"}); err != nil {
			t.Fatalf("switch reconstructed profile: %v", err)
		}
		reconstructed, err := profileStore.Get("other")
		if err != nil {
			t.Fatalf("get reconstructed profile: %v", err)
		}
		if reconstructed.BaseURL != "https://self-hosted.example.com" {
			t.Fatalf("BaseURL: want self-hosted URL, got %q", reconstructed.BaseURL)
		}
	})

	t.Run("rejects an invalid profile name", func(t *testing.T) {
		SetTestModeWithSDK(NewMockClient())
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()

		err := authSwitchCmd.RunE(authSwitchCmd, []string{"walter.agent"})
		assertExitCode(t, err, 1)
	})

	t.Run("fails for unknown profile", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		defer resetTest()

		err := authSwitchCmd.RunE(authSwitchCmd, []string{"nonexistent"})
		assertExitCode(t, err, 7)
	})
}

func TestProfileAliasesUseSharedAccountWithDistinctTokens(t *testing.T) {
	type observedRequest struct {
		path          string
		authorization string
	}
	requests := make(chan observedRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observedRequest{path: r.URL.Path, authorization: r.Header.Get("Authorization")}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	store := newTestCredStore(t, t.TempDir())
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	for _, name := range []string{"walter", "walter2"} {
		if err := profileStore.Create(&profile.Profile{
			Name:    name,
			BaseURL: server.URL,
			Extra: map[string]json.RawMessage{
				"account": json.RawMessage(`"1"`),
			},
		}); err != nil {
			t.Fatalf("create profile %s: %v", name, err)
		}
	}

	for profileName, token := range map[string]string{
		"walter":  "walter-token",
		"walter2": "agent-token",
	} {
		saveTestToken(t, store, profileName, token)
	}

	SetTestMode()
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	SetTestConfig("", "other-account", server.URL)
	defer resetTest()

	for _, tt := range []struct {
		profile string
		token   string
	}{
		{profile: "walter", token: "walter-token"},
		{profile: "walter2", token: "agent-token"},
	} {
		t.Run(tt.profile, func(t *testing.T) {
			cfgProfile = tt.profile
			cfg.Token = ""
			if err := resolveProfile(); err != nil {
				t.Fatalf("resolve profile: %v", err)
			}
			resolveToken()

			if cfg.Account != "1" {
				t.Errorf("account: want shared account '1', got %q", cfg.Account)
			}
			if cfg.Token != tt.token {
				t.Errorf("token: want %q, got %q", tt.token, cfg.Token)
			}

			if err := initSDK(fixtureListCmd, cfg.APIURL, cfg.Token); err != nil {
				t.Fatalf("initialize SDK: %v", err)
			}
			if _, err := getSDK().Get(context.Background(), "/posts.json"); err != nil {
				t.Fatalf("read the account: %v", err)
			}
			request := <-requests
			if request.path != "/1/posts.json" {
				t.Errorf("request path: want /1/posts.json, got %q", request.path)
			}
			if request.authorization != "Bearer "+tt.token {
				t.Errorf("authorization: want bearer token for %s, got %q", tt.profile, request.authorization)
			}
		})
	}
}

func TestProfileStoreReadFailureIsReported(t *testing.T) {
	profilePath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(profilePath, []byte(`{invalid`), 0600); err != nil {
		t.Fatalf("write malformed profile store: %v", err)
	}
	SetTestProfiles(profile.NewStore(profilePath))
	SetTestConfig("", "agent", testAPIURL)
	cfgProfile = "agent"
	defer resetTest()

	if err := resolveProfile(); err == nil {
		t.Fatal("expected profile-store read error")
	}
}

func TestInvalidProfileAccountMetadataIsRejected(t *testing.T) {
	for _, raw := range []string{`{}`, `null`, `""`, `"   "`, `"../other"`, `"other?admin=1"`, `"other%2Fadmin"`} {
		t.Run(raw, func(t *testing.T) {
			profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
			if err := profileStore.Create(&profile.Profile{
				Name:    "agent",
				BaseURL: testAPIURL,
				Extra:   map[string]json.RawMessage{"account": json.RawMessage(raw)},
			}); err != nil {
				t.Fatalf("create profile: %v", err)
			}
			SetTestProfiles(profileStore)
			SetTestConfig("token", "legacy", testAPIURL)
			defer resetTest()

			if err := resolveProfile(); err == nil {
				t.Fatal("expected invalid account metadata error")
			}
		})
	}
}

func TestProfileAccountDefaultsToProfileName(t *testing.T) {
	p := &profile.Profile{Name: "6102600", BaseURL: "https://mailpiece.example.com"}
	if account := profileAccount(p.Name, p); account != "6102600" {
		t.Errorf("account: want profile name, got %q", account)
	}
}

func TestProfileFlagTokenSelection(t *testing.T) {
	t.Run("resolveToken loads token for profile specified via flag", func(t *testing.T) {
		profileDir := t.TempDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: "https://mailpiece.example.com"})

		// Save tokens for two profiles
		saveTestToken(t, store, "acme", "acme-token")
		saveTestToken(t, store, "other", "other-token")

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		defer resetTest()

		// Simulate --profile other flag: resolve profile first, then token
		cfgProfile = "other"

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolveToken()

		if cfg.Token != "other-token" {
			t.Errorf("expected 'other-token' for --profile other, got '%s'", cfg.Token)
		}
		if cfg.Account != "other" {
			t.Errorf("expected account 'other' from profile resolution, got '%s'", cfg.Account)
		}
	})

	t.Run("resolveToken uses default profile when no flag", func(t *testing.T) {
		profileDir := t.TempDir()

		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})
		profileStore.Create(&profile.Profile{Name: "other", BaseURL: "https://mailpiece.example.com"})

		saveTestToken(t, store, "acme", "acme-token")
		saveTestToken(t, store, "other", "other-token")

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		// No --profile flag, "acme" is default (first created)
		cfgProfile = ""

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		resolveToken()

		if cfg.Token != "acme-token" {
			t.Errorf("expected 'acme-token' for default profile, got '%s'", cfg.Token)
		}
	})

	t.Run("invalid --profile flag returns error", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "acme", "https://mailpiece.example.com")
		defer resetTest()

		cfgProfile = "nonexistent"

		err := resolveProfile()
		if err == nil {
			t.Error("expected error for invalid --profile flag")
		}
	})

	t.Run("invalid MAILPIECE_PROFILE env var returns error", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://mailpiece.example.com"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		t.Setenv("MAILPIECE_PROFILE", "nonexistent")

		err := resolveProfile()
		if err == nil {
			t.Error("expected error for invalid MAILPIECE_PROFILE env var")
		}
	})
}

func TestResolveTokenPrecedence(t *testing.T) {
	t.Run("profile credential outranks the config token", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "cred-token")

		SetTestCreds(store)
		SetTestConfig("yaml-token", "acme", testAPIURL)
		activeProfile = "acme"
		defer resetTest()

		resolveToken()

		if cfg.Token != "cred-token" {
			t.Errorf("expected 'cred-token' from credstore, got '%s'", cfg.Token)
		}
	})

	t.Run("environment outranks the profile credential and flag outranks both", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "cred-token")

		SetTestCreds(store)
		SetTestConfig("", "acme", testAPIURL)
		activeProfile = "acme"
		defer resetTest()

		t.Setenv("MAILPIECE_TOKEN", "env-token")
		resolveToken()
		if cfg.Token != "env-token" {
			t.Errorf("expected 'env-token', got '%s'", cfg.Token)
		}

		cfgToken = "flag-token"
		resolveToken()
		if cfg.Token != "flag-token" {
			t.Errorf("expected 'flag-token', got '%s'", cfg.Token)
		}
	})

	t.Run("does not persist an env-var token to credstore", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())

		SetTestCreds(store)
		SetTestConfig("", "acme", testAPIURL)
		activeProfile = "acme"
		defer resetTest()

		t.Setenv("MAILPIECE_TOKEN", "env-token")
		resolveToken()

		// Credstore should remain empty: env tokens must not be persisted
		if _, err := store.Load("profile:acme"); err == nil {
			t.Error("env-var token should not be saved to credstore")
		}
	})
}

func TestProfileResolution(t *testing.T) {
	t.Run("MAILPIECE_PROFILE env var sets account and BaseURL", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "staging", BaseURL: "https://staging.mailpiece.dev"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		t.Setenv("MAILPIECE_PROFILE", "staging")

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Account != "staging" {
			t.Errorf("expected account 'staging', got '%s'", cfg.Account)
		}
		if cfg.APIURL != "https://staging.mailpiece.dev" {
			t.Errorf("expected APIURL 'https://staging.mailpiece.dev', got '%s'", cfg.APIURL)
		}
	})

	t.Run("profile BaseURL overrides YAML config APIURL", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "custom", BaseURL: "https://custom.example.com"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		// Single profile auto-selects
		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.APIURL != "https://custom.example.com" {
			t.Errorf("expected APIURL 'https://custom.example.com', got '%s'", cfg.APIURL)
		}
	})

	t.Run("MAILPIECE_API_URL env var beats profile BaseURL", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "custom", BaseURL: "https://profile.example.com"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		t.Setenv("MAILPIECE_API_URL", "https://env.example.com")
		// Simulate what config.Load() does: apply env var to cfg before profile resolution
		cfg.APIURL = "https://env.example.com"

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Env var value should survive: resolveProfile must not overwrite it with profile BaseURL
		if cfg.APIURL != "https://env.example.com" {
			t.Errorf("expected APIURL 'https://env.example.com' (from env), got '%s'", cfg.APIURL)
		}
	})

	t.Run("the profile's account beats MAILPIECE_ACCOUNT", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "agent",
			BaseURL: "https://mailpiece.example.com",
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
		})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		defer resetTest()

		t.Setenv("MAILPIECE_PROFILE", "agent")
		t.Setenv("MAILPIECE_ACCOUNT", "2")
		// Simulate what config.Load() does: apply env var to cfg before profile resolution
		cfg.Account = "2"

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if cfg.Account != "1" {
			t.Errorf("expected account '1' (from the profile), got '%s'", cfg.Account)
		}
		if activeProfile != "agent" || !activeProfileExplicitAccount {
			t.Errorf("expected active alias 'agent', got %q (explicit %v)", activeProfile, activeProfileExplicitAccount)
		}
	})

	t.Run("a selected profile is taken as the account while none is saved", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		SetTestProfiles(profileStore)
		SetTestConfig("", "", "https://mailpiece.example.com")
		cfgProfile = "acme"
		defer resetTest()

		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if activeProfile != "acme" || cfg.Account != "acme" {
			t.Errorf("expected profile and account 'acme', got %q and %q", activeProfile, cfg.Account)
		}

		t.Setenv("MAILPIECE_ACCOUNT", "env-account")
		cfg.Account = "env-account"
		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if activeProfile != "acme" || cfg.Account != "acme" {
			t.Errorf("expected --profile to beat MAILPIECE_ACCOUNT, got %q and %q", activeProfile, cfg.Account)
		}

		cfgProfile = ""
		if err := resolveProfile(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if activeProfile != "env-account" || cfg.Account != "env-account" {
			t.Errorf("expected MAILPIECE_ACCOUNT as profile and account, got %q and %q", activeProfile, cfg.Account)
		}
	})
}

func TestEnsureProfileUpdatesExisting(t *testing.T) {
	t.Run("updates existing profile's BaseURL and account", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://old.example.com"})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		defer resetTest()

		if err := ensureProfileForAccount("acme", "1", "https://new.example.com"); err != nil {
			t.Fatalf("ensure profile: %v", err)
		}

		p, err := profileStore.Get("acme")
		if err != nil {
			t.Fatalf("expected profile to exist: %v", err)
		}
		if p.BaseURL != "https://new.example.com" {
			t.Errorf("expected BaseURL 'https://new.example.com', got '%s'", p.BaseURL)
		}
		if account := profileAccount("acme", p); account != "1" {
			t.Errorf("expected account '1', got '%s'", account)
		}
	})

	t.Run("overwrites self-hosted BaseURL with default on hosted re-login", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: "https://selfhosted.example.com",
			Extra: map[string]json.RawMessage{
				"account": json.RawMessage(`"1"`),
			},
		})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		defer resetTest()

		if err := ensureProfileForAccount("acme", "", testAPIURL); err != nil {
			t.Fatalf("ensure profile: %v", err)
		}

		p, err := profileStore.Get("acme")
		if err != nil {
			t.Fatalf("expected profile to exist: %v", err)
		}
		if p.BaseURL != testAPIURL {
			t.Errorf("expected BaseURL '%s', got '%s'", testAPIURL, p.BaseURL)
		}
		// The account should be preserved since we passed empty
		if account := profileAccount("acme", p); account != "1" {
			t.Errorf("expected account '1' to be preserved, got '%s'", account)
		}
	})

	t.Run("preserves existing BaseURL when caller passes empty", func(t *testing.T) {
		profileDir := t.TempDir()
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: "https://custom.example.com",
		})

		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		SetTestProfiles(profileStore)
		defer resetTest()

		if err := ensureProfileForAccount("acme", "", ""); err != nil {
			t.Fatalf("ensure profile: %v", err)
		}

		p, err := profileStore.Get("acme")
		if err != nil {
			t.Fatalf("expected profile to exist: %v", err)
		}
		if p.BaseURL != "https://custom.example.com" {
			t.Errorf("expected BaseURL 'https://custom.example.com', got '%s'", p.BaseURL)
		}
	})

	t.Run("an account equal to the name is stored implicitly", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: testAPIURL,
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
		})

		SetTestProfiles(profileStore)
		defer resetTest()

		if err := ensureProfileForAccount("acme", "acme", ""); err != nil {
			t.Fatalf("ensure profile: %v", err)
		}

		p, err := profileStore.Get("acme")
		if err != nil {
			t.Fatalf("expected profile to exist: %v", err)
		}
		if _, ok := p.Extra["account"]; ok {
			t.Errorf("expected no explicit account, got %s", p.Extra["account"])
		}
		binding, err := resolveProfileAccountBinding("acme", p)
		if err != nil || binding.Account != "acme" || binding.Explicit {
			t.Errorf("expected implicit account 'acme', got %#v (%v)", binding, err)
		}
	})

	t.Run("an alias fails without a profile store", func(t *testing.T) {
		defer resetTest()
		if err := ensureProfileForAccount("agent", "acme", ""); err == nil {
			t.Fatal("expected an error without a profile store")
		}
		if err := ensureProfileForAccount("acme", "acme", ""); err != nil {
			t.Fatalf("expected a profile named after its account to need no store, got %v", err)
		}
	})
}

// TestPrecedenceChainIntegration exercises the full precedence chain as
// PersistentPreRunE would: config.Load() → resolveProfile() → resolveToken()
// → flag overrides, wired up with real config files, credstore, and profile store.
func TestPrecedenceChainIntegration(t *testing.T) {
	t.Run("profile outranks YAML config for APIURL and account", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		// Write YAML config with values that should be overridden
		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()
		yamlCfg := &config.Config{
			Token:   "yaml-token",
			Account: "yaml-account",
			APIURL:  "https://yaml.example.com",
		}
		yamlData, _ := yaml.Marshal(yamlCfg)
		os.WriteFile(filepath.Join(tempDir, "config.yaml"), yamlData, 0600)

		// Profile with different BaseURL and account
		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: "https://profile.example.com",
			Extra: map[string]json.RawMessage{
				"account": json.RawMessage(`"1"`),
			},
		})
		profileStore.SetDefault("acme")

		// Credstore with token under profile key
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "cred-token")

		// Step 1: config.Load() picks up YAML values
		loaded := config.Load()

		// Step 2: wire up package state as PersistentPreRunE would
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		defer resetTest()
		cfg = loaded
		SetTestCreds(store)
		SetTestProfiles(profileStore)

		// Step 3: resolveProfile() overwrites YAML
		if err := resolveProfile(); err != nil {
			t.Fatalf("resolveProfile: %v", err)
		}

		// Step 4: resolveToken() overwrites the YAML token with the credstore's
		resolveToken()

		if cfg.Account != "1" {
			t.Errorf("account: want profile value '1', got '%s'", cfg.Account)
		}
		if cfg.APIURL != "https://profile.example.com" {
			t.Errorf("APIURL: want profile value 'https://profile.example.com', got '%s'", cfg.APIURL)
		}
		if cfg.Token != "cred-token" {
			t.Errorf("token: want credstore value 'cred-token', got '%s'", cfg.Token)
		}
	})

	t.Run("env vars beat profile, except for its account", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: "https://profile.example.com",
			Extra: map[string]json.RawMessage{
				"account": json.RawMessage(`"1"`),
			},
		})
		profileStore.SetDefault("acme")

		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "cred-token")

		// Set env vars that should beat profile
		t.Setenv("MAILPIECE_API_URL", "https://env.example.com")
		t.Setenv("MAILPIECE_TOKEN", "env-token")

		// Step 1: config.Load() picks up env vars
		loaded := config.Load()

		// Step 2: wire up
		mock := NewMockClient()
		SetTestModeWithSDK(mock)
		defer resetTest()
		cfg = loaded
		SetTestCreds(store)
		SetTestProfiles(profileStore)

		// Step 3: resolveProfile()
		if err := resolveProfile(); err != nil {
			t.Fatalf("resolveProfile: %v", err)
		}

		// Step 4: resolveToken()
		resolveToken()

		if cfg.APIURL != "https://env.example.com" {
			t.Errorf("APIURL: want env value 'https://env.example.com', got '%s'", cfg.APIURL)
		}
		if cfg.Account != "1" {
			t.Errorf("account: want profile value '1', got '%s'", cfg.Account)
		}
		if cfg.Token != "env-token" {
			t.Errorf("token: want env value 'env-token', got '%s'", cfg.Token)
		}
	})

	t.Run("flags beat env and profile", func(t *testing.T) {
		tempDir := t.TempDir()
		profileDir := t.TempDir()

		config.SetTestConfigDir(tempDir)
		config.SetTestWorkingDir(t.TempDir())
		defer config.ResetTestConfigDir()
		defer config.ResetTestWorkingDir()

		profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
		profileStore.Create(&profile.Profile{
			Name:    "acme",
			BaseURL: "https://profile.example.com",
		})
		profileStore.SetDefault("acme")

		t.Setenv("MAILPIECE_API_URL", "https://env.example.com")

		SetTestModeWithSDK(NewMockClient())
		SetTestProfiles(profileStore)
		defer resetTest()

		_, err := runCobraWithArgs("auth", "status", "--api-url", "https://flag.example.com")
		assertExitCode(t, err, 0)

		if cfg.APIURL != "https://flag.example.com" {
			t.Errorf("APIURL: want flag value 'https://flag.example.com', got '%s'", cfg.APIURL)
		}
		data := responseDataMap(t, lastResult)
		if data["account"] != "acme" {
			t.Errorf("account: want acme from the profile, got %v", data["account"])
		}
		if data["api_url"] != "https://flag.example.com" {
			t.Errorf("api_url: want flag value, got %v", data["api_url"])
		}
	})
}

// setWithToken turns on --with-token with the given stdin for one test.
func setWithToken(t *testing.T, stdin string) {
	t.Helper()
	if err := authLoginCmd.Flags().Set("with-token", "true"); err != nil {
		t.Fatal(err)
	}
	authLoginCmd.SetIn(strings.NewReader(stdin))
	t.Cleanup(func() {
		_ = authLoginCmd.Flags().Set("with-token", "false")
		authLoginCmd.SetIn(nil)
	})
}
