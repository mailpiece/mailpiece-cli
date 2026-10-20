package commands

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/zalando/go-keyring"
)

func TestValidateAccountIdentifier(t *testing.T) {
	for _, tt := range []struct {
		account string
		valid   bool
	}{
		{"834206594", true},
		{"acme", true},
		{"acme.staging_1~x-y", true},
		{"", false},
		{" acme", false},
		{"-acme", false},
		{".acme", false},
		{"../other", false},
		{"other/admin", false},
		{"other?admin=1", false},
		{"other%2Fadmin", false},
	} {
		t.Run(tt.account, func(t *testing.T) {
			err := validateAccountIdentifier(tt.account)
			if tt.valid && err != nil {
				t.Errorf("expected %q to be valid, got %v", tt.account, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("expected %q to be invalid", tt.account)
			}
		})
	}
}

func TestIsCredentialNotFound(t *testing.T) {
	store := newTestCredStore(t, t.TempDir())
	SetTestCreds(store)
	defer resetTest()

	_, missingErr := credsLoadProfileToken("nobody")
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"file store miss", missingErr, true},
		{"keyring miss", fmt.Errorf("load: %w", keyring.ErrNotFound), true},
		{"other failure", stderrors.New("permission denied"), false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCredentialNotFound(tt.err); got != tt.want {
				t.Errorf("isCredentialNotFound(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestCredsProfileTokenRoundTrip(t *testing.T) {
	store := newTestCredStore(t, t.TempDir())
	SetTestCreds(store)
	defer resetTest()

	if err := credsSaveProfileToken("acme", "acme-token"); err != nil {
		t.Fatalf("save: %v", err)
	}
	raw, err := store.Load("profile:acme")
	if err != nil {
		t.Fatalf("expected the token under 'profile:acme': %v", err)
	}
	if string(raw) != `"acme-token"` {
		t.Errorf("expected a JSON-encoded token, got %s", raw)
	}
	if token, err := credsLoadProfileToken("acme"); err != nil || token != "acme-token" {
		t.Errorf("load: got %q (%v)", token, err)
	}
	if err := credsDeleteProfileToken("acme"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := credsLoadProfileToken("acme"); !isCredentialNotFound(err) {
		t.Errorf("expected not found after delete, got %v", err)
	}
}

func TestTokenSource(t *testing.T) {
	t.Run("flag", func(t *testing.T) {
		SetTestConfig("flag-token", "acme", testAPIURL)
		cfgToken = "flag-token"
		t.Setenv("MAILPIECE_TOKEN", "env-token")
		defer resetTest()

		if got := tokenSource(); got != "flag" {
			t.Errorf("expected 'flag', got %q", got)
		}
	})

	t.Run("environment", func(t *testing.T) {
		SetTestConfig("env-token", "acme", testAPIURL)
		t.Setenv("MAILPIECE_TOKEN", "env-token")
		defer resetTest()

		if got := tokenSource(); got != "env" {
			t.Errorf("expected 'env', got %q", got)
		}
	})

	t.Run("credentials file", func(t *testing.T) {
		store := newTestCredStore(t, t.TempDir())
		saveTestToken(t, store, "acme", "cred-token")
		SetTestCreds(store)
		SetTestConfig("cred-token", "acme", testAPIURL)
		activeProfile = "acme"
		defer resetTest()

		if got := tokenSource(); got != "credentials file" {
			t.Errorf("expected 'credentials file', got %q", got)
		}
	})

	t.Run("config", func(t *testing.T) {
		SetTestCreds(newTestCredStore(t, t.TempDir()))
		SetTestConfig("yaml-token", "acme", testAPIURL)
		activeProfile = "acme"
		defer resetTest()

		if got := tokenSource(); got != "config" {
			t.Errorf("expected 'config', got %q", got)
		}
	})

	t.Run("none", func(t *testing.T) {
		SetTestConfig("", "acme", testAPIURL)
		defer resetTest()

		if got := tokenSource(); got != "" {
			t.Errorf("expected no source, got %q", got)
		}
	})
}

func TestRestoreProfileState(t *testing.T) {
	t.Run("removes a profile that did not exist and restores the default", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testAPIURL})
		SetTestProfiles(profileStore)
		defer resetTest()

		snapshot, err := snapshotProfileState("agent")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if snapshot.previous != nil || snapshot.previousDefault != "acme" {
			t.Fatalf("unexpected snapshot: %#v", snapshot)
		}

		profileStore.Create(&profile.Profile{Name: "agent", BaseURL: testAPIURL})
		profileStore.SetDefault("agent")

		if err := restoreProfileState(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if _, err := profileStore.Get("agent"); err == nil {
			t.Error("expected the new profile removed")
		}
		if _, defaultName, _ := profileStore.List(); defaultName != "acme" {
			t.Errorf("expected default 'acme', got %q", defaultName)
		}
	})

	t.Run("puts back a profile that was changed", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testAPIURL})
		profileStore.Create(&profile.Profile{
			Name:    "agent",
			BaseURL: "https://old.example.com",
			Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
		})
		SetTestProfiles(profileStore)
		defer resetTest()

		snapshot, err := snapshotProfileState("agent")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}

		if err := ensureProfileForAccount("agent", "2", "https://new.example.com"); err != nil {
			t.Fatalf("change profile: %v", err)
		}
		profileStore.SetDefault("agent")

		if err := restoreProfileState(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
		restored, err := profileStore.Get("agent")
		if err != nil {
			t.Fatalf("get restored profile: %v", err)
		}
		if restored.BaseURL != "https://old.example.com" || profileAccount("agent", restored) != "1" {
			t.Errorf("expected the old profile, got %q bound to %q", restored.BaseURL, profileAccount("agent", restored))
		}
		if _, defaultName, _ := profileStore.List(); defaultName != "acme" {
			t.Errorf("expected default 'acme', got %q", defaultName)
		}
	})

	t.Run("keeps a lone profile that was not the default off the default", func(t *testing.T) {
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
		profileStore.Create(&profile.Profile{Name: "first", BaseURL: testAPIURL})
		profileStore.Create(&profile.Profile{Name: "agent", BaseURL: "https://old.example.com"})
		profileStore.Delete("first")
		SetTestProfiles(profileStore)
		defer resetTest()

		snapshot, err := snapshotProfileState("agent")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if snapshot.previous == nil || snapshot.previousDefault != "" {
			t.Fatalf("unexpected snapshot: %#v", snapshot)
		}

		if err := ensureProfileForAccount("agent", "", "https://new.example.com"); err != nil {
			t.Fatalf("change profile: %v", err)
		}
		profileStore.SetDefault("agent")

		if err := restoreProfileState(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
		allProfiles, defaultName, err := profileStore.List()
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(allProfiles) != 1 || allProfiles["agent"] == nil {
			t.Fatalf("expected only 'agent' left, got %v", allProfiles)
		}
		if allProfiles["agent"].BaseURL != "https://old.example.com" {
			t.Errorf("expected the old base URL, got %q", allProfiles["agent"].BaseURL)
		}
		if defaultName != "" {
			t.Errorf("expected no default, got %q", defaultName)
		}
	})

	t.Run("does nothing without a profile store", func(t *testing.T) {
		defer resetTest()
		snapshot, err := snapshotProfileState("agent")
		if err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		if err := restoreProfileState(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
	})
}

func TestRestoreGlobalConfig(t *testing.T) {
	t.Run("deletes a config that did not exist", func(t *testing.T) {
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()

		snapshot := snapshotGlobalConfig()
		if err := (&config.Config{Account: "new"}).Save(); err != nil {
			t.Fatalf("save: %v", err)
		}

		if err := restoreGlobalConfig(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
		if config.Exists() {
			t.Error("expected the global config removed")
		}
	})

	t.Run("puts back the values a config had", func(t *testing.T) {
		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()
		if err := (&config.Config{Token: "old-token", Account: "old", APIURL: "https://old.example.com"}).Save(); err != nil {
			t.Fatalf("save: %v", err)
		}

		snapshot := snapshotGlobalConfig()
		if err := (&config.Config{Account: "new", APIURL: "https://new.example.com"}).Save(); err != nil {
			t.Fatalf("save: %v", err)
		}

		if err := restoreGlobalConfig(snapshot); err != nil {
			t.Fatalf("restore: %v", err)
		}
		restored := config.LoadGlobal()
		if restored.Token != "old-token" || restored.Account != "old" || restored.APIURL != "https://old.example.com" {
			t.Errorf("expected the old values, got %#v", restored)
		}
	})
}
