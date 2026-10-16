package commands

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/zalando/go-keyring"
)

var accountIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]*$`)

type profileAccountBinding struct {
	Account  string
	Explicit bool
}

// resolveProfile uses profile.Resolve() to determine the active profile,
// then applies its BaseURL and account to cfg.
//
// Profile settings (layer 3) outrank the global YAML config (layer 4) but
// yield to env vars (layer 2) and flags (layer 1).
//
// Returns an error when the user explicitly selected a profile (via flag
// or env var) that doesn't exist.
func resolveProfile() error {
	activeProfile = ""
	activeProfileExplicitAccount = false
	selected := firstNonEmpty(cfgProfile, config.Env("MAILPIECE_PROFILE"))
	if profiles == nil {
		useSelectedAsAccount(selected)
		return nil
	}

	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return err
	}
	if len(allProfiles) == 0 {
		useSelectedAsAccount(selected)
		return nil
	}

	resolved, err := profile.Resolve(profile.ResolveOptions{
		FlagValue:      cfgProfile,
		EnvVar:         config.Env("MAILPIECE_PROFILE"),
		DefaultProfile: defaultName,
		Profiles:       allProfiles,
	})
	if err != nil {
		if selected != "" {
			return err
		}
		return nil
	}
	if resolved == "" {
		return nil
	}

	p := allProfiles[resolved]
	if p == nil {
		return nil
	}

	binding, err := resolveProfileAccountBinding(resolved, p)
	if err != nil {
		return err
	}
	activeProfile = resolved
	activeProfileExplicitAccount = binding.Explicit
	if config.Env("MAILPIECE_ACCOUNT") == "" {
		cfg.Account = binding.Account
		accountSource = "profile"
	}
	if p.BaseURL != "" && config.Env("MAILPIECE_API_URL") == "" {
		cfg.APIURL = p.BaseURL
	}
	return nil
}

// useSelectedAsAccount takes a selected profile name as both profile and
// account while no profile is saved, until one with account metadata exists.
func useSelectedAsAccount(selected string) {
	if selected == "" {
		return
	}
	activeProfile = selected
	if config.Env("MAILPIECE_ACCOUNT") == "" {
		cfg.Account = selected
		accountSource = "profile"
	}
}

// resolveToken applies token precedence: YAML → credstore → env → flag.
func resolveToken() {
	if creds != nil && activeProfile != "" {
		if t, err := credsLoadProfileToken(activeProfile); err == nil && t != "" {
			cfg.Token = t
		}
	}
	if t := config.Env("MAILPIECE_TOKEN"); t != "" {
		cfg.Token = t
	}
	if cfgToken != "" {
		cfg.Token = cfgToken
	}
}

// tokenSource names where the effective token came from.
func tokenSource() string {
	switch {
	case cfgToken != "":
		return "flag"
	case config.Env("MAILPIECE_TOKEN") != "":
		return "env"
	}
	if creds != nil && activeProfile != "" {
		if t, err := credsLoadProfileToken(activeProfile); err == nil && t != "" {
			if creds.UsingKeyring() {
				return "keyring"
			}
			return "credentials file"
		}
	}
	if cfg != nil && cfg.Token != "" {
		return "config"
	}
	return ""
}

func resolveProfileAccountBinding(name string, p *profile.Profile) (profileAccountBinding, error) {
	var raw json.RawMessage
	present := false
	if p != nil && p.Extra != nil {
		raw, present = p.Extra["account"]
	}
	if !present {
		if err := validateAccountIdentifier(name); err != nil {
			return profileAccountBinding{}, fmt.Errorf("profile %q: %w", name, err)
		}
		return profileAccountBinding{Account: name}, nil
	}

	var account string
	if err := json.Unmarshal(raw, &account); err != nil || strings.TrimSpace(account) == "" {
		return profileAccountBinding{}, fmt.Errorf("profile %q has invalid account metadata: account must be a non-empty string", name)
	}
	account = strings.TrimSpace(account)
	if err := validateAccountIdentifier(account); err != nil {
		return profileAccountBinding{}, fmt.Errorf("profile %q has invalid account metadata: %w", name, err)
	}
	return profileAccountBinding{Account: account, Explicit: true}, nil
}

func validateAccountIdentifier(account string) error {
	if !accountIdentifierPattern.MatchString(account) {
		return fmt.Errorf("invalid account %q: use a slug containing only letters, numbers, periods, underscores, tildes, or hyphens", account)
	}
	return nil
}

// ensureProfileForAccount creates or replaces a profile bound to an account.
// An account equal to the profile name is stored implicitly.
func ensureProfileForAccount(name, account, baseURL string) error {
	if profiles == nil {
		return fmt.Errorf("profile store is unavailable")
	}
	if err := profile.ValidateName(name); err != nil {
		return err
	}

	allProfiles, _, err := profiles.List()
	if err != nil {
		return err
	}
	existing := allProfiles[name]

	if baseURL == "" {
		if existing == nil || existing.BaseURL == "" {
			return errNoAPIURL()
		}
		baseURL = existing.BaseURL
	}

	extra := map[string]json.RawMessage{}
	if existing != nil {
		for k, v := range existing.Extra {
			extra[k] = v
		}
	}
	if account != "" {
		if account == name {
			delete(extra, "account")
		} else {
			b, _ := json.Marshal(account)
			extra["account"] = b
		}
	}

	p := &profile.Profile{Name: name, BaseURL: baseURL}
	if len(extra) > 0 {
		p.Extra = extra
	}

	if existing != nil {
		if err := profiles.Delete(name); err != nil {
			return err
		}
	}
	if err := profiles.Create(p); err != nil {
		if existing != nil {
			if restoreErr := profiles.Create(existing); restoreErr != nil {
				return stderrors.Join(err, restoreErr)
			}
		}
		return err
	}
	return nil
}

// credsSaveProfileToken JSON-encodes a token and saves it to the credential store
// under a profile-scoped key ("profile:<name>").
func credsSaveProfileToken(profileName, token string) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return creds.Save(profile.CredentialKey(profileName, ""), data)
}

// credsLoadProfileToken loads and JSON-decodes a token for a profile.
func credsLoadProfileToken(profileName string) (string, error) {
	data, err := creds.Load(profile.CredentialKey(profileName, ""))
	if err != nil {
		return "", err
	}
	var token string
	if json.Unmarshal(data, &token) == nil {
		return token, nil
	}
	return string(data), nil
}

// credsDeleteProfileToken removes the token for a profile.
func credsDeleteProfileToken(profileName string) error {
	return creds.Delete(profile.CredentialKey(profileName, ""))
}

func isCredentialNotFound(err error) bool {
	if err == nil {
		return false
	}
	return stderrors.Is(err, keyring.ErrNotFound) || strings.Contains(err.Error(), "credentials not found for ")
}

type profileStateSnapshot struct {
	name            string
	previous        *profile.Profile
	previousDefault string
}

func snapshotProfileState(name string) (profileStateSnapshot, error) {
	snapshot := profileStateSnapshot{name: name}
	if profiles == nil {
		return snapshot, nil
	}
	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return profileStateSnapshot{}, err
	}
	snapshot.previous = allProfiles[name]
	snapshot.previousDefault = defaultName
	return snapshot, nil
}

func restoreProfileState(snapshot profileStateSnapshot) error {
	if profiles == nil {
		return nil
	}
	allProfiles, _, err := profiles.List()
	if err != nil {
		return err
	}
	if _, exists := allProfiles[snapshot.name]; exists {
		if err := profiles.Delete(snapshot.name); err != nil {
			return err
		}
	}
	if snapshot.previous != nil {
		// The store makes the first profile created the default, so a profile
		// that was not the default is recreated beside a placeholder.
		if snapshot.previousDefault == "" && len(allProfiles) == 1 {
			temporaryName := "mailpiece-restore"
			for suffix := 2; ; suffix++ {
				if _, exists := allProfiles[temporaryName]; !exists {
					break
				}
				temporaryName = fmt.Sprintf("mailpiece-restore-%d", suffix)
			}
			if err := profiles.Create(&profile.Profile{Name: temporaryName, BaseURL: snapshot.previous.BaseURL}); err != nil {
				return err
			}
			if err := profiles.Create(snapshot.previous); err != nil {
				return err
			}
			return profiles.Delete(temporaryName)
		}
		if err := profiles.Create(snapshot.previous); err != nil {
			return err
		}
	}
	if snapshot.previousDefault != "" {
		return profiles.SetDefault(snapshot.previousDefault)
	}
	return nil
}

type globalConfigSnapshot struct {
	config *config.Config
	exists bool
}

func snapshotGlobalConfig() globalConfigSnapshot {
	loaded := config.LoadGlobal()
	configCopy := *loaded
	return globalConfigSnapshot{config: &configCopy, exists: config.Exists()}
}

func restoreGlobalConfig(snapshot globalConfigSnapshot) error {
	if !snapshot.exists {
		return config.Delete()
	}
	return snapshot.config.Save()
}
