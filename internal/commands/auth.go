package commands

import (
	stderrors "errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var authLoginAccount string

var authCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  "Commands for managing API authentication with a personal access token.",
}

var authLoginCmd = &cobra.Command{
	Use:   "login [TOKEN]",
	Short: "Save API token",
	Long: `Saves an API token to the system keyring (or fallback file) under a profile.

Create a token in Mailpiece under Profile › Personal access tokens. With --with-token the token
is read from stdin, which keeps it out of shell history; it can also be given as an argument.
The address of the Mailpiece install is taken from --api-url, MAILPIECE_API_URL or the config.
The token is saved under the selected profile (--profile, MAILPIECE_PROFILE or the default), or
one named after the configured account when none is selected. --account sets the account the
profile routes to; a new profile otherwise routes to the account it is named after.`,
	Example: "$ op read \"op://<vault>/<item>/credential\" | mailpiece auth login --with-token --profile 834206594 --api-url https://mailpiece.example.com\n$ mailpiece auth login TOKEN --profile work --account 834206594 --api-url http://app.mailpiece.localhost:3020",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		withToken, _ := cmd.Flags().GetBool("with-token")
		switch {
		case withToken && len(args) > 0:
			return output.ErrUsageHint("--with-token reads the token from stdin, so it takes no TOKEN argument",
				"Pipe it in: ... | mailpiece auth login --with-token --profile <number>")
		case !withToken && len(args) == 0:
			return output.ErrUsageHint("No token given",
				"Pipe it in: ... | mailpiece auth login --with-token --profile <number>")
		}
		profileName := currentProfileName()
		account := mailpiece.NormalizeAccountSlug(firstNonEmpty(authLoginAccount, cfg.Account))

		if profileName == "" {
			return errors.NewInvalidArgsError("No profile configured. Set --profile flag, MAILPIECE_PROFILE, or run 'mailpiece setup'")
		}
		if account == "" {
			return errors.NewInvalidArgsError("No account configured. Set --account to the Mailpiece account slug")
		}
		if err := profile.ValidateName(profileName); err != nil {
			return errors.NewInvalidArgsError(err.Error())
		}
		if err := validateAccountIdentifier(account); err != nil {
			return errors.NewInvalidArgsError(err.Error())
		}

		var token string
		if withToken {
			var err error
			if token, err = readTokenFromStdin(cmd); err != nil {
				return err
			}
		} else {
			token = args[0]
		}

		activeProfile = profileName
		cfg.Account = account

		if creds != nil {
			_, err := saveProfileCredentialState(profileCredentialSaveOptions{
				ProfileName: profileName,
				Account:     account,
				BaseURL:     cfg.APIURL,
				Token:       token,
				UpdateGlobal: func(globalCfg *config.Config, credentialStored bool) {
					globalCfg.Account = account
					if credentialStored {
						globalCfg.Token = ""
					}
				},
			})
			if err != nil {
				return &output.Error{Code: output.CodeAPI, Message: err.Error()}
			}
		} else {
			// Fallback: save to config file (test mode or credstore unavailable)
			globalCfg := config.LoadGlobal()
			globalCfg.Token = token
			globalCfg.Account = account
			globalCfg.APIURL = cfg.APIURL
			if err := globalCfg.Save(); err != nil {
				return &output.Error{Code: output.CodeAPI, Message: err.Error()}
			}
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("status", "mailpiece auth status", "Check auth status"),
			breadcrumb("identity", "mailpiece identity", "View identity"),
			breadcrumb("doctor", "mailpiece doctor", "Check CLI health"),
		}

		printMutation(map[string]any{
			"authenticated": true,
			"profile":       profileName,
			"account":       account,
			"storage":       tokenStorage(),
			"message":       "Token saved",
		}, "", breadcrumbs)
		return nil
	},
}

// tokenStorage names where a login just saved its token.
func tokenStorage() string {
	switch {
	case creds == nil:
		return "config"
	case creds.UsingKeyring():
		return "keyring"
	default:
		return "file"
	}
}

// saveLoginWithFallback saves a sign-in's token under a profile named after the
// account, putting it in the global config when no credential store takes it.
func saveLoginWithFallback(account, apiURL, token string) error {
	warning, err := saveProfileCredentialState(profileCredentialSaveOptions{
		ProfileName:            account,
		Account:                account,
		BaseURL:                apiURL,
		Token:                  token,
		AllowYAMLTokenFallback: true,
		UpdateGlobal: func(globalCfg *config.Config, credentialStored bool) {
			if credentialStored {
				globalCfg.Token = ""
			} else {
				globalCfg.Token = token
			}
			globalCfg.Account = account
			globalCfg.APIURL = apiURL
		},
	})
	if warning != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save token to credential store: %v\n", warning)
	}
	return err
}

var authLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove saved credentials",
	Long:  "Removes saved credentials for the current profile (or all profiles with --all).",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		if all {
			return authLogoutAll()
		}

		profileName := currentProfileName()
		if profileName == "" {
			return errors.NewInvalidArgsError("No profile configured. Use --profile to specify which profile to log out, or --all to log out of all profiles")
		}

		// Read profile state before cleanup so failures cannot silently change
		// which identity remains active.
		wasDefault := false
		profileExists := false
		if profiles != nil {
			allProfiles, defaultName, err := profiles.List()
			if err != nil {
				return &output.Error{Code: output.CodeAPI, Message: err.Error()}
			}
			_, profileExists = allProfiles[profileName]
			wasDefault = defaultName == profileName
		}

		var cleanupErrors []error
		if creds != nil {
			if err := credsDeleteProfileToken(profileName); err != nil && !isCredentialNotFound(err) {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete credential: %w", err))
			}
		}
		if profiles != nil && profileExists {
			if err := profiles.Delete(profileName); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete profile: %w", err))
			}
		}

		globalCfg := config.LoadGlobal()
		if wasDefault || globalCfg.Account == profileName {
			globalCfg.Account = ""
			globalCfg.Token = ""
		}
		if err := globalCfg.Save(); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("save global config: %w", err))
		}
		if err := stderrors.Join(cleanupErrors...); err != nil {
			return &output.Error{Code: output.CodeAPI, Message: fmt.Sprintf("logout incomplete: %v", err)}
		}

		printMutation(map[string]any{
			"authenticated": false,
			"profile":       profileName,
			"message":       "Logged out successfully",
		}, "", []Breadcrumb{breadcrumb("login", "mailpiece auth login <token>", "Log in again")})
		return nil
	},
}

func authLogoutAll() error {
	var cleanupErrors []error
	if profiles != nil {
		allProfiles, _, err := profiles.List()
		if err != nil {
			return &output.Error{Code: output.CodeAPI, Message: err.Error()}
		}
		for name := range allProfiles {
			if creds != nil {
				if err := credsDeleteProfileToken(name); err != nil && !isCredentialNotFound(err) {
					cleanupErrors = append(cleanupErrors, fmt.Errorf("delete credential %q: %w", name, err))
				}
			}
			if err := profiles.Delete(name); err != nil {
				cleanupErrors = append(cleanupErrors, fmt.Errorf("delete profile %q: %w", name, err))
			}
		}
	}
	if err := config.Delete(); err != nil {
		cleanupErrors = append(cleanupErrors, fmt.Errorf("delete global config: %w", err))
	}
	if err := stderrors.Join(cleanupErrors...); err != nil {
		return &output.Error{Code: output.CodeAPI, Message: fmt.Sprintf("logout incomplete: %v", err)}
	}

	printMutation(map[string]any{
		"authenticated": false,
		"message":       "Logged out of all profiles",
	}, "", []Breadcrumb{breadcrumb("login", "mailpiece auth login <token>", "Log in again")})
	return nil
}

var authStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show authentication status",
	Long:  "Shows whether a token is configured, where it comes from, and which profile and account are in use. It does not call the API; run 'mailpiece doctor' for that.",
	RunE: func(cmd *cobra.Command, args []string) error {
		status := map[string]any{
			"authenticated": cfg.Token != "",
		}

		if cfg.Token != "" {
			status["token_source"] = tokenSource()
		}
		if activeProfile != "" {
			status["profile"] = activeProfile
		}
		if cfg.Account != "" {
			status["account"] = cfg.Account
		}
		if cfg.APIURL != "" {
			status["api_url"] = cfg.APIURL
		}
		if creds != nil {
			status["using_keyring"] = creds.UsingKeyring()
			if w := creds.FallbackWarning(); w != "" {
				status["credential_warning"] = w
			}
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("identity", "mailpiece identity", "View identity"),
			breadcrumb("logout", "mailpiece auth logout", "Log out"),
		}
		if cfg.Token == "" {
			breadcrumbs = []Breadcrumb{breadcrumb("login", "mailpiece auth login <token>", "Log in")}
		}
		if profiles != nil {
			allProfiles, _, _ := profiles.List()
			if len(allProfiles) > 1 {
				breadcrumbs = append(breadcrumbs, breadcrumb("list", "mailpiece auth list", "List profiles"))
			}
		}

		printDetail(status, "", breadcrumbs)
		return nil
	},
}

