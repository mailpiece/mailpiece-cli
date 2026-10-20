package commands

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/basecamp/cli/output"
	"github.com/charmbracelet/huh"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/tui"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive setup wizard",
	Long:  "Configure the Mailpiece CLI with your API URL, personal access token, and account.\nNew users without an account will be guided through signup.",
	RunE:  runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	if cfgJQ != "" {
		return errors.ErrJQNotSupported("the setup command")
	}
	if IsMachineOutput() {
		return output.ErrUsageHint("setup requires an interactive terminal", "Run without --agent/--json/--quiet in a TTY, or use: mailpiece auth login TOKEN")
	}

	aw, wait := tui.AnimateBannerAsync(cmd.ErrOrStderr())
	fmt.Fprintln(aw)
	fmt.Fprintln(aw, "Welcome to Mailpiece CLI setup!")
	fmt.Fprintln(aw)
	wait()

	// Ask if user has an account before checking existing config
	var hasAccount string
	err := huh.NewSelect[string]().
		Title("Do you have a Mailpiece account?").
		Options(
			huh.NewOption("Yes, I have an account", "yes"),
			huh.NewOption("No, I'd like to sign up", "no"),
		).
		Value(&hasAccount).
		Run()
	if err != nil {
		fmt.Println("Setup cancelled.")
		return nil //nolint:nilerr // user cancelled prompt
	}

	if hasAccount == "no" {
		apiURL, ok := askAPIURL()
		if !ok {
			return nil
		}
		cfg.APIURL = apiURL
		return emailWizard(cmd, false)
	}

	if location := existingConfigLocation(); location != "" {
		var reconfigure bool
		err = huh.NewConfirm().
			Title(fmt.Sprintf("Existing %s found. Reconfigure?", location)).
			Value(&reconfigure).
			Run()
		if err != nil {
			fmt.Println("Setup cancelled.")
			return nil //nolint:nilerr // user cancelled prompt
		}
		if !reconfigure {
			fmt.Println("Setup cancelled. Existing configuration unchanged.")
			return nil
		}
	}

	apiURL, ok := askAPIURL()
	if !ok {
		return nil
	}

	var token string
	var identity *mailpiece.Identity
	for {
		err = huh.NewInput().
			Title("Enter your personal access token").
			Description("In Mailpiece: Profile › Personal access tokens").
			Value(&token).
			EchoMode(huh.EchoModePassword).
			Validate(func(s string) error {
				if s == "" {
					return fmt.Errorf("token is required")
				}
				return nil
			}).
			Run()
		if err != nil {
			fmt.Println("Setup cancelled.")
			return nil //nolint:nilerr // user cancelled prompt
		}

		fmt.Print("Validating token... ")
		identity, err = validateToken(cmd, apiURL, token)
		if err == nil {
			fmt.Println("✓")
			break
		}
		fmt.Println("✗")

		var retry bool
		_ = huh.NewConfirm().
			Title("Invalid token. Would you like to try again?").
			Value(&retry).
			Run()
		if !retry {
			fmt.Println("Setup cancelled.")
			return nil
		}
	}

	if len(identity.Accounts) == 0 {
		return errors.NewError("No accounts found for this token")
	}

	var slug string
	if len(identity.Accounts) == 1 {
		slug = mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
		fmt.Printf("Using account: %s (%s)\n", identity.Accounts[0].Name, slug)
	} else {
		options := make([]huh.Option[string], len(identity.Accounts))
		for i, acc := range identity.Accounts {
			s := mailpiece.NormalizeAccountSlug(acc.Slug)
			options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", acc.Name, s), s)
		}
		err = huh.NewSelect[string]().
			Title("Select your account").
			Options(options...).
			Value(&slug).
			Run()
		if err != nil {
			fmt.Println("Setup cancelled.")
			return nil //nolint:nilerr // user cancelled prompt
		}
	}

	var saveGlobal bool
	err = huh.NewSelect[bool]().
		Title("Where should we save the configuration?").
		Options(
			huh.NewOption("Global (~/.config/mailpiece/config.yaml)", true),
			huh.NewOption("Local (.mailpiece.yaml in current directory)", false),
		).
		Value(&saveGlobal).
		Run()
	if err != nil {
		fmt.Println("Setup cancelled.")
		return nil //nolint:nilerr // user cancelled prompt
	}

	if saveGlobal {
		if err := saveLoginWithFallback(slug, apiURL, token); err != nil {
			return &output.Error{Code: output.CodeAPI, Message: err.Error()}
		}
		fmt.Println()
		fmt.Printf("✓ Configuration saved to %s\n", globalConfigPathForDoctor())
	} else {
		localCfg := &config.Config{Token: token, Account: slug, APIURL: apiURL}
		if err := localCfg.SaveLocal(); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("✓ Configuration saved to .mailpiece.yaml")
		fmt.Println()
		fmt.Println("⚠ Remember to add .mailpiece.yaml to your .gitignore to avoid committing your token!")
	}

	// Coding agent integration
	if err := setupAgents(cmd); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("You're all set! Try: mailpiece doctor")
	return nil
}

// askAPIURL asks for the address of the Mailpiece install, starting from the
// configured one. It is false when the prompt is cancelled.
func askAPIURL() (string, bool) {
	apiURL := cfg.APIURL
	err := huh.NewInput().
		Title("Enter your Mailpiece address").
		Description("The address of your Mailpiece install").
		Placeholder("https://mailpiece.example.com").
		Value(&apiURL).
		Validate(validateAPIURL).
		Run()
	if err != nil {
		fmt.Println("Setup cancelled.")
		return "", false
	}
	return apiURL, true
}

// validateToken validates the token by calling the identity endpoint via the SDK.
func validateToken(cmd *cobra.Command, apiURL, token string) (*mailpiece.Identity, error) {
	client, err := newCheckedSDKClient(cmd, apiURL, token)
	if err != nil {
		return nil, err
	}
	identity, _, err := client.Identity().GetMyIdentity(cmd.Context())
	return identity, err
}

// validateAPIURL checks that the URL is well-formed and enforces HTTPS for non-localhost URLs.
func validateAPIURL(s string) error {
	if s == "" {
		return fmt.Errorf("URL is required")
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return fmt.Errorf("URL must start with http:// or https://")
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL must include a hostname")
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		if host != "localhost" && host != "127.0.0.1" && host != "::1" && !strings.HasSuffix(host, ".localhost") {
			return fmt.Errorf("non-localhost URLs must use https://")
		}
	}
	return nil
}

// existingConfigLocation names the configuration setup would replace. Saved
// profiles count too, since a login leaves no config file when the keyring holds
// the token.
func existingConfigLocation() string {
	if localPath := config.LocalConfigPath(); localPath != "" {
		return "local config (" + localPath + ")"
	}
	if config.Exists() {
		return "global config"
	}
	if allProfiles, _ := profileStoreInfo(); len(allProfiles) > 0 {
		return "saved profile"
	}
	return ""
}
