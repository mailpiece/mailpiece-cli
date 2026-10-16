package commands

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/basecamp/cli/output"
	"github.com/charmbracelet/lipgloss"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/harness"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DoctorCheck represents a single diagnostic check result.
type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // pass, warn, fail, skip
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// DoctorProfileResult holds diagnostic results for a single profile.
type DoctorProfileResult struct {
	Name    string        `json:"name"`
	Default bool          `json:"default,omitempty"`
	Checks  []DoctorCheck `json:"checks"`
	Passed  int           `json:"passed"`
	Failed  int           `json:"failed"`
	Warned  int           `json:"warned"`
	Skipped int           `json:"skipped"`
}

// DoctorResult holds the complete diagnostic results.
type DoctorResult struct {
	Checks   []DoctorCheck         `json:"checks"`
	Profiles []DoctorProfileResult `json:"profiles,omitempty"`
	Passed   int                   `json:"passed"`
	Failed   int                   `json:"failed"`
	Warned   int                   `json:"warned"`
	Skipped  int                   `json:"skipped"`
}

func (r *DoctorResult) Summary() string {
	if r.Failed == 0 && r.Warned == 0 && r.Passed > 0 {
		if r.Skipped > 0 {
			return fmt.Sprintf("All %d checks passed, %d skipped", r.Passed, r.Skipped)
		}
		return fmt.Sprintf("All %d checks passed", r.Passed)
	}
	parts := []string{}
	if r.Passed > 0 {
		parts = append(parts, fmt.Sprintf("%d passed", r.Passed))
	}
	if r.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", r.Failed))
	}
	if r.Warned > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", r.Warned, pluralize(r.Warned, "warning", "warnings")))
	}
	if r.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", r.Skipped))
	}
	return strings.Join(parts, ", ")
}

type doctorEffectiveConfig struct {
	ProfileName    string
	Default        bool
	ProfileSource  string
	Account        string
	AccountSource  string
	APIURL         string
	APIURLSource   string
	Token          string
	TokenSource    string
	TokenSourceRaw string
	ConfigError    string
}