var authProfileColumns = render.Columns{
	{Header: "Profile", Field: "profile"},
	{Header: "Account", Field: "account"},
	{Header: "Active", Field: "active"},
	{Header: "Base URL", Field: "base_url"},
}

var authListCmd = &cobra.Command{
	Use:   "list",
	Short: "List authenticated profiles",
	Long:  "Shows all profiles with saved credentials.",
	RunE: func(cmd *cobra.Command, args []string) error {
		noProfiles := func() error {
			breadcrumbs := []Breadcrumb{
				breadcrumb("login", "mailpiece auth login <token>", "Log in"),
				breadcrumb("email", "mailpiece auth email", "Sign in by email"),
			}
			printList([]any{}, authProfileColumns, "No profiles configured", breadcrumbs)
			return nil
		}
		if profiles == nil {
			return noProfiles()
		}

		allProfiles, defaultName, err := profiles.List()
		if err != nil {
			return &output.Error{Code: output.CodeAPI, Message: err.Error()}
		}
		if len(allProfiles) == 0 {
			return noProfiles()
		}

		entries := make([]any, 0, len(allProfiles))
		for name, p := range allProfiles {
			binding, err := resolveProfileAccountBinding(name, p)
			if err != nil {
				return &output.Error{Code: output.CodeUsage, Message: err.Error()}
			}
			entry := map[string]any{
				"profile":  name,
				"account":  binding.Account,
				"base_url": p.BaseURL,
				"active":   name == defaultName,
			}
			if creds != nil {
				_, err := credsLoadProfileToken(name)
				entry["has_token"] = err == nil
			} else {
				entry["has_token"] = false
			}
			entries = append(entries, entry)
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("switch", "mailpiece auth switch <profile>", "Switch profile"),
		}

		printList(entries, authProfileColumns, fmt.Sprintf("%d profile(s)", len(entries)), breadcrumbs)
		return nil
	},
}

var authSwitchCmd = &cobra.Command{
	Use:   "switch PROFILE",
	Short: "Switch active profile",
	Long:  "Sets the active profile for subsequent commands.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		profileName := args[0]
		if err := profile.ValidateName(profileName); err != nil {
			return errors.NewInvalidArgsError(err.Error())
		}

		profileSnapshot, err := snapshotProfileState(profileName)
		if err != nil {
			return &output.Error{Code: output.CodeAPI, Message: err.Error()}
		}
		if profileSnapshot.previous != nil {
			if _, err := resolveProfileAccountBinding(profileName, profileSnapshot.previous); err != nil {
				return &output.Error{Code: output.CodeUsage, Message: err.Error()}
			}
		}

		hasToken := false
		if creds != nil {
			if token, err := credsLoadProfileToken(profileName); err == nil && token != "" {
				hasToken = true
			}
		}
		if !hasToken {
			return errors.NewError(fmt.Sprintf("No credentials found for profile %q. Run 'mailpiece auth login <token> --profile %s' or 'mailpiece auth email'", profileName, profileName))
		}

		globalSnapshot := snapshotGlobalConfig()
		// Preserve a saved deployment URL and seed reconstructed profiles from
		// the current effective URL.
		profileBaseURL := ""
		if profileSnapshot.previous == nil && cfg != nil {
			profileBaseURL = cfg.APIURL
		}
		if profiles != nil {
			if err := ensureProfileForAccount(profileName, "", profileBaseURL); err != nil {
				return &output.Error{Code: output.CodeAPI, Message: err.Error()}
			}
			if err := profiles.SetDefault(profileName); err != nil {
				restoreErr := restoreProfileState(profileSnapshot)
				return &output.Error{Code: output.CodeAPI, Message: stderrors.Join(err, restoreErr).Error()}
			}
		}

		profileAccount := profileName
		profileExplicitAccount := false
		profileAPIURL := ""
		if cfg != nil {
			profileAPIURL = cfg.APIURL
		}
		if profiles != nil {
			p, err := profiles.Get(profileName)
			if err != nil {
				restoreErr := restoreProfileState(profileSnapshot)
				return &output.Error{Code: output.CodeAPI, Message: stderrors.Join(err, restoreErr).Error()}
			}
			binding, bindingErr := resolveProfileAccountBinding(profileName, p)
			if bindingErr != nil {
				restoreErr := restoreProfileState(profileSnapshot)
				return &output.Error{Code: output.CodeUsage, Message: stderrors.Join(bindingErr, restoreErr).Error()}
			}
			profileAccount = binding.Account
			profileExplicitAccount = binding.Explicit
			if p.BaseURL != "" {
				profileAPIURL = p.BaseURL
			}
		}

		globalCfg := config.LoadGlobal()
		globalCfg.Account = profileAccount
		globalCfg.APIURL = profileAPIURL
		if err := globalCfg.Save(); err != nil {
			restoreErr := restoreProfileState(profileSnapshot)
			globalRestoreErr := restoreGlobalConfig(globalSnapshot)
			return &output.Error{Code: output.CodeAPI, Message: stderrors.Join(err, restoreErr, globalRestoreErr).Error()}
		}

		if cfg != nil {
			activeProfile = profileName
			activeProfileExplicitAccount = profileExplicitAccount
			cfg.Account = profileAccount
			if creds != nil {
				if t, err := credsLoadProfileToken(profileName); err == nil {
					cfg.Token = t
				}
			}
			if cfgAPIURL == "" && os.Getenv("MAILPIECE_API_URL") == "" {
				cfg.APIURL = profileAPIURL
			}
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("identity", "mailpiece identity", "Show your accounts"),
			breadcrumb("status", "mailpiece auth status", "Check auth status"),
		}

		printMutation(map[string]any{
			"profile": profileName,
			"account": profileAccount,
			"message": fmt.Sprintf("Switched to profile %s", profileName),
		}, "", breadcrumbs)
		return nil
	},
}

