// Package config handles configuration loading for the Mailpiece CLI.
package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LocalConfigFile is the name of the local project config file.
// Mailpiece is self-hosted, so there is no default API URL: it comes from a
// flag, the environment, a profile or a config file.
const LocalConfigFile = ".mailpiece.yaml"

// testConfigDir is used to override global config directory for testing.
// When set, all global config operations use this directory instead of ~/.mailpiece
var testConfigDir string

// testWorkingDir is used to override working directory for testing local config.
var testWorkingDir string

// SetTestConfigDir sets a custom global config directory for testing.
func SetTestConfigDir(dir string) {
	testConfigDir = dir
}

// ResetTestConfigDir resets the global config directory to default.
func ResetTestConfigDir() {
	testConfigDir = ""
}

// SetTestWorkingDir sets a custom working directory for testing local config.
func SetTestWorkingDir(dir string) {
	testWorkingDir = dir
}

// ResetTestWorkingDir resets the working directory to default.
func ResetTestWorkingDir() {
	testWorkingDir = ""
}

// Config holds the CLI configuration.
type Config struct {
	Token   string `yaml:"token"`
	Account string `yaml:"account"`
	APIURL  string `yaml:"api_url"`
}

// globalConfigPaths returns the possible global configuration file paths in order of preference.
func globalConfigPaths() []string {
	if testConfigDir != "" {
		return []string{filepath.Join(testConfigDir, "config.yaml")}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".config", "mailpiece", "config.yaml"),
		filepath.Join(home, ".mailpiece", "config.yaml"),
	}
}

// GlobalConfigPaths returns the possible global configuration file paths in order of preference.
func GlobalConfigPaths() []string {
	paths := globalConfigPaths()
	out := make([]string, len(paths))
	copy(out, paths)
	return out
}

// findLocalConfig walks up the directory tree looking for .mailpiece.yaml
func findLocalConfig() string {
	var startDir string
	if testWorkingDir != "" {
		startDir = testWorkingDir
	} else {
		var err error
		startDir, err = os.Getwd()
		if err != nil {
			return ""
		}
	}

	dir := startDir
	for {
		configPath := filepath.Join(dir, LocalConfigFile)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	return ""
}

// Load loads configuration from files and environment variables.
// Full precedence (highest to lowest):
//
//  1. CLI flags (--token, --profile, --api-url)
//  2. Environment variables (MAILPIECE_TOKEN, MAILPIECE_PROFILE, MAILPIECE_API_URL)
//  3. Named profile settings (account and BaseURL from config.json)
//  4. Local project config (.mailpiece.yaml)
//  5. Global config (~/.config/mailpiece/config.yaml)
//
// Load() handles layers 2, 4 and 5. Profile resolution (layer 3) and flag
// application (layer 1) happen afterward in root.go's PersistentPreRunE.
// Because profiles outrank local/global config, resolveProfile() intentionally
// overwrites values that Load() set from YAML, but preserves env var values.
//
// Local config (.mailpiece.yaml) is searched for in the current directory and parent
// directories. Values from local config override global config values.
func Load() *Config {
	cfg := &Config{}

	// Load from global config file first
	for _, path := range globalConfigPaths() {
		if data, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(data, cfg)
			break
		}
	}

	// Override with local config (walks up directory tree)
	if localPath := findLocalConfig(); localPath != "" {
		if data, err := os.ReadFile(localPath); err == nil {
			var localCfg Config
			if yaml.Unmarshal(data, &localCfg) == nil {
				// Only override non-empty values from local config
				if localCfg.Token != "" {
					cfg.Token = localCfg.Token
				}
				if localCfg.Account != "" {
					cfg.Account = localCfg.Account
				}
				if localCfg.APIURL != "" {
					cfg.APIURL = localCfg.APIURL
				}
			}
		}
	}

	// Override with environment variables
	if token := os.Getenv("MAILPIECE_TOKEN"); token != "" {
		cfg.Token = token
	}
	if account := os.Getenv("MAILPIECE_ACCOUNT"); account != "" {
		cfg.Account = account
	}
	if apiURL := os.Getenv("MAILPIECE_API_URL"); apiURL != "" {
		cfg.APIURL = apiURL
	}

	ensureAPIURL(cfg)
	return cfg
}

// LoadGlobal loads configuration only from the global config file(s).
// It does not apply local project config or environment variables.
func LoadGlobal() *Config {
	cfg := &Config{}
	for _, path := range globalConfigPaths() {
		if data, err := os.ReadFile(path); err == nil {
			_ = yaml.Unmarshal(data, cfg)
			break
		}
	}
	ensureAPIURL(cfg)
	return cfg
}

func ensureAPIURL(cfg *Config) {
	if cfg == nil {
		return
	}
	cfg.APIURL = strings.TrimSpace(cfg.APIURL)

	// Warn about non-HTTPS URLs (except localhost/loopback for development)
	if strings.HasPrefix(cfg.APIURL, "http://") {
		if u, err := url.Parse(cfg.APIURL); err == nil {
			host := u.Hostname() // strips port and IPv6 brackets
			if host != "localhost" && host != "127.0.0.1" && host != "::1" {
				fmt.Fprintf(os.Stderr, "Warning: API URL %q uses HTTP. HTTPS is recommended for security.\n", cfg.APIURL)
			}
		}
	}
}

// ConfigPath returns the path to the global config file (creating directory if needed).
func ConfigPath() (string, error) {
	paths := globalConfigPaths()
	if len(paths) == 0 {
		return "", fmt.Errorf("unable to determine config path")
	}

	// Prefer an existing config file path (so we don't end up with multiple global configs).
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return "", err
			}
			return path, nil
		}
	}

	// Otherwise, use the preferred path.
	preferred := paths[0]
	if err := os.MkdirAll(filepath.Dir(preferred), 0700); err != nil {
		return "", err
	}
	return preferred, nil
}

// StateDir returns the directory where Mailpiece stores command state.
func StateDir() (string, error) {
	if testConfigDir != "" {
		if err := os.MkdirAll(testConfigDir, 0o700); err != nil {
			return "", err
		}
		return testConfigDir, nil
	}

	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "state")
	}
	dir := filepath.Join(base, "mailpiece")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// Save saves the configuration to the global config file.
func (c *Config) Save() error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// Delete removes the global config file.
func Delete() error {
	for _, path := range globalConfigPaths() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// Exists checks if a global config file exists.
func Exists() bool {
	for _, path := range globalConfigPaths() {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// LocalConfigPath returns the path to the local config file if found.
// Returns empty string if no local config exists.
func LocalConfigPath() string {
	return findLocalConfig()
}

// SaveLocal saves the configuration to a local .mailpiece.yaml file in the current directory.
func (c *Config) SaveLocal() error {
	var dir string
	if testWorkingDir != "" {
		dir = testWorkingDir
	} else {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(dir, LocalConfigFile), data, 0600)
}
