// Package commands implements CLI commands for the Mailpiece CLI.
package commands

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/basecamp/cli/credstore"
	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/itchyny/gojq"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// Breadcrumb is a type alias for output.Breadcrumb.
type Breadcrumb = output.Breadcrumb

var (
	// Global flags
	cfgToken    string
	cfgProfile  string
	cfgAPIURL   string
	cfgVerbose  bool
	cfgJSON     bool
	cfgQuiet    bool
	cfgIDsOnly  bool
	cfgCount    bool
	cfgAgent    bool
	cfgStyled   bool
	cfgMarkdown bool
	cfgLimit    int
	cfgJQ       string

	// Loaded config
	cfg *config.Config

	// SDK client
	sdk *mailpiece.Client

	// Credential store
	creds *credstore.Store

	// Profile store and the selected profile name. The selected profile is
	// kept separate from cfg.Account so aliases can target the same account.
	profiles                     *profile.Store
	activeProfile                string
	activeProfileExplicitAccount bool

	// Output writer
	out       *output.Writer
	outWriter io.Writer // raw writer for styled/markdown rendering
)

// rootCmd represents the base command.
var rootCmd = &cobra.Command{
	Use:     "mailpiece",
	Short:   "Mailpiece CLI - Command-line interface for the Mailpiece API",
	Long:    `Command-line interface for Mailpiece, the newsletter app.`,
	Version: "dev",
	RunE:    runRootDefault,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		errOutputWrite = nil
		// Early jq validation: check flag conflicts first (actionable message),
		// then parse + compile before RunE so invalid expressions are rejected
		// with no side effects. The compiled code is reused below to avoid
		// parsing the expression twice.
		var jqCode *gojq.Code
		if cfgJQ != "" {
			if cfgIDsOnly {
				return errors.ErrJQConflict("--ids-only")
			}
			if cfgCount {
				return errors.ErrJQConflict("--count")
			}
			var err error
			jqCode, err = compileJQ(cfgJQ)
			if err != nil {
				return err
			}
		}

		// Resolve output format from parsed flags (must happen post-parse).
		format, err := resolveFormat()
		if err != nil {
			return &output.Error{Code: output.CodeUsage, Message: err.Error()}
		}
		if lastResult != nil {
			// Test mode: preserve test buffer as writer.
			outWriter = &testBuf
			var w io.Writer = &testBuf
			if jqCode != nil {
				w = newJQWriterWithCode(&testBuf, jqCode)
			}
			out = output.New(output.Options{Format: format, Writer: w})
		} else {
			outWriter = os.Stdout
			var w io.Writer = os.Stdout
			if jqCode != nil {
				w = newJQWriterWithCode(os.Stdout, jqCode)
			}
			out = output.New(output.Options{Format: format, Writer: w})
		}

		// In test mode, cfg is already set by SetTestConfig - don't overwrite
		if cfg == nil {
			cfg = config.Load()
		}
		// Initialize credential store (skip in test mode)
		if creds == nil && lastResult == nil {
			fallbackDir := ""
			if cfgPath, err := config.ConfigPath(); err == nil {
				fallbackDir = filepath.Join(filepath.Dir(cfgPath), "credentials")
			} else if home, err := os.UserHomeDir(); err == nil {
				fallbackDir = filepath.Join(home, ".config", "mailpiece", "credentials")
			}
			creds = credstore.NewStore(credstore.StoreOptions{
				ServiceName:   "mailpiece",
				DisableEnvVar: "MAILPIECE_NO_KEYRING",
				FallbackDir:   fallbackDir,
			})
		}

		// Initialize profile store (skip in test mode)
		if profiles == nil && lastResult == nil {
			if cfgPath, err := config.ConfigPath(); err == nil {
				profiles = profile.NewStore(filepath.Join(filepath.Dir(cfgPath), "config.json"))
			}
		}

		doctorAllProfiles := false
		if cmd.Name() == "doctor" {
			doctorAllProfiles, _ = cmd.Flags().GetBool("all-profiles")
		}
		if err := resolveProfile(); err != nil {
			// Login can create a new profile. An explicit account creates an alias;
			// otherwise the profile name remains the routed account.
			newProfile := firstNonEmpty(cfgProfile, os.Getenv("MAILPIECE_PROFILE"), os.Getenv("MAILPIECE_ACCOUNT"))
			if cmd == authLoginCmd && newProfile != "" {
				activeProfile = newProfile
				cfg.Account = firstNonEmpty(authLoginAccount, newProfile)
			} else if cmd == authLogoutCmd {
				if newProfile != "" {
					activeProfile = newProfile
				} else if activeProfile == "" {
					return &output.Error{Code: output.CodeUsage, Message: err.Error()}
				}
			} else if cmd != authSwitchCmd && !doctorAllProfiles {
				return &output.Error{Code: output.CodeUsage, Message: err.Error()}
			}
		}
		cfg.Account = mailpiece.NormalizeAccountSlug(cfg.Account)
		if cfg.Account != "" && cmd != authLogoutCmd && cmd != authSwitchCmd && !doctorAllProfiles {
			if err := validateAccountIdentifier(cfg.Account); err != nil {
				return &output.Error{Code: output.CodeUsage, Message: err.Error()}
			}
		}

		// Commands that replace or remove credentials do not resolve an existing
		// token first. Profile sweeps resolve each target separately.
		if cmd != authLoginCmd && cmd != authLogoutCmd && cmd != authSwitchCmd && !doctorAllProfiles {
			resolveToken()
		}

		// --api-url flag overrides everything (including profile BaseURL)
		if cfgAPIURL != "" {
			cfg.APIURL = cfgAPIURL
		}

		// MAILPIECE_DEBUG enables verbose output
		if os.Getenv("MAILPIECE_DEBUG") != "" {
			cfgVerbose = true
		}

		// Initialize SDK client (skip if already set by test mode)
		if sdk == nil {
			errSDKInit = initSDK(cmd, cfg.APIURL, cfg.Token)
		}

		startUpdateCheck()
		return nil
	},
	PersistentPostRunE: func(cmd *cobra.Command, args []string) error {
		if errOutputWrite != nil {
			err := errOutputWrite
			errOutputWrite = nil
			return err
		}
		if RefreshSkillsIfVersionChanged() && !IsMachineOutput() {
			fmt.Fprintf(os.Stderr, "Agent skill updated to match CLI %s\n", currentVersion())
		}
		finishUpdateCheck()
		return nil
	},
	SilenceUsage:               true,
	SilenceErrors:              true,
	SuggestionsMinimumDistance: 2,
}

