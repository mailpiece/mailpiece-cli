package commands

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
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

// tokenSource names where the effective token came from.
func tokenSource() string {
	switch {
	case cfgToken != "":
		return "flag"
	case os.Getenv("MAILPIECE_TOKEN") != "":
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
		if account != "" && account != name {
			return fmt.Errorf("profile store is unavailable for alias %q", name)
		}
		return nil
	}
	if err := profile.ValidateName(name); err != nil {
		return err
	}

	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return err
	}
	existing := allProfiles[name]

	if baseURL == "" && existing != nil {
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

	if existing == nil {
		return profiles.Create(p)
	}

	if err := profiles.Delete(name); err != nil {
		return err
	}
	if err := profiles.Create(p); err != nil {
		restoreErr := profiles.Create(existing)
		if restoreErr == nil && defaultName == name {
			restoreErr = profiles.SetDefault(name)
		}
		if restoreErr != nil {
			return fmt.Errorf("update profile %q: %w (restore failed: %w)", name, err, restoreErr)
		}
		return err
	}
	if defaultName == name {
		if err := profiles.SetDefault(name); err != nil {
			restoreErr := profiles.Delete(name)
			if restoreErr == nil {
				restoreErr = profiles.Create(existing)
			}
			if restoreErr == nil {
				restoreErr = profiles.SetDefault(name)
			}
			if restoreErr != nil {
				return stderrors.Join(
					fmt.Errorf("update profile %q: %w", name, err),
					fmt.Errorf("restore profile %q: %w", name, restoreErr),
				)
			}
			return err
		}
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

type credentialStateSnapshot struct {
	profileName string
	data        []byte
	exists      bool
}

func snapshotProfileCredential(profileName string) (credentialStateSnapshot, error) {
	snapshot := credentialStateSnapshot{profileName: profileName}
	if creds == nil {
		return snapshot, nil
	}
	data, err := creds.Load(profile.CredentialKey(profileName, ""))
	if err != nil {
		if isCredentialNotFound(err) {
			return snapshot, nil
		}
		return credentialStateSnapshot{}, err
	}
	snapshot.data = append([]byte(nil), data...)
	snapshot.exists = true
	return snapshot, nil
}

func restoreProfileCredential(snapshot credentialStateSnapshot) error {
	if creds == nil {
		return nil
	}
	key := profile.CredentialKey(snapshot.profileName, "")
	if snapshot.exists {
		return creds.Save(key, snapshot.data)
	}
	if err := creds.Delete(key); err != nil && !isCredentialNotFound(err) {
		return err
	}
	return nil
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

type profileCredentialSaveOptions struct {
	ProfileName            string
	Account                string
	BaseURL                string
	Token                  string
	AllowYAMLTokenFallback bool
	UpdateGlobal           func(*config.Config, bool)
}

// saveProfileCredentialState saves a login: the profile, made the default, its
// token, and the global config. A step that fails puts back what the earlier
// ones changed. The first value is a warning: the credential store refused the
// token and it went to the global config instead.
func saveProfileCredentialState(opts profileCredentialSaveOptions) (error, error) {
	var warning error
	if err := profile.ValidateName(opts.ProfileName); err != nil {
		return nil, err
	}
	if err := validateAccountIdentifier(opts.Account); err != nil {
		return nil, err
	}

	profileSnapshot, err := snapshotProfileState(opts.ProfileName)
	if err != nil {
		return nil, err
	}
	credentialSnapshot, err := snapshotProfileCredential(opts.ProfileName)
	if err != nil {
		return nil, err
	}
	globalSnapshot := snapshotGlobalConfig()

	rollback := func(operationErr error, restoreCredential, restoreGlobal bool) error {
		rollbackErrors := []error{operationErr}
		if restoreCredential {
			if restoreErr := restoreProfileCredential(credentialSnapshot); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore credential: %w", restoreErr))
			}
		}
		if restoreErr := restoreProfileState(profileSnapshot); restoreErr != nil {
			rollbackErrors = append(rollbackErrors, fmt.Errorf("restore profile: %w", restoreErr))
		}
		if restoreGlobal {
			if restoreErr := restoreGlobalConfig(globalSnapshot); restoreErr != nil {
				rollbackErrors = append(rollbackErrors, fmt.Errorf("restore global config: %w", restoreErr))
			}
		}
		return stderrors.Join(rollbackErrors...)
	}

	if err := ensureProfileForAccount(opts.ProfileName, opts.Account, opts.BaseURL); err != nil {
		return nil, err
	}
	if profiles != nil {
		if err := profiles.SetDefault(opts.ProfileName); err != nil {
			return nil, rollback(err, false, false)
		}
	}

	credentialStored := false
	if creds != nil {
		if err := credsSaveProfileToken(opts.ProfileName, opts.Token); err != nil {
			if opts.AllowYAMLTokenFallback && !credentialSnapshot.exists {
				// With no prior profile credential, the YAML fallback remains
				// unambiguous and can safely carry the new token.
				warning = err
			} else {
				if restoreErr := restoreProfileCredential(credentialSnapshot); restoreErr != nil {
					return nil, rollback(stderrors.Join(err, fmt.Errorf("restore credential: %w", restoreErr)), false, false)
				}
				return nil, rollback(err, false, false)
			}
		} else {
			credentialStored = true
		}
	}

	globalCfg := config.LoadGlobal()
	if opts.UpdateGlobal != nil {
		opts.UpdateGlobal(globalCfg, credentialStored)
	}
	if err := globalCfg.Save(); err != nil {
		return warning, rollback(err, credentialStored, true)
	}
	return warning, nil
}
