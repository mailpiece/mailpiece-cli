package commands

import (
	"bufio"
	stderrors "errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/basecamp/cli/output"
	"github.com/charmbracelet/huh"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var authEmailCmd = &cobra.Command{
	Use:   "email",
	Short: "Sign in with an email and a one-time code",
	Long:  "Interactive sign-in wizard. Use subcommands (start, verify, complete) for programmatic access.",
	RunE:  runAuthEmail,
}

var authEmailStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Send a one-time code to an email address",
	RunE:  runAuthEmailStart,
}

var authEmailVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify the one-time code",
	RunE:  runAuthEmailVerify,
}

var authEmailCompleteCmd = &cobra.Command{
	Use:   "complete",
	Short: "Generate and save an access token for the signed-in identity",
	Long:  "Generates an access token for the signed-in identity and saves it. The session token from the verify step is read from stdin, or from a masked prompt in a terminal.",
	RunE:  runAuthEmailComplete,
}

func init() {
	authCmd.AddCommand(authEmailCmd)
	authEmailCmd.AddCommand(authEmailStartCmd)
	authEmailCmd.AddCommand(authEmailVerifyCmd)
	authEmailCmd.AddCommand(authEmailCompleteCmd)

	authEmailStartCmd.Flags().String("email", "", "Email address")
	_ = authEmailStartCmd.MarkFlagRequired("email")

	authEmailVerifyCmd.Flags().String("code", "", "One-time code from the email")
	authEmailVerifyCmd.Flags().String("pending-token", "", "Pending authentication token from the start step")
	_ = authEmailVerifyCmd.MarkFlagRequired("code")
	_ = authEmailVerifyCmd.MarkFlagRequired("pending-token")

	authEmailCompleteCmd.Flags().String("account", "", "Account slug (required when the identity reaches several)")
	authEmailCompleteCmd.Flags().Bool("write", false, "Ask for a token that can write, not just read")

	authEmailCmd.Flags().Bool("write", false, "Ask for a token that can write, not just read")
}

// runAuthEmail is the interactive wizard: start, verify, then complete.
func runAuthEmail(cmd *cobra.Command, args []string) error {
	if IsMachineOutput() {
		return output.ErrUsageHint("auth email requires an interactive terminal: use subcommands (start, verify, complete) for programmatic access", "Run without --agent/--json/--quiet or in a TTY")
	}

	write, _ := cmd.Flags().GetBool("write")
	return emailWizard(cmd, write)
}

// emailWizard is the interactive email flow shared by `auth email` and
// `signup`: the app signs a known email in and a new one up, so a new
// identity is asked for its name and finishes signing up on the way.
func emailWizard(cmd *cobra.Command, write bool) error {
	sessions, err := newSessionsService()
	if err != nil {
		return err
	}

	var email string
	if err = huh.NewInput().
		Title("Enter your email address").
		Placeholder("you@example.com").
		Value(&email).
		Validate(requireNonEmpty("email")).
		Run(); err != nil {
		fmt.Println("Cancelled.")
		return nil //nolint:nilerr // user cancelled prompt
	}

	fmt.Print("Sending a one-time code... ")
	start, err := sessions.Start(cmd.Context(), email)
	if err != nil {
		fmt.Println("✗")
		return errors.NewError(fmt.Sprintf("Failed to send code: %v", err))
	}
	fmt.Println("✓")
	if start.DevCode != "" {
		fmt.Printf("Development code: %s\n", start.DevCode)
	} else {
		fmt.Println("Check your email for a one-time code.")
	}
	fmt.Println()

	var sessionToken string
	var requiresCompletion bool
	for {
		var code string
		if err = huh.NewInput().
			Title("Enter the one-time code from your email").
			Value(&code).
			Validate(requireNonEmpty("code")).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return nil //nolint:nilerr // user cancelled prompt
		}

		fmt.Print("Verifying code... ")
		verify, verifyErr := sessions.Verify(cmd.Context(), start.PendingAuthenticationToken, code)
		if verifyErr != nil {
			fmt.Println("✗")
			var he *mailpiece.SessionHTTPError
			if stderrors.As(verifyErr, &he) && he.StatusCode == http.StatusUnauthorized {
				var retry bool
				_ = huh.NewConfirm().Title("Invalid code. Try again?").Value(&retry).Run()
				if !retry {
					fmt.Println("Cancelled.")
					return nil
				}
				continue
			}
			return errors.NewError(fmt.Sprintf("Failed to verify code: %v", verifyErr))
		}
		fmt.Println("✓")
		sessionToken = verify.SessionToken
		requiresCompletion = verify.RequiresSignupCompletion
		break
	}

	if requiresCompletion {
		var fullName string
		if err = huh.NewInput().
			Title("Enter your full name").
			Placeholder("Jane Smith").
			Value(&fullName).
			Validate(requireNonEmpty("name")).
			Run(); err != nil {
			fmt.Println("Cancelled.")
			return nil //nolint:nilerr // user cancelled prompt
		}

		fmt.Print("Completing signup... ")
		if err = sessions.CompleteSignup(cmd.Context(), sessionToken, fullName); err != nil {
			fmt.Println("✗")
			return errors.NewError(fmt.Sprintf("Failed to complete signup: %v", err))
		}
		fmt.Println("✓")
	}

	identity, err := sessions.Identity(cmd.Context(), sessionToken)
	if err != nil {
		return errors.NewError(fmt.Sprintf("Failed to fetch accounts: %v", err))
	}
	if len(identity.Accounts) == 0 {
		return errors.NewError("This token reaches no account")
	}

	var slug string
	if len(identity.Accounts) == 1 {
		slug = mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
		fmt.Printf("Using account: %s (%s)\n", identity.Accounts[0].Name, slug)
	} else if slug, err = selectAccount(identity.Accounts); err != nil {
		fmt.Println("Cancelled.")
		return nil //nolint:nilerr // user cancelled prompt
	}

	fmt.Print("Generating access token... ")
	if _, err := saveEmailToken(cmd, sessions, sessionToken, slug, write); err != nil {
		fmt.Println("✗")
		return err
	}
	fmt.Println("✓")

	fmt.Println()
	if requiresCompletion {
		fmt.Println("✓ Signed up. Try: mailpiece doctor")
	} else {
		fmt.Println("✓ Signed in. Try: mailpiece doctor")
	}
	return nil
}