var cliVersion = "dev"

// SetVersion sets the CLI version used for `--version` and `version`.
func SetVersion(v string) {
	if v == "" {
		return
	}
	cliVersion = v
	rootCmd.Version = v
}

func currentVersion() string {
	return cliVersion
}

// Execute runs the root command.
func Execute() {
	configureCLIUX()

	// Default to Auto; PersistentPreRunE will re-resolve from parsed flags.
	outWriter = os.Stdout
	out = output.New(output.Options{Format: output.FormatAuto, Writer: os.Stdout})
	cmd, err := executeRoot()
	if err != nil {
		if format, formatErr := resolveFormat(); formatErr == nil {
			out = output.New(output.Options{Format: format, Writer: os.Stdout})
		}

		var e *output.Error
		if !stderrors.As(err, &e) {
			// Cobra-level errors (arg count, unknown flag) → usage
			e = &output.Error{Code: output.CodeUsage, Message: err.Error()}
		}

		// jq-related errors (validation failures, unsupported commands, conflicts)
		// must never be fed through the jq filter. Rebuild the output writer
		// without jq so the error renders cleanly. Re-resolve the format to
		// honor explicit flags like --agent --json.
		if errors.IsJQError(err) && cfgJQ != "" {
			format, fmtErr := resolveFormat()
			if fmtErr != nil {
				// resolveFormat() can fail when --jq conflicts with another flag
				// (e.g. --jq --styled). Fall back to a sensible machine format.
				format = output.FormatJSON
				if cfgAgent || cfgQuiet {
					format = output.FormatQuiet
				}
			}
			out = output.New(output.Options{Format: format, Writer: outWriter})
		}

		if isHumanOutput() {
			printHumanError(cmd, e)
		} else {
			_ = out.Err(e)
		}
		os.Exit(e.ExitCode())
	}
}

