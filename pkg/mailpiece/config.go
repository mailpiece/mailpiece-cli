package mailpiece

import (
	"os"
	"strings"
)

// Config holds the resolved configuration for API access.
type Config struct {
	// BaseURL is the API base URL of a Mailpiece install (e.g., "https://mailpiece.example.com").
	// Mailpiece is self-hosted, so there is no default.
	BaseURL string `json:"base_url"`

	// Account is the default account slug.
	Account string `json:"account"`
}

// LoadConfigFromEnv loads configuration from environment variables.
// Environment variables override any values already set in the config.
func (c *Config) LoadConfigFromEnv() {
	if v := os.Getenv("MAILPIECE_API_URL"); v != "" {
		c.BaseURL = v
	}
	if v := os.Getenv("MAILPIECE_ACCOUNT"); v != "" {
		c.Account = v
	}
}

// NormalizeBaseURL ensures consistent URL format (no trailing slash).
func NormalizeBaseURL(url string) string {
	return strings.TrimSuffix(url, "/")
}

// NormalizeAccountSlug strips the leading slash the API renders on an account's slug.
func NormalizeAccountSlug(slug string) string {
	return strings.Trim(slug, "/")
}
