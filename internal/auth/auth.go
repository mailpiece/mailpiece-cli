// Package auth provides the credential store for personal access tokens.
package auth

import "github.com/basecamp/cli/credstore"

// ServiceName is the keyring service the CLI stores tokens under.
const ServiceName = "mailpiece"

// DisableKeyringEnvVar forces the file fallback when set.
const DisableKeyringEnvVar = "MAILPIECE_NO_KEYRING"

// NewCredentialStore creates a credential store for this CLI.
func NewCredentialStore(fallbackDir string) *credstore.Store {
	return credstore.NewStore(credstore.StoreOptions{
		ServiceName:   ServiceName,
		DisableEnvVar: DisableKeyringEnvVar,
		FallbackDir:   fallbackDir,
	})
}