// executeRoot runs the root command. A group handed a positional that names
// none of its subcommands is a usage error, not help with a zero exit, so a
// typo never reads as success.
func executeRoot() (*cobra.Command, error) {
	cmd, err := rootCmd.ExecuteC()
	if err == nil && cmd != rootCmd && isBareGroupWithUnknownArg(cmd) {
		err = unknownSubcommandError(cmd, cmd.Flags().Args()[0])
	}
	return cmd, err
}

// isBareGroupWithUnknownArg reports whether cmd is a group with no action of
// its own, handed a positional and not asked for help.
func isBareGroupWithUnknownArg(cmd *cobra.Command) bool {
	if cmd.Runnable() || !cmd.HasSubCommands() {
		return false
	}
	if help, _ := cmd.Flags().GetBool("help"); help {
		return false
	}
	return len(cmd.Flags().Args()) > 0
}

// unknownSubcommandError offers cobra's spelling suggestions, falling back to help.
func unknownSubcommandError(cmd *cobra.Command, arg string) error {
	msg := fmt.Sprintf("unknown command %q for %q", arg, cmd.CommandPath())
	// Only the root sets SuggestionsMinimumDistance; borrow its threshold.
	if cmd.SuggestionsMinimumDistance <= 0 {
		cmd.SuggestionsMinimumDistance = cmd.Root().SuggestionsMinimumDistance
	}
	if suggestions := cmd.SuggestionsFor(arg); len(suggestions) > 0 {
		return output.ErrUsageHint(msg, "Did you mean: "+strings.Join(suggestions, ", ")+"?")
	}
	return output.ErrUsageHint(msg, "Run: "+cmd.CommandPath()+" --help")
}

// resolveFormat returns the output format from flags.
// Default is FormatAuto (TTY→Styled, pipe→JSON). At most one format flag may be set.
func resolveFormat() (output.Format, error) {
	// Count mutually exclusive format flags
	n := 0
	if cfgJSON {
		n++
	}
	if cfgQuiet {
		n++
	}
	if cfgIDsOnly {
		n++
	}
	if cfgCount {
		n++
	}
	if cfgStyled {
		n++
	}
	if cfgMarkdown {
		n++
	}
	if n > 1 {
		return 0, fmt.Errorf("only one output format flag may be used at a time (--json, --quiet, --ids-only, --count, --styled, --markdown)")
	}

	// --agent is orthogonal to format flags but --agent --styled is an error
	if cfgAgent && cfgStyled {
		return 0, fmt.Errorf("--agent and --styled cannot be used together")
	}

	// --jq is a JSON transform and is incompatible with human/count/id renderers.
	if cfgJQ != "" && (cfgStyled || cfgMarkdown || cfgIDsOnly || cfgCount) {
		return 0, fmt.Errorf("--jq filters JSON output; use it with default JSON output or --quiet, not with --styled, --markdown, --ids-only, or --count")
	}

	// Explicit format flag wins
	switch {
	case cfgQuiet:
		return output.FormatQuiet, nil
	case cfgIDsOnly:
		return output.FormatIDs, nil
	case cfgCount:
		return output.FormatCount, nil
	case cfgJSON:
		return output.FormatJSON, nil
	case cfgStyled:
		return output.FormatStyled, nil
	case cfgMarkdown:
		return output.FormatMarkdown, nil
	}

	// --jq implies JSON (or quiet for --agent)
	if cfgJQ != "" {
		if cfgAgent {
			return output.FormatQuiet, nil
		}
		return output.FormatJSON, nil
	}

	// --agent alone defaults to FormatQuiet
	if cfgAgent {
		return output.FormatQuiet, nil
	}

	return output.FormatAuto, nil
}

// IsMachineOutput returns true when output should be treated as machine-consumable.
// True when any machine format flag is set, --agent is set, or stdout/stdin is not a TTY.
func IsMachineOutput() bool {
	if cfgAgent || cfgJSON || cfgQuiet || cfgIDsOnly || cfgCount || cfgJQ != "" {
		return true
	}
	if !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return true
	}
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return true
	}
	return false
}