// newCheckedSDKClient builds a client for a token other than the resolved one,
// turning the plain-HTTP panic into a usage error.
func newCheckedSDKClient(cmd *cobra.Command, apiURL, token string) (client *mailpiece.Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			client = nil
			err = &output.Error{
				Code:    output.CodeUsage,
				Message: fmt.Sprintf("Cannot initialize SDK: %v", r),
				Hint:    "Non-localhost API URLs must use HTTPS",
			}
		}
	}()
	return newSDKClient(cmd, apiURL, token), nil
}

func init() {
	rootCmd.AddCommand(authCmd)
	authCmd.AddCommand(authLoginCmd)
	authCmd.AddCommand(authLogoutCmd)
	authCmd.AddCommand(authStatusCmd)
	authCmd.AddCommand(authListCmd)
	authCmd.AddCommand(authSwitchCmd)

	authLoginCmd.Flags().Bool("with-token", false, "Read the token from stdin")
	authLoginCmd.Flags().StringVar(&authLoginAccount, "account", "", "Account slug for this profile (e.g. 834206594)")
	authLogoutCmd.Flags().Bool("all", false, "Log out of all profiles")
}

// maxTokenBytes bounds what --with-token reads from stdin: a personal access
// token is a few dozen bytes, so anything larger is not one.
const maxTokenBytes = 4096

// readTokenFromStdin reads one access token from the command's stdin. A terminal
// is refused, since the flag exists to be piped from a secret store. The token
// is never echoed or included in an error.
func readTokenFromStdin(cmd *cobra.Command) (string, error) {
	in := cmd.InOrStdin()
	if f, ok := in.(*os.File); ok && (isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())) {
		return "", output.ErrUsageHint("--with-token reads the token from stdin, and stdin is a terminal",
			"Pipe it in: op read \"op://<vault>/<item>/credential\" | mailpiece auth login --with-token --profile <number>")
	}

	data, err := io.ReadAll(io.LimitReader(in, maxTokenBytes+1))
	if err != nil {
		return "", fmt.Errorf("reading token from stdin: %w", err)
	}
	if len(data) > maxTokenBytes {
		return "", output.ErrUsage(fmt.Sprintf("Token on stdin is longer than %d bytes; expected a single access token", maxTokenBytes))
	}

	// One line ending, as a pipe delivers it, is stripped and nothing else is:
	// trimming more would hide a malformed secret.
	token := string(data)
	if strings.HasSuffix(token, "\r\n") {
		token = strings.TrimSuffix(token, "\r\n")
	} else {
		token = strings.TrimSuffix(token, "\n")
	}
	if token == "" {
		return "", output.ErrUsageHint("No token on stdin",
			"Pipe it in: op read \"op://<vault>/<item>/credential\" | mailpiece auth login --with-token --profile <number>")
	}
	if strings.IndexFunc(token, func(c rune) bool { return unicode.IsSpace(c) || unicode.IsControl(c) }) >= 0 {
		return "", output.ErrUsage("Token on stdin must be a single line with no whitespace or control characters (one trailing line ending is allowed)")
	}
	return token, nil
}