// runAuthEmailStart handles `mailpiece auth email start --email you@example.com`.
func runAuthEmailStart(cmd *cobra.Command, args []string) error {
	return emailStart(cmd, "mailpiece auth email verify")
}

// emailStart sends the one-time code and points to verifyCommand.
func emailStart(cmd *cobra.Command, verifyCommand string) error {
	email, _ := cmd.Flags().GetString("email")

	sessions, err := newSessionsService()
	if err != nil {
		return err
	}

	start, err := sessions.Start(cmd.Context(), email)
	if err != nil {
		return errors.NewError(fmt.Sprintf("Failed to send code: %v", err))
	}

	data := map[string]any{
		"pending_authentication_token": start.PendingAuthenticationToken,
	}
	if start.DevCode != "" {
		data["code"] = start.DevCode
	}

	breadcrumbs := []Breadcrumb{
		breadcrumb("verify", verifyCommand+" --code <CODE> --pending-token <TOKEN>", "Verify the one-time code"),
	}
	printMutation(signupHumanData(data, "code"), "Code sent. Check your email.", breadcrumbs)
	return nil
}

// runAuthEmailVerify handles `mailpiece auth email verify --code ABC123 --pending-token ...`.
func runAuthEmailVerify(cmd *cobra.Command, args []string) error {
	return emailVerify(cmd, "mailpiece auth email complete")
}

// emailVerify redeems the one-time code. A new identity is pointed to
// `signup complete --name`; a known one gets its accounts and is pointed to
// completeCommand.
func emailVerify(cmd *cobra.Command, completeCommand string) error {
	code, _ := cmd.Flags().GetString("code")
	pendingToken, _ := cmd.Flags().GetString("pending-token")

	sessions, err := newSessionsService()
	if err != nil {
		return err
	}

	verify, err := sessions.Verify(cmd.Context(), pendingToken, code)
	if err != nil {
		return errors.NewError(fmt.Sprintf("Failed to verify code: %v", err))
	}

	result := map[string]any{
		"session_token":              verify.SessionToken,
		"requires_signup_completion": verify.RequiresSignupCompletion,
	}

	var breadcrumbs []Breadcrumb
	if verify.RequiresSignupCompletion {
		breadcrumbs = []Breadcrumb{
			breadcrumb("complete", `mailpiece signup complete --name "<NAME>"`, "Finish signing up and save an access token (session token on stdin)"),
		}
	} else {
		identity, identityErr := sessions.Identity(cmd.Context(), verify.SessionToken)
		if identityErr != nil {
			return errors.NewError(fmt.Sprintf("Failed to fetch accounts: %v", identityErr))
		}
		accounts := make([]map[string]any, len(identity.Accounts))
		for i, acc := range identity.Accounts {
			accounts[i] = map[string]any{
				"id":   acc.ID,
				"name": acc.Name,
				"slug": mailpiece.NormalizeAccountSlug(acc.Slug),
			}
		}
		result["accounts"] = accounts

		switch len(accounts) {
		case 1:
			completeCommand += " --account " + mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
		case 0:
		default:
			completeCommand += " --account <SLUG>"
		}
		breadcrumbs = []Breadcrumb{
			breadcrumb("complete", completeCommand, "Generate and save an access token (session token on stdin)"),
		}
	}

	printDetail(signupHumanData(result, "requires_signup_completion", "accounts"), "", breadcrumbs)
	return nil
}