func isHumanOutput() bool {
	if cfgStyled || cfgMarkdown || requestedHumanOutput() {
		return true
	}
	if out != nil {
		switch out.EffectiveFormat() {
		case output.FormatStyled, output.FormatMarkdown:
			return true
		default:
			return false
		}
	}
	return !IsMachineOutput()
}

func requestedHumanOutput() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--styled" || arg == "--markdown" {
			return true
		}
	}
	return false
}

func printHumanError(cmd *cobra.Command, err error) {
	e := output.AsError(err)
	msg := strings.TrimSpace(e.Message)
	if msg != "" {
		fmt.Fprintln(os.Stderr, msg)
	}
	if e.Hint != "" && !strings.Contains(msg, e.Hint) {
		fmt.Fprintf(os.Stderr, "\nHint: %s\n", e.Hint)
	}
	if e.Code == output.CodeUsage && !strings.Contains(msg, "--help") {
		fmt.Fprintf(os.Stderr, "\nRun `%s` for usage.\n", usageHelpCommand(cmd))
	}
}

func usageHelpCommand(cmd *cobra.Command) string {
	if cmd == nil {
		return rootCmd.CommandPath() + " --help"
	}
	path := strings.TrimSpace(cmd.CommandPath())
	if path == "" {
		path = rootCmd.CommandPath()
	}
	return path + " --help"
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgToken, "token", "", "API access token")
	rootCmd.PersistentFlags().StringVar(&cfgProfile, "profile", "", "Named profile to use")
	rootCmd.PersistentFlags().StringVar(&cfgAPIURL, "api-url", "", "API base URL")
	rootCmd.PersistentFlags().BoolVar(&cfgVerbose, "verbose", false, "Show request/response details")
	rootCmd.PersistentFlags().BoolVar(&cfgJSON, "json", false, "JSON envelope output")
	rootCmd.PersistentFlags().BoolVar(&cfgQuiet, "quiet", false, "Raw JSON data without envelope")
	rootCmd.PersistentFlags().BoolVar(&cfgIDsOnly, "ids-only", false, "Print one ID per line")
	rootCmd.PersistentFlags().BoolVar(&cfgCount, "count", false, "Print count of results")
	rootCmd.PersistentFlags().BoolVar(&cfgAgent, "agent", false, "Agent mode (default: quiet format, no interactive prompts)")
	rootCmd.PersistentFlags().BoolVar(&cfgStyled, "styled", false, "Styled terminal output with colors")
	rootCmd.PersistentFlags().BoolVar(&cfgMarkdown, "markdown", false, "Markdown formatted output")
	rootCmd.PersistentFlags().IntVar(&cfgLimit, "limit", 0, "Maximum number of results to display")
	rootCmd.PersistentFlags().StringVar(&cfgJQ, "jq", "", "Apply jq filter to JSON output (built-in, no external jq required; implies --json)")
}

// errSDKInit stores any error from SDK initialization so commands can return it.
var errSDKInit error

// getSDK returns an SDK AccountClient bound to the resolved account.
// Call requireAuthAndAccount() first.
func getSDK() *mailpiece.AccountClient {
	return sdk.ForAccount(cfg.Account)
}

// getSDKClient returns the root SDK Client (for account-independent operations).
func getSDKClient() *mailpiece.Client {
	return sdk
}

// requireSDK returns the SDK init error if the SDK failed to initialize.
// Commands that use getSDK()/getSDKClient() should call this first.
func requireSDK() error {
	return errSDKInit
}

// initSDK creates the SDK client, guarding against panics from HTTPS validation.
func initSDK(cmd *cobra.Command, apiURL, token string) (initErr error) {
	sdk = nil
	defer func() {
		if r := recover(); r != nil {
			initErr = &output.Error{
				Code:    output.CodeUsage,
				Message: fmt.Sprintf("Cannot initialize SDK: %v", r),
				Hint:    "Non-localhost API URLs must use HTTPS. Fix --api-url, MAILPIECE_API_URL, or run 'mailpiece setup'.",
			}
		}
	}()

	sdk = newSDKClient(cmd, apiURL, token)
	return nil
}