func NewDoctorCmd() *cobra.Command {
	var verbose bool
	var allProfiles bool

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check CLI health and diagnose issues",
		Long: `Run diagnostic checks on installation health, configuration, authentication, API connectivity,
account access, shell integration, and coding-agent setup.

Use --profile NAME to check a specific saved profile, or --all-profiles to sweep every saved
profile in addition to the global install checks.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if allProfiles && cfgProfile != "" {
				return errors.NewInvalidArgsError("--all-profiles cannot be used with --profile")
			}
			if allProfiles && cfgToken != "" {
				return errors.NewInvalidArgsError("--all-profiles cannot be used with --token")
			}
			if allProfiles && cfgAPIURL != "" {
				return errors.NewInvalidArgsError("--all-profiles cannot be used with --api-url")
			}
			if allProfiles && cfgAccount != "" {
				return errors.NewInvalidArgsError("--all-profiles cannot be used with --account")
			}

			result := runDoctor(cmd.Context(), verbose, allProfiles)
			breadcrumbs := buildDoctorBreadcrumbs(flattenDoctorChecks(result))

			switch out.EffectiveFormat() {
			case output.FormatStyled:
				renderDoctorStyled(outWriter, result, breadcrumbs)
				captureResponse()
				return nil
			case output.FormatMarkdown:
				renderDoctorMarkdown(outWriter, result, breadcrumbs)
				captureResponse()
				return nil
			default:
				opts := []output.ResponseOption{output.WithSummary(result.Summary())}
				if len(breadcrumbs) > 0 {
					opts = append(opts, output.WithBreadcrumbs(breadcrumbs...))
				}
				recordOutputError(out.OK(result, opts...))
				captureResponse()
				return nil
			}
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show additional diagnostic detail")
	cmd.Flags().BoolVar(&allProfiles, "all-profiles", false, "Run profile health checks for every saved profile")
	return cmd
}

func init() {
	rootCmd.AddCommand(NewDoctorCmd())
}

func runDoctor(ctx context.Context, verbose, allProfiles bool) *DoctorResult {
	result := &DoctorResult{Checks: runDoctorGlobalChecks(verbose)}
	if allProfiles {
		for _, target := range doctorTargetsFromProfileStore() {
			profileResult := DoctorProfileResult{
				Name:    target.ProfileName,
				Default: target.Default,
				Checks:  runDoctorTargetChecks(ctx, target, verbose),
			}
			summarizeDoctorProfile(&profileResult)
			result.Profiles = append(result.Profiles, profileResult)
		}
		if len(result.Profiles) == 0 {
			result.Checks = append(result.Checks, DoctorCheck{
				Name:    "Profile Sweep",
				Status:  "skip",
				Message: "No saved profiles to check",
				Hint:    "Run: mailpiece auth login <token>",
			})
		}
	} else {
		result.Checks = append(result.Checks, runDoctorTargetChecks(ctx, resolveDoctorEffectiveConfig(), verbose)...)
	}
	summarizeDoctorResult(result)
	return result
}

func runDoctorGlobalChecks(verbose bool) []DoctorCheck {
	checks := []DoctorCheck{checkDoctorVersion(verbose)}
	if verbose {
		checks = append(checks, checkDoctorRuntime())
	}
	checks = append(checks,
		checkDoctorGlobalConfig(verbose),
		checkDoctorLocalConfig(verbose),
		checkDoctorProfileStore(verbose),
		checkDoctorSavedProfiles(),
		checkDoctorFilesystem(verbose),
		checkDoctorShellCompletion(verbose),
		checkDoctorSkillInstallation(),
	)
	if baselineSkillInstalled() {
		checks = append(checks, checkDoctorSkillVersion())
	}
	for _, agent := range harness.DetectedAgents() {
		if agent.Checks == nil {
			continue
		}
		for _, c := range agent.Checks() {
			status := c.Status
			if status == "fail" && isOptionalAgentCheck(c.Name) {
				status = "warn"
			}
			checks = append(checks, DoctorCheck{Name: c.Name, Status: status, Message: c.Message, Hint: c.Hint})
		}
	}
	return checks
}

func runDoctorTargetChecks(ctx context.Context, eff doctorEffectiveConfig, verbose bool) []DoctorCheck {
	if eff.ConfigError != "" {
		return []DoctorCheck{
			{Name: "Effective Config", Status: "fail", Message: eff.ConfigError},
			{Name: "Credentials", Status: "skip", Message: "Skipped (invalid profile configuration)"},
			{Name: "API Reachability", Status: "skip", Message: "Skipped (invalid profile configuration)"},
			{Name: "Authentication", Status: "skip", Message: "Skipped (invalid profile configuration)"},
			{Name: "Account Access", Status: "skip", Message: "Skipped (invalid profile configuration)"},
		}
	}
	credCheck := checkDoctorCredentials(eff)
	checks := []DoctorCheck{
		checkDoctorEffectiveConfig(eff, verbose),
		credCheck,
		checkDoctorCredentialStorage(eff),
		checkDoctorAPIURL(eff),
	}

	reachabilityCheck := checkDoctorAPIReachability(ctx, eff, verbose)
	checks = append(checks, reachabilityCheck)

	switch {
	case credCheck.Status == "fail":
		authMsg := "Skipped (missing credentials)"
		checks = append(checks,
			DoctorCheck{Name: "Authentication", Status: "skip", Message: authMsg},
			DoctorCheck{Name: "Account Access", Status: "skip", Message: authMsg},
		)
	case reachabilityCheck.Status != "pass":
		authMsg := "Skipped (API unreachable)"
		checks = append(checks,
			DoctorCheck{Name: "Authentication", Status: "skip", Message: authMsg},
			DoctorCheck{Name: "Account Access", Status: "skip", Message: authMsg},
		)
	default:
		authCheck, identity := checkDoctorAuthentication(ctx, eff, verbose)
		checks = append(checks, authCheck)
		if authCheck.Status == "pass" {
			checks = append(checks, checkDoctorAccountAccess(ctx, eff, identity, verbose))
		} else {
			checks = append(checks, DoctorCheck{Name: "Account Access", Status: "skip", Message: "Skipped (authentication failed)"})
		}
	}
	return checks
}

func summarizeDoctorProfile(result *DoctorProfileResult) {
	result.Passed, result.Failed, result.Warned, result.Skipped = countDoctorChecks(result.Checks)
}

func summarizeDoctorResult(result *DoctorResult) {
	result.Passed, result.Failed, result.Warned, result.Skipped = countDoctorChecks(flattenDoctorChecks(result))
}

func countDoctorChecks(checks []DoctorCheck) (passed, failed, warned, skipped int) {
	for _, c := range checks {
		switch c.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		case "warn":
			warned++
		case "skip":
			skipped++
		}
	}
	return passed, failed, warned, skipped
}

func flattenDoctorChecks(result *DoctorResult) []DoctorCheck {
	checks := append([]DoctorCheck{}, result.Checks...)
	for _, profileResult := range result.Profiles {
		checks = append(checks, profileResult.Checks...)
	}
	return checks
}

func checkDoctorVersion(verbose bool) DoctorCheck {
	check := DoctorCheck{Name: "CLI Version", Status: "pass", Message: currentVersion()}
	if verbose {
		if exe, err := os.Executable(); err == nil {
			check.Message = fmt.Sprintf("%s (%s)", currentVersion(), exe)
		}
	}
	return check
}

func checkDoctorRuntime() DoctorCheck {
	return DoctorCheck{
		Name:    "Runtime",
		Status:  "pass",
		Message: fmt.Sprintf("Go %s (%s/%s)", runtime.Version(), runtime.GOOS, runtime.GOARCH),
	}
}

func checkDoctorGlobalConfig(verbose bool) DoctorCheck {
	paths := config.GlobalConfigPaths()
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return validateDoctorConfigFile(path, "Global Config", verbose)
		}
	}
	pathHint := "~/.config/mailpiece/config.yaml"
	if len(paths) > 0 {
		pathHint = paths[0]
	}
	return DoctorCheck{
		Name:    "Global Config",
		Status:  "warn",
		Message: "Not found (using defaults)",
		Hint:    fmt.Sprintf("Create %s or run: mailpiece setup", pathHint),
	}
}

func checkDoctorLocalConfig(verbose bool) DoctorCheck {
	path := config.LocalConfigPath()
	if path == "" {
		return DoctorCheck{Name: "Local Config", Status: "skip", Message: "Not found"}
	}
	return validateDoctorConfigFile(path, "Local Config", verbose)
}

func validateDoctorConfigFile(path, name string, verbose bool) DoctorCheck {
	data, err := os.ReadFile(path)
	if err != nil {
		return DoctorCheck{
			Name:    name,
			Status:  "fail",
			Message: fmt.Sprintf("Cannot read %s", path),
			Hint:    err.Error(),
		}
	}
	var parsed map[string]any
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		return DoctorCheck{
			Name:    name,
			Status:  "fail",
			Message: fmt.Sprintf("Invalid YAML: %s", path),
			Hint:    err.Error(),
		}
	}
	msg := path
	if verbose {
		msg = fmt.Sprintf("%s (%d keys)", path, len(parsed))
	}
	return DoctorCheck{Name: name, Status: "pass", Message: msg}
}

func checkDoctorProfileStore(verbose bool) DoctorCheck {
	if profiles == nil {
		return DoctorCheck{
			Name:    "Profile Store",
			Status:  "warn",
			Message: "Profile store unavailable",
			Hint:    "Run: mailpiece setup or mailpiece auth login <token>",
		}
	}
	allProfiles, _, err := profiles.List()
	if err != nil {
		return DoctorCheck{
			Name:    "Profile Store",
			Status:  "fail",
			Message: "Cannot read profile store",
			Hint:    err.Error(),
		}
	}
	if len(allProfiles) == 0 {
		return DoctorCheck{
			Name:    "Profile Store",
			Status:  "warn",
			Message: "No named profiles configured",
			Hint:    "Run: mailpiece setup or mailpiece auth login <token>",
		}
	}
	msg := fmt.Sprintf("%d profile(s) configured", len(allProfiles))
	if verbose {
		msg += " [config.json]"
	}
	return DoctorCheck{
		Name:    "Profile Store",
		Status:  "pass",
		Message: msg,
		Hint:    "Use: mailpiece doctor --profile NAME or mailpiece doctor --all-profiles",
	}
}

func checkDoctorSavedProfiles() DoctorCheck {
	if profiles == nil {
		return DoctorCheck{Name: "Saved Profiles", Status: "skip", Message: "Profile store unavailable"}
	}
	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return DoctorCheck{Name: "Saved Profiles", Status: "skip", Message: "Unavailable", Hint: err.Error()}
	}
	if len(allProfiles) == 0 {
		return DoctorCheck{Name: "Saved Profiles", Status: "skip", Message: "No saved profiles"}
	}
	names := make([]string, 0, len(allProfiles))
	for name := range allProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if name == defaultName {
			names[i] = name + " (default)"
		}
	}
	return DoctorCheck{
		Name:    "Saved Profiles",
		Status:  "pass",
		Message: strings.Join(names, ", "),
		Hint:    "Use: mailpiece doctor --all-profiles to check each saved profile",
	}
}

// resolveDoctorEffectiveConfig describes the configuration the current
// invocation runs with, after root.go has applied every layer.
func resolveDoctorEffectiveConfig() doctorEffectiveConfig {
	eff := doctorEffectiveConfig{
		ProfileName: activeProfile,
		Account:     cfg.Account,
		APIURL:      cfg.APIURL,
	}

	globalCfg, _ := loadDoctorConfigFile(globalConfigPathForDoctor())
	localCfg, _ := loadDoctorConfigFile(config.LocalConfigPath())
	_, defaultName := profileStoreInfo()
	eff.Default = activeProfile != "" && activeProfile == defaultName

	switch {
	case activeProfile == "":
		eff.ProfileSource = "unset"
	case cfgProfile != "":
		eff.ProfileSource = "flag --profile"
	case config.Env("MAILPIECE_PROFILE") != "":
		eff.ProfileSource = "env MAILPIECE_PROFILE"
	default:
		eff.ProfileSource = "profile store"
	}

	switch accountSource {
	case "flag":
		eff.AccountSource = "flag --account"
	case "env":
		eff.AccountSource = "env MAILPIECE_ACCOUNT"
	case "profile":
		eff.AccountSource = "profile store"
	case "identity":
		eff.AccountSource = "token's only account"
	case "address":
		eff.AccountSource = "API URL " + configuredAPIURL
	case "config":
		if localCfg != nil && localCfg.Account != "" {
			eff.AccountSource = "local config"
		} else {
			eff.AccountSource = "global config"
		}
	default:
		eff.AccountSource = "unset"
	}

	profileURL := ""
	if activeProfile != "" && profiles != nil {
		if p, err := profiles.Get(activeProfile); err == nil && p != nil {
			profileURL = p.BaseURL
		}
	}
	switch {
	case cfgAPIURL != "":
		eff.APIURLSource = "flag --api-url"
	case config.Env("MAILPIECE_API_URL") != "":
		eff.APIURLSource = "env MAILPIECE_API_URL"
	case profileURL != "":
		eff.APIURLSource = "profile store"
	case localCfg != nil && localCfg.APIURL != "":
		eff.APIURLSource = "local config"
	case globalCfg != nil && globalCfg.APIURL != "":
		eff.APIURLSource = "global config"
	default:
		eff.APIURLSource = "none"
	}

	eff.TokenSourceRaw, eff.TokenSource, eff.Token = doctorTokenSourceWithValue(activeProfile, localCfg, globalCfg)
	return eff
}

// doctorTokenSourceWithValue follows resolveToken's precedence: flag, env,
// the profile's stored credential, then local and global config.
func doctorTokenSourceWithValue(profileName string, localCfg, globalCfg *config.Config) (string, string, string) {
	if cfgToken != "" {
		return "flag", "CLI flag", cfgToken
	}
	if envToken := config.Env("MAILPIECE_TOKEN"); envToken != "" {
		return "env", "environment variable", envToken
	}
	return doctorStoredTokenSourceForProfile(profileName, localCfg, globalCfg)
}

func doctorStoredTokenSourceForProfile(profileName string, localCfg, globalCfg *config.Config) (string, string, string) {
	if creds != nil && profileName != "" {
		if token, err := credsLoadProfileToken(profileName); err == nil && token != "" {
			if creds.UsingKeyring() {
				return "keyring", "system keyring", token
			}
			return "fallback-file", "fallback credential file", token
		}
	}
	if localCfg != nil && localCfg.Token != "" {
		return "local-config", "local config file", localCfg.Token
	}
	if globalCfg != nil && globalCfg.Token != "" {
		return "global-config", "global config file", globalCfg.Token
	}
	return "none", "not configured", ""
}

// doctorTargetsFromProfileStore builds one target per saved profile from what
// the store and config files hold; flags and environment are left out.
func doctorTargetsFromProfileStore() []doctorEffectiveConfig {
	allProfiles, defaultName := profileStoreInfo()
	if len(allProfiles) == 0 {
		return nil
	}
	globalCfg, _ := loadDoctorConfigFile(globalConfigPathForDoctor())
	localCfg, _ := loadDoctorConfigFile(config.LocalConfigPath())

	names := make([]string, 0, len(allProfiles))
	for name := range allProfiles {
		names = append(names, name)
	}
	sort.Strings(names)

	targets := make([]doctorEffectiveConfig, 0, len(names))
	for _, name := range names {
		p := allProfiles[name]
		binding, bindingErr := resolveProfileAccountBinding(name, p)
		tokenRaw, tokenSource, token := doctorStoredTokenSourceForProfile(name, localCfg, globalCfg)
		apiURL := ""
		apiURLSource := "none"
		switch {
		case p != nil && strings.TrimSpace(p.BaseURL) != "":
			apiURL = p.BaseURL
			apiURLSource = "profile store"
		case localCfg != nil && strings.TrimSpace(localCfg.APIURL) != "":
			apiURL = localCfg.APIURL
			apiURLSource = "local config"
		case globalCfg != nil && strings.TrimSpace(globalCfg.APIURL) != "":
			apiURL = globalCfg.APIURL
			apiURLSource = "global config"
		}
		configError := ""
		if bindingErr != nil {
			configError = bindingErr.Error()
		}
		if address, account := splitAccountAddress(apiURL); account != "" {
			if bindingErr == nil && account != binding.Account {
				configError = fmt.Sprintf("The Mailpiece address %s names the account %s, but the account is set to %s", apiURL, account, binding.Account)
			}
			apiURL = address
		}
		targets = append(targets, doctorEffectiveConfig{
			ProfileName:    name,
			Default:        name == defaultName,
			ProfileSource:  "profile store",
			Account:        binding.Account,
			AccountSource:  "profile store",
			APIURL:         apiURL,
			APIURLSource:   apiURLSource,
			Token:          token,
			TokenSourceRaw: tokenRaw,
			TokenSource:    tokenSource,
			ConfigError:    configError,
		})
	}
	return targets
}

func checkDoctorEffectiveConfig(eff doctorEffectiveConfig, verbose bool) DoctorCheck {
	parts := []string{}
	if eff.ProfileName != "" {
		if verbose {
			parts = append(parts, fmt.Sprintf("profile=%s [%s]", eff.ProfileName, eff.ProfileSource))
		} else {
			parts = append(parts, fmt.Sprintf("profile=%s", eff.ProfileName))
		}
	} else if verbose {
		parts = append(parts, "profile=<unset>")
	}
	if eff.Account != "" {
		if verbose {
			parts = append(parts, fmt.Sprintf("account=%s [%s]", eff.Account, eff.AccountSource))
		} else {
			parts = append(parts, fmt.Sprintf("account=%s", eff.Account))
		}
	} else if verbose {
		parts = append(parts, "account=<unset>")
	}
	if eff.APIURL != "" {
		if verbose {
			parts = append(parts, fmt.Sprintf("api_url=%s [%s]", eff.APIURL, eff.APIURLSource))
		} else {
			parts = append(parts, fmt.Sprintf("api_url=%s", eff.APIURL))
		}
	}
	if eff.TokenSource != "" {
		if verbose {
			parts = append(parts, fmt.Sprintf("token=%s [%s]", eff.TokenSource, eff.TokenSourceRaw))
		} else {
			parts = append(parts, fmt.Sprintf("token=%s", eff.TokenSource))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "No effective configuration")
	}
	return DoctorCheck{Name: "Effective Config", Status: "pass", Message: strings.Join(parts, ", ")}
}

func checkDoctorCredentials(eff doctorEffectiveConfig) DoctorCheck {
	if strings.TrimSpace(eff.Token) == "" || eff.TokenSourceRaw == "none" {
		return DoctorCheck{
			Name:    "Credentials",
			Status:  "fail",
			Message: "No credentials found",
			Hint:    doctorLoginHint(eff.ProfileName),
		}
	}
	return DoctorCheck{Name: "Credentials", Status: "pass", Message: eff.TokenSource}
}

func checkDoctorCredentialStorage(eff doctorEffectiveConfig) DoctorCheck {
	switch eff.TokenSourceRaw {
	case "local-config":
		return DoctorCheck{
			Name:    "Credential Storage",
			Status:  "warn",
			Message: "Token is stored in local project config",
			Hint:    "Move credentials to the keyring with: mailpiece auth login <token>",
		}
	case "global-config":
		return DoctorCheck{
			Name:    "Credential Storage",
			Status:  "warn",
			Message: "Token is stored in global config",
			Hint:    "Prefer the system keyring: mailpiece auth login <token>",
		}
	case "fallback-file":
		hint := "System keyring unavailable; credentials are stored in a fallback file"
		if creds != nil {
			if warning := creds.FallbackWarning(); warning != "" {
				hint = warning
			}
		}
		return DoctorCheck{
			Name:    "Credential Storage",
			Status:  "warn",
			Message: "Using fallback credential file",
			Hint:    hint,
		}
	case "none":
		return DoctorCheck{Name: "Credential Storage", Status: "skip", Message: "Skipped (no credentials)"}
	default:
		return DoctorCheck{Name: "Credential Storage", Status: "pass", Message: "No storage issues detected"}
	}
}

func checkDoctorAPIURL(eff doctorEffectiveConfig) DoctorCheck {
	if eff.APIURL == "" {
		return DoctorCheck{Name: "API URL", Status: "fail", Message: "No Mailpiece address configured", Hint: noAPIURLHint}
	}
	if err := validateAPIURL(eff.APIURL); err != nil {
		return DoctorCheck{
			Name:    "API URL",
			Status:  "fail",
			Message: fmt.Sprintf("Invalid API URL: %s", eff.APIURL),
			Hint:    fmt.Sprintf("%v. Fix --api-url, MAILPIECE_API_URL, profile base_url, or run: mailpiece setup", err),
		}
	}
	if strings.HasPrefix(eff.APIURL, "http://") {
		return DoctorCheck{
			Name:    "API URL",
			Status:  "warn",
			Message: fmt.Sprintf("Using insecure HTTP URL: %s", eff.APIURL),
			Hint:    "Use HTTPS outside localhost development",
		}
	}
	return DoctorCheck{Name: "API URL", Status: "pass", Message: eff.APIURL}
}

func checkDoctorAPIReachability(ctx context.Context, eff doctorEffectiveConfig, verbose bool) DoctorCheck {
	if eff.APIURL == "" {
		return DoctorCheck{Name: "API Reachability", Status: "fail", Message: "No Mailpiece address configured", Hint: noAPIURLHint}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eff.APIURL, nil)
	if err != nil {
		return DoctorCheck{Name: "API Reachability", Status: "fail", Message: "Cannot build API request", Hint: err.Error()}
	}
	start := time.Now()
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return DoctorCheck{Name: "API Reachability", Status: "fail", Message: "Cannot reach API host", Hint: err.Error()}
	}
	defer resp.Body.Close()
	latency := time.Since(start)
	if resp.StatusCode >= 500 {
		return DoctorCheck{
			Name:    "API Reachability",
			Status:  "warn",
			Message: fmt.Sprintf("Host reachable but returned %d", resp.StatusCode),
			Hint:    "Check service status and try again",
		}
	}
	msg := "API host reachable"
	if verbose {
		msg = fmt.Sprintf("API host reachable (%d, %dms)", resp.StatusCode, latency.Milliseconds())
	}
	return DoctorCheck{Name: "API Reachability", Status: "pass", Message: msg}
}

func checkDoctorAuthentication(ctx context.Context, eff doctorEffectiveConfig, verbose bool) (DoctorCheck, *mailpiece.Identity) {
	client, err := newDoctorClient(eff)
	if err != nil {
		return DoctorCheck{Name: "Authentication", Status: "fail", Message: "SDK initialization failed", Hint: err.Error()}, nil
	}
	start := time.Now()
	identity, _, err := client.Identity().GetMyIdentity(ctx)
	if err != nil {
		return doctorFailure("Authentication", "", err, doctorLoginHint(eff.ProfileName)), nil
	}
	msg := "Token accepted"
	if verbose {
		msg = fmt.Sprintf("Token accepted (%dms, %d %s)", time.Since(start).Milliseconds(), len(identity.Accounts), pluralize(len(identity.Accounts), "account", "accounts"))
	}
	return DoctorCheck{Name: "Authentication", Status: "pass", Message: msg}, identity
}

// checkDoctorAccountAccess falls back, as requireAccount does, to the token's
// only account when none is configured.
func checkDoctorAccountAccess(ctx context.Context, eff doctorEffectiveConfig, identity *mailpiece.Identity, verbose bool) DoctorCheck {
	account, source := eff.Account, eff.AccountSource
	if account == "" && identity != nil {
		switch len(identity.Accounts) {
		case 0:
			return DoctorCheck{Name: "Account Access", Status: "fail", Message: "This token reaches no account"}
		case 1:
			account = mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
			source = "token's only account"
		default:
			slugs := make([]string, 0, len(identity.Accounts))
			for _, a := range identity.Accounts {
				slugs = append(slugs, fmt.Sprintf("%s (%s)", mailpiece.NormalizeAccountSlug(a.Slug), a.Name))
			}
			return DoctorCheck{
				Name:    "Account Access",
				Status:  "warn",
				Message: "This token reaches several accounts",
				Hint:    "Pick one with --account or MAILPIECE_ACCOUNT: " + strings.Join(slugs, ", "),
			}
		}
	}
	if account == "" {
		return DoctorCheck{
			Name:    "Account Access",
			Status:  "warn",
			Message: "No account configured",
			Hint:    "Set --account, MAILPIECE_ACCOUNT, or run: mailpiece setup",
		}
	}
	client, err := newDoctorClient(eff)
	if err != nil {
		return DoctorCheck{Name: "Account Access", Status: "fail", Message: "SDK initialization failed", Hint: err.Error()}
	}
	start := time.Now()
	if err := doctorAccountRead(ctx, client, account, identity); err != nil {
		return doctorFailure("Account Access", fmt.Sprintf("Cannot access account %s", account), err, "")
	}
	msg := fmt.Sprintf("Account %s accessible (%s)", account, source)
	if verbose {
		msg = fmt.Sprintf("Account %s accessible (%s, %dms)", account, source, time.Since(start).Milliseconds())
	}
	return DoctorCheck{Name: "Account Access", Status: "pass", Message: msg}
}

// doctorAccountRead proves the account answers this token with the one read
// every role may make: your own user. An account may be configured by a
// handle typed by hand while the identity names only the number, so the account
// whose slug matches is tried first and the others after it.
func doctorAccountRead(ctx context.Context, client *mailpiece.Client, account string, identity *mailpiece.Identity) error {
	if identity == nil || len(identity.Accounts) == 0 {
		return mailpiece.ErrForbidden("This token reaches no account")
	}
	var matching, others []mailpiece.IdentityAccount
	for _, a := range identity.Accounts {
		if mailpiece.NormalizeAccountSlug(a.Slug) == account {
			matching = append(matching, a)
		} else {
			others = append(others, a)
		}
	}
	var err error
	for _, a := range append(matching, others...) {
		var path string
		if path, err = mailpiece.Path("/users/%s.json", a.User.ID); err != nil {
			return err
		}
		if _, err = client.ForAccount(account).Get(ctx, path); err == nil {
			return nil
		}
	}
	return err
}

func newDoctorClient(eff doctorEffectiveConfig) (client *mailpiece.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cannot initialize SDK: %v", r)
			client = nil
		}
	}()
	return mailpiece.NewClient(&mailpiece.Config{BaseURL: eff.APIURL}, &mailpiece.StaticTokenProvider{Token: eff.Token}, mailpiece.WithUserAgent("mailpiece-cli/"+currentVersion())), nil
}

func doctorFailure(name, message string, err error, hint string) DoctorCheck {
	conv := convertSDKError(err)
	var outErr *output.Error
	if stderrors.As(conv, &outErr) {
		if message == "" {
			return DoctorCheck{Name: name, Status: "fail", Message: outErr.Message, Hint: firstNonEmpty(outErr.Hint, hint)}
		}
		return DoctorCheck{Name: name, Status: "fail", Message: message, Hint: outErr.Message}
	}
	var sdkErr *mailpiece.Error
	if stderrors.As(err, &sdkErr) {
		return DoctorCheck{Name: name, Status: "fail", Message: firstNonEmpty(message, sdkErr.Message), Hint: firstNonEmpty(sdkErr.Hint, hint)}
	}
	return DoctorCheck{Name: name, Status: "fail", Message: firstNonEmpty(message, err.Error()), Hint: hint}
}

func checkDoctorFilesystem(verbose bool) DoctorCheck {
	var warnings []string
	var badPaths []string

	globalPath := globalConfigPathForDoctor()
	if globalPath != "" {
		badPaths = appendIfBadParent(badPaths, filepath.Dir(globalPath))
		warnings = appendPermissionWarning(warnings, globalPath)
	}
	if localPath := config.LocalConfigPath(); localPath != "" {
		badPaths = appendIfBadParent(badPaths, filepath.Dir(localPath))
		warnings = appendPermissionWarning(warnings, localPath)
	}
	if home, err := os.UserHomeDir(); err == nil {
		badPaths = appendIfBadParent(badPaths, filepath.Join(home, ".agents", "skills"))
	}

	if len(badPaths) > 0 {
		return DoctorCheck{
			Name:    "Filesystem",
			Status:  "fail",
			Message: "Path collision detected",
			Hint:    strings.Join(badPaths, "; "),
		}
	}
	if len(warnings) > 0 {
		return DoctorCheck{
			Name:    "Filesystem",
			Status:  "warn",
			Message: "Sensitive files have broad permissions",
			Hint:    strings.Join(warnings, "; "),
		}
	}
	msg := "Expected paths look sane"
	if verbose && globalPath != "" {
		msg = fmt.Sprintf("Expected paths look sane (%s)", filepath.Dir(globalPath))
	}
	return DoctorCheck{Name: "Filesystem", Status: "pass", Message: msg}
}

func checkDoctorShellCompletion(verbose bool) DoctorCheck {
	shell := detectDoctorShell()
	if shell == "" {
		return DoctorCheck{Name: "Shell Completion", Status: "skip", Message: "Could not detect shell"}
	}
	installed, path := doctorCompletionPath(shell)
	if installed {
		msg := fmt.Sprintf("%s (installed)", shell)
		if verbose && path != "" {
			msg = fmt.Sprintf("%s (%s)", shell, path)
		}
		return DoctorCheck{Name: "Shell Completion", Status: "pass", Message: msg}
	}
	return DoctorCheck{
		Name:    "Shell Completion",
		Status:  "warn",
		Message: fmt.Sprintf("%s completion not installed", shell),
		Hint:    fmt.Sprintf("Run: mailpiece completion %s --help", shell),
	}
}

func checkDoctorSkillInstallation() DoctorCheck {
	if baselineSkillInstalled() {
		return DoctorCheck{Name: "Agent Skill", Status: "pass", Message: "Installed"}
	}
	return DoctorCheck{
		Name:    "Agent Skill",
		Status:  "warn",
		Message: "Baseline skill not installed",
		Hint:    "Run: mailpiece skill install",
	}
}

func checkDoctorSkillVersion() DoctorCheck {
	installed := installedSkillVersion()
	if installed == "" {
		return DoctorCheck{Name: "Skill Version", Status: "pass", Message: "Installed (version not tracked)"}
	}
	if currentVersion() == "dev" {
		return DoctorCheck{Name: "Skill Version", Status: "pass", Message: fmt.Sprintf("Installed (%s, dev build)", installed)}
	}
	if installed == currentVersion() {
		return DoctorCheck{Name: "Skill Version", Status: "pass", Message: fmt.Sprintf("Up to date (%s)", installed)}
	}
	return DoctorCheck{
		Name:    "Skill Version",
		Status:  "warn",
		Message: fmt.Sprintf("Stale (installed: %s, current: %s)", installed, currentVersion()),
		Hint:    "Run: mailpiece skill install",
	}
}

func buildDoctorBreadcrumbs(checks []DoctorCheck) []Breadcrumb {
	var breadcrumbs []Breadcrumb
	for _, c := range checks {
		if c.Status != "fail" && c.Status != "warn" {
			continue
		}
		switch c.Name {
		case "Global Config", "Local Config", "API URL", "API Reachability", "Filesystem", "Effective Config":
			breadcrumbs = append(breadcrumbs, breadcrumb("setup", "mailpiece setup", "Review and repair configuration"))
		case "Profile Store":
			breadcrumbs = append(breadcrumbs,
				breadcrumb("profiles", "mailpiece auth list", "List saved profiles"),
				breadcrumb("doctor_all", "mailpiece doctor --all-profiles", "Check every saved profile"),
			)
		case "Credentials", "Credential Storage", "Authentication":
			breadcrumbs = append(breadcrumbs,
				breadcrumb("login", "mailpiece auth login <token>", "Refresh credentials"),
				breadcrumb("status", "mailpiece auth status", "Check authentication status"),
			)
		case "Account Access":
			breadcrumbs = append(breadcrumbs,
				breadcrumb("identity", "mailpiece identity show", "Verify identity and accounts"),
				breadcrumb("identity", "mailpiece identity", "Show the accounts the token reaches"),
			)
		case "Shell Completion":
			if shell := detectDoctorShell(); shell != "" {
				breadcrumbs = append(breadcrumbs, breadcrumb("completion", fmt.Sprintf("mailpiece completion %s --help", shell), "Install shell completion"))
			}
		case "Agent Skill", "Skill Version":
			breadcrumbs = append(breadcrumbs, breadcrumb("skill", "mailpiece skill install", "Install or refresh the agent skill"))
		case "Claude Code Skill":
			breadcrumbs = append(breadcrumbs, breadcrumb("claude", "mailpiece setup claude", "Repair Claude Code integration"))
		}
	}
	seen := map[string]bool{}
	unique := make([]Breadcrumb, 0, len(breadcrumbs))
	for _, crumb := range breadcrumbs {
		if seen[crumb.Cmd] {
			continue
		}
		seen[crumb.Cmd] = true
		unique = append(unique, crumb)
	}
	return unique
}

func renderDoctorStyled(w io.Writer, result *DoctorResult, breadcrumbs []Breadcrumb) {
	if w == nil {
		return
	}
	title := lipgloss.NewStyle().Bold(true)
	passStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	statusIcon := map[string]string{"pass": "✓", "fail": "✗", "warn": "!", "skip": "○"}
	statusStyle := map[string]lipgloss.Style{"pass": passStyle, "fail": failStyle, "warn": warnStyle, "skip": skipStyle}
	renderChecks := func(indent string, checks []DoctorCheck) {
		for _, check := range checks {
			style := statusStyle[check.Status]
			fmt.Fprintf(w, "%s%s %s %s\n", indent, style.Render(statusIcon[check.Status]), title.Render(check.Name), style.Render(check.Message))
			if check.Hint != "" && (check.Status == "warn" || check.Status == "fail") {
				fmt.Fprintf(w, "%s    %s\n", indent, hintStyle.Render("↳ "+check.Hint))
			}
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, title.Render("Mailpiece CLI Doctor"))
	fmt.Fprintln(w)
	renderChecks("  ", result.Checks)
	if len(result.Profiles) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, title.Render("Profiles"))
		for _, profileResult := range result.Profiles {
			name := profileResult.Name
			if profileResult.Default {
				name += " (default)"
			}
			fmt.Fprintf(w, "  %s\n", title.Render(name))
			renderChecks("    ", profileResult.Checks)
			fmt.Fprintln(w)
		}
	}
	var summaryParts []string
	if result.Passed > 0 {
		summaryParts = append(summaryParts, passStyle.Render(fmt.Sprintf("%d passed", result.Passed)))
	}
	if result.Failed > 0 {
		summaryParts = append(summaryParts, failStyle.Render(fmt.Sprintf("%d failed", result.Failed)))
	}
	if result.Warned > 0 {
		summaryParts = append(summaryParts, warnStyle.Render(fmt.Sprintf("%d %s", result.Warned, pluralize(result.Warned, "warning", "warnings"))))
	}
	if result.Skipped > 0 {
		summaryParts = append(summaryParts, skipStyle.Render(fmt.Sprintf("%d skipped", result.Skipped)))
	}
	fmt.Fprintf(w, "  %s\n", strings.Join(summaryParts, "  "))
	if len(breadcrumbs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, title.Render("Next steps"))
		for _, crumb := range breadcrumbs {
			fmt.Fprintf(w, "  %s", crumb.Cmd)
			if crumb.Description != "" {
				fmt.Fprintf(w, "  # %s", crumb.Description)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w)
}

func renderDoctorMarkdown(w io.Writer, result *DoctorResult, breadcrumbs []Breadcrumb) {
	if w == nil {
		return
	}
	icons := map[string]string{"pass": "✅", "fail": "❌", "warn": "⚠️", "skip": "➖"}
	renderChecks := func(checks []DoctorCheck) {
		for _, check := range checks {
			fmt.Fprintf(w, "- %s **%s:** %s\n", icons[check.Status], check.Name, check.Message)
			if check.Hint != "" && (check.Status == "warn" || check.Status == "fail") {
				fmt.Fprintf(w, "  - Hint: %s\n", check.Hint)
			}
		}
	}

	fmt.Fprintln(w, "# Mailpiece CLI Doctor")
	fmt.Fprintln(w)
	renderChecks(result.Checks)
	if len(result.Profiles) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Profiles")
		for _, profileResult := range result.Profiles {
			name := profileResult.Name
			if profileResult.Default {
				name += " (default)"
			}
			fmt.Fprintf(w, "### %s\n", name)
			renderChecks(profileResult.Checks)
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintf(w, "**Summary:** %s\n", result.Summary())
	if len(breadcrumbs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "## Next steps")
		for _, crumb := range breadcrumbs {
			fmt.Fprintf(w, "- `%s`", crumb.Cmd)
			if crumb.Description != "" {
				fmt.Fprintf(w, ": %s", crumb.Description)
			}
			fmt.Fprintln(w)
		}
	}
}

func detectDoctorShell() string {
	shell := strings.TrimSpace(os.Getenv("SHELL"))
	if shell == "" {
		return ""
	}
	switch filepath.Base(shell) {
	case "bash", "zsh", "fish":
		return filepath.Base(shell)
	default:
		return ""
	}
}

func doctorCompletionPath(shell string) (bool, string) {
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home != "" {
		home = filepath.Clean(home)
	}
	switch shell {
	case "bash":
		paths := []string{"/opt/homebrew/etc/bash_completion.d/mailpiece", "/usr/local/etc/bash_completion.d/mailpiece", "/etc/bash_completion.d/mailpiece"}
		if home != "" {
			paths = append(paths, filepath.Join(home, ".local", "share", "bash-completion", "completions", "mailpiece"))
		}
		for _, path := range paths {
			if doctorPathExists(path) {
				return true, path
			}
		}
	case "zsh":
		paths := []string{"/opt/homebrew/share/zsh/site-functions/_mailpiece", "/usr/local/share/zsh/site-functions/_mailpiece"}
		if home != "" {
			paths = append(paths, filepath.Join(home, ".zsh", "completions", "_mailpiece"))
		}
		for _, path := range paths {
			if doctorPathExists(path) {
				return true, path
			}
		}
		if doctorZshrcHasCompletionEval() {
			return true, "~/.zshrc (via eval)"
		}
	case "fish":
		if home != "" {
			path := filepath.Join(home, ".config", "fish", "completions", "mailpiece.fish")
			if doctorPathExists(path) {
				return true, path
			}
		}
	}
	return false, ""
}

func doctorZshrcHasCompletionEval() bool {
	home := strings.TrimSpace(os.Getenv("HOME"))
	if home == "" {
		return false
	}
	data, err := os.ReadFile(filepath.Join(filepath.Clean(home), ".zshrc")) //nolint:gosec // trusted home-relative path for best-effort shell detection
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "mailpiece completion zsh")
}

func globalConfigPathForDoctor() string {
	paths := config.GlobalConfigPaths()
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if len(paths) > 0 {
		return paths[0]
	}
	return ""
}

func loadDoctorConfigFile(path string) (*config.Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfgFile config.Config
	if err := yaml.Unmarshal(data, &cfgFile); err != nil {
		return nil, err
	}
	return &cfgFile, nil
}

func doctorLoginHint(profileName string) string {
	if strings.TrimSpace(profileName) == "" {
		return "Run: mailpiece auth login <token>"
	}
	return fmt.Sprintf("Run: mailpiece auth login <token> --profile %s", profileName)
}

func doctorPathExists(path string) bool {
	clean := filepath.Clean(path)
	_, err := os.Stat(clean) //nolint:gosec // trusted path built from constants and cleaned home-relative locations
	return err == nil
}

func isOptionalAgentCheck(name string) bool {
	switch name {
	case "Claude Code Skill":
		return true
	default:
		return false
	}
}

func appendIfBadParent(paths []string, dir string) []string {
	if strings.TrimSpace(dir) == "" {
		return paths
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		return append(paths, fmt.Sprintf("%s exists but is not a directory", dir))
	}
	return paths
}

func appendPermissionWarning(warnings []string, path string) []string {
	if runtime.GOOS == "windows" || strings.TrimSpace(path) == "" {
		return warnings
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return warnings
	}
	if info.Mode().Perm()&0o077 != 0 {
		return append(warnings, fmt.Sprintf("tighten permissions on %s (recommended: chmod 600)", path))
	}
	return warnings
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}