// runAuthEmailComplete handles `mailpiece auth email complete [--account ...] [--write]`. The session
// token is read from stdin (piped or interactive prompt) to avoid exposing it in shell history.
func runAuthEmailComplete(cmd *cobra.Command, args []string) error {
	account, _ := cmd.Flags().GetString("account")
	write, _ := cmd.Flags().GetBool("write")

	sessionToken, err := requireSessionToken()
	if err != nil {
		return err
	}

	sessions, err := newSessionsService()
	if err != nil {
		return err
	}

	slug := mailpiece.NormalizeAccountSlug(account)
	if slug == "" {
		identity, identityErr := sessions.Identity(cmd.Context(), sessionToken)
		if identityErr != nil {
			return errors.NewError(fmt.Sprintf("Failed to fetch accounts: %v", identityErr))
		}
		switch {
		case len(identity.Accounts) == 0:
			return errors.NewError("This token reaches no account")
		case len(identity.Accounts) == 1:
			slug = mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
		case IsMachineOutput():
			return errors.NewInvalidArgsError("This identity reaches several accounts; pass --account SLUG")
		default:
			if slug, err = selectAccount(identity.Accounts); err != nil {
				return err
			}
		}
	}

	permission, err := saveEmailToken(cmd, sessions, sessionToken, slug, write)
	if err != nil {
		return err
	}

	breadcrumbs := []Breadcrumb{
		breadcrumb("status", "mailpiece auth status", "Check auth status"),
		breadcrumb("doctor", "mailpiece doctor", "Check CLI health"),
	}
	printMutation(map[string]any{
		"authenticated": true,
		"account":       slug,
		"permission":    permission,
	}, "Signed in", breadcrumbs)
	return nil
}

// saveEmailToken generates an access token for slug with the session token
// and saves it under the sign-in's profile. It returns the permission asked for.
func saveEmailToken(cmd *cobra.Command, sessions *mailpiece.SessionsService, sessionToken, slug string, write bool) (string, error) {
	if err := validateAccountIdentifier(slug); err != nil {
		return "", errors.NewInvalidArgsError(err.Error())
	}

	permission := "read"
	if write {
		permission = "write"
	}

	token, err := sessions.CreateAccessToken(cmd.Context(), sessionToken, slug, "Mailpiece CLI", permission)
	if err != nil {
		return "", errors.NewError(fmt.Sprintf("Failed to generate access token: %v", err))
	}

	if err := saveLoginWithFallback(slug, authEmailAPIURL(), token.Token); err != nil {
		return "", &output.Error{Code: output.CodeAPI, Message: err.Error()}
	}
	return permission, nil
}

// newSessionsService builds the session flow's client against the API URL.
func newSessionsService() (*mailpiece.SessionsService, error) {
	sessions, err := mailpiece.NewSessionsService(authEmailAPIURL())
	if err != nil {
		return nil, &output.Error{Code: output.CodeUsage, Message: err.Error()}
	}
	return sessions, nil
}

// authEmailAPIURL returns the effective API URL for the sign-in flow, normalized without a trailing slash.
func authEmailAPIURL() string {
	if cfg == nil {
		return ""
	}
	return mailpiece.NormalizeBaseURL(cfg.APIURL)
}

func requireNonEmpty(field string) func(string) error {
	return func(s string) error {
		if s == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}
}

// signupHumanData removes machine-only secrets from human-facing output while
// preserving the full payload for JSON/quiet/agent workflows.
func signupHumanData(data map[string]any, fields ...string) map[string]any {
	if !isHumanOutput() {
		return data
	}

	result := make(map[string]any, len(fields))
	for _, field := range fields {
		if value, ok := data[field]; ok {
			result[field] = value
		}
	}
	return result
}

// selectAccount asks which of an identity's accounts to use.
func selectAccount(accounts []mailpiece.IdentityAccount) (string, error) {
	options := make([]huh.Option[string], len(accounts))
	for i, acc := range accounts {
		s := mailpiece.NormalizeAccountSlug(acc.Slug)
		options[i] = huh.NewOption(fmt.Sprintf("%s (%s)", acc.Name, s), s)
	}
	var slug string
	err := huh.NewSelect[string]().Title("Select your account").Options(options...).Value(&slug).Run()
	return slug, err
}

// requireSessionToken reads the session token, refusing an empty one.
func requireSessionToken() (string, error) {
	sessionToken, err := readSessionToken()
	if err != nil {
		return "", errors.NewError(fmt.Sprintf("Failed to read session token: %v", err))
	}
	if sessionToken == "" {
		return "", errors.NewInvalidArgsError("Session token is required (pipe via stdin or enter when prompted)")
	}
	return sessionToken, nil
}

// readSessionToken reads a session token from stdin. If stdin is a terminal,
// it prompts interactively with masked input. If stdin is a pipe, it reads
// a line silently.
func readSessionToken() (string, error) {
	if isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		var token string
		err := huh.NewInput().
			Title("Enter session token").
			EchoMode(huh.EchoModePassword).
			Value(&token).
			Run()
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(token), nil
	}

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", nil
}