func newSDKClient(cmd *cobra.Command, apiURL, token string) *mailpiece.Client {
	opts := []mailpiece.ClientOption{mailpiece.WithUserAgent("mailpiece-cli/" + cmd.Root().Version)}
	if cfgVerbose {
		// The hooks log at debug level, which a default handler drops.
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
		opts = append(opts, mailpiece.WithHooks(mailpiece.NewSlogHooks(logger)))
	}
	return mailpiece.NewClient(&mailpiece.Config{BaseURL: apiURL}, &mailpiece.StaticTokenProvider{Token: token}, opts...)
}

// normalizeAny converts any value to map[string]any or []map[string]any
// via JSON round-trip, so typed SDK results render like raw JSON.
func normalizeAny(v any) any {
	if v == nil {
		return nil
	}
	switch d := v.(type) {
	case json.RawMessage:
		if len(d) == 0 {
			return nil
		}
		var parsed any
		if json.Unmarshal(d, &parsed) != nil {
			return nil
		}
		return normalizeAny(parsed)
	case map[string]any:
		return d
	case []map[string]any:
		return d
	case []any:
		maps := make([]map[string]any, 0, len(d))
		for _, item := range d {
			if m, ok := item.(map[string]any); ok {
				maps = append(maps, m)
			} else {
				return v // mixed types, return as-is
			}
		}
		return maps
	}
	b, err := json.Marshal(v)
	if err != nil {
		return v
	}
	var result any
	if json.Unmarshal(b, &result) != nil {
		return v
	}
	return normalizeAny(result)
}

// requireAuth checks that we have authentication configured.
func requireAuth() error {
	if cfg.Token == "" {
		return errors.NewAuthError("No API token configured. Run 'mailpiece auth login TOKEN' or set MAILPIECE_TOKEN")
	}
	return nil
}

// requireAccount checks that we have an account configured.
func requireAccount() error {
	if cfg.Account == "" {
		return errors.NewInvalidArgsError("No account configured. Set --profile flag, MAILPIECE_PROFILE, or run 'mailpiece setup'")
	}
	return nil
}

// requireAuthAndAccount checks auth, account, and SDK initialization.
func requireAuthAndAccount() error {
	if err := requireAuth(); err != nil {
		return err
	}
	if err := requireAccount(); err != nil {
		return err
	}
	return requireSDK()
}

// Helper function for required flag errors
func newRequiredFlagError(flag string) error {
	return errors.NewInvalidArgsError("required flag --" + flag + " not provided")
}

// checkFlagValue holds a flag to the values the app takes; an empty one passes.
func checkFlagValue(flag, value string, allowed []string) error {
	if value == "" || slices.Contains(allowed, value) {
		return nil
	}
	return errors.NewInvalidArgsError(fmt.Sprintf("--%s must be one of: %s", flag, strings.Join(allowed, ", ")))
}

// changedString and changedInt point at a flag's value when the flag was passed, so an
// update sends only what was asked, an emptied field included.
func changedString(cmd *cobra.Command, name string, value *string) *string {
	if cmd.Flags().Changed(name) {
		return value
	}
	return nil
}

func changedInt[T int | int64](cmd *cobra.Command, name string, value *T) *T {
	if cmd.Flags().Changed(name) {
		return value
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
	if profiles == nil {
		// No profile store: the selected name is both profile and account.
		if p := firstNonEmpty(cfgProfile, profileEnvVar()); p != "" {
			activeProfile = p
			cfg.Account = p
		}
		return nil
	}

	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return err
	}
	if len(allProfiles) == 0 {
		// No profiles saved: the selected name is both profile and account
		// until a profile with an explicit account is saved.
		if p := firstNonEmpty(cfgProfile, profileEnvVar()); p != "" {
			activeProfile = p
			cfg.Account = p
		}
		return nil
	}

	envProfile := profileEnvVar()
	resolved, err := profile.Resolve(profile.ResolveOptions{
		FlagValue:      cfgProfile,
		EnvVar:         envProfile,
		DefaultProfile: defaultName,
		Profiles:       allProfiles,
	})
	if err != nil {
		// A profile named explicitly must exist; an ambiguous default is skipped.
		if cfgProfile != "" || envProfile != "" {
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

	activeProfile = resolved
	binding, err := resolveProfileAccountBinding(resolved, p)
	if err != nil {
		return err
	}
	activeProfileExplicitAccount = binding.Explicit
	cfg.Account = binding.Account
	if p.BaseURL != "" && os.Getenv("MAILPIECE_API_URL") == "" {
		cfg.APIURL = p.BaseURL
	}
	return nil
}

// profileEnvVar returns MAILPIECE_PROFILE, falling back to the deprecated
// MAILPIECE_ACCOUNT.
func profileEnvVar() string {
	if v := os.Getenv("MAILPIECE_PROFILE"); v != "" {
		return v
	}
	if v := os.Getenv("MAILPIECE_ACCOUNT"); v != "" {
		fmt.Fprintln(os.Stderr, "Warning: MAILPIECE_ACCOUNT is deprecated, use MAILPIECE_PROFILE instead")
		return v
	}
	return ""
}

// currentProfileName returns the selected credential profile. Config without
// a named profile uses the configured account as its credential key.
func currentProfileName() string {
	if activeProfile != "" {
		return activeProfile
	}
	if cfg != nil {
		return cfg.Account
	}
	return ""
}

// resolveToken applies token precedence: YAML → credstore → env → flag.
func resolveToken() {
	if creds != nil {
		if profileName := currentProfileName(); profileName != "" {
			// Explicit account bindings are aliases and require their own
			// profile-scoped credential.
			if activeProfileExplicitAccount {
				cfg.Token = ""
			}
			if t, err := credsLoadProfileToken(profileName); err == nil && t != "" {
				cfg.Token = t
			}
		}
	}
	if t := os.Getenv("MAILPIECE_TOKEN"); t != "" {
		cfg.Token = t
	}
	if cfgToken != "" {
		cfg.Token = cfgToken
	}
}

// SetTestMode configures the commands package for testing.
// It captures results in a test buffer instead of writing to stdout.
func SetTestMode() *CommandResult {
	testBuf.Reset()
	outWriter = &testBuf
	out = output.New(output.Options{Format: output.FormatJSON, Writer: &testBuf})
	lastResult = &CommandResult{}
	return lastResult
}

// SetTestFormat reconfigures the output writer with the given format.
// Must be called after SetTestMode.
func SetTestFormat(format output.Format) {
	testBuf.Reset()
	outWriter = &testBuf
	out = output.New(output.Options{Format: format, Writer: &testBuf})
}

// TestOutput returns the raw output from the last command execution.
// Useful for verifying non-JSON format output.
func TestOutput() string {
	return lastRawOutput
}

// SetTestSDK configures the commands package for SDK-based testing.
// Pass an httptest.Server URL and the SDK will be created pointing at it.
func SetTestSDK(baseURL string) {
	sdk = mailpiece.NewClient(
		&mailpiece.Config{BaseURL: baseURL},
		&mailpiece.StaticTokenProvider{Token: "test-token"},
		mailpiece.WithBaseDelay(time.Millisecond),
		mailpiece.WithMaxJitter(time.Millisecond),
	)
}

// SetTestCreds sets the credential store for testing.
func SetTestCreds(store *credstore.Store) {
	creds = store
}

// SetTestProfiles sets the profile store for testing.
func SetTestProfiles(store *profile.Store) {
	profiles = store
}

// SetTestConfig sets the config for testing.
func SetTestConfig(token, account, apiURL string) {
	cfg = &config.Config{
		Token:   token,
		Account: account,
		APIURL:  apiURL,
	}
}

// ResetTestMode resets the test mode configuration.
func ResetTestMode() {
	sdk = nil
	errSDKInit = nil
	lastResult = nil
	lastRawOutput = ""
	errOutputWrite = nil
	cfg = nil
	creds = nil
	profiles = nil
	activeProfile = ""
	activeProfileExplicitAccount = false
	cfgToken = ""
	authLoginAccount = ""
	cfgAPIURL = ""
	cfgVerbose = false
	cfgJSON = false
	cfgQuiet = false
	cfgIDsOnly = false
	cfgCount = false
	cfgAgent = false
	cfgStyled = false
	cfgMarkdown = false
	cfgLimit = 0
	cfgJQ = ""
	cfgProfile = ""
}
