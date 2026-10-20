package commands

import (
	"fmt"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/tui"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

var signupCmd = &cobra.Command{
	Use:     "signup",
	Short:   "Sign up or generate an access token",
	Long:    "Interactive signup wizard. Use subcommands (start, verify, complete) for programmatic access.",
	Example: "$ mailpiece signup\n$ mailpiece signup start --email you@example.com",
	RunE:    runSignup,
}

var signupStartCmd = &cobra.Command{
	Use:     "start",
	Short:   "Send a one-time code to an email address",
	Example: "$ mailpiece signup start --email you@example.com",
	RunE:    runSignupStart,
}

var signupVerifyCmd = &cobra.Command{
	Use:     "verify",
	Short:   "Verify the one-time code",
	Example: "$ mailpiece signup verify --code CODE --pending-token TOKEN",
	RunE:    runSignupVerify,
}

var signupCompleteCmd = &cobra.Command{
	Use:     "complete",
	Short:   "Complete signup and generate an access token",
	Long:    "Completes a new identity's signup with --name, or picks an existing identity's account with --account, then generates and saves an access token. The session token from the verify step is read from stdin, or from a masked prompt in a terminal.",
	Example: "$ mailpiece signup complete --name \"Jane Smith\" < /tmp/mailpiece-session\n$ mailpiece signup complete --account 834206594 < /tmp/mailpiece-session",
	RunE:    runSignupComplete,
}

func init() {
	rootCmd.AddCommand(signupCmd)
	signupCmd.AddCommand(signupStartCmd)
	signupCmd.AddCommand(signupVerifyCmd)
	signupCmd.AddCommand(signupCompleteCmd)

	signupStartCmd.Flags().String("email", "", "Email address")
	_ = signupStartCmd.MarkFlagRequired("email")

	signupVerifyCmd.Flags().String("code", "", "One-time code from the email")
	signupVerifyCmd.Flags().String("pending-token", "", "Pending authentication token from the start step")
	_ = signupVerifyCmd.MarkFlagRequired("code")
	_ = signupVerifyCmd.MarkFlagRequired("pending-token")

	signupCompleteCmd.Flags().String("name", "", "Full name (required for new users)")
	signupCompleteCmd.Flags().String("account", "", "Account slug (required for existing users)")
	signupCompleteCmd.Flags().Bool("write", false, "Ask for a token that can write, not just read")

	signupCmd.Flags().Bool("write", false, "Ask for a token that can write, not just read")
}

// runSignup is the interactive wizard that walks through the entire signup flow.
func runSignup(cmd *cobra.Command, args []string) error {
	if IsMachineOutput() {
		return output.ErrUsageHint("signup requires an interactive terminal: use subcommands (start, verify, complete) for programmatic access", "Run without --agent/--json/--quiet or in a TTY")
	}

	aw, wait := tui.AnimateBannerAsync(cmd.ErrOrStderr())
	fmt.Fprintln(aw)
	fmt.Fprintln(aw, "Welcome to Mailpiece CLI signup!")
	fmt.Fprintln(aw)
	wait()

	write, _ := cmd.Flags().GetBool("write")
	return emailWizard(cmd, write)
}

// runSignupStart handles `mailpiece signup start --email you@example.com`.
func runSignupStart(cmd *cobra.Command, args []string) error {
	return emailStart(cmd, "mailpiece signup verify")
}

// runSignupVerify handles `mailpiece signup verify --code ABC123 --pending-token ...`.
func runSignupVerify(cmd *cobra.Command, args []string) error {
	return emailVerify(cmd, "mailpiece signup complete")
}

// runSignupComplete handles `mailpiece signup complete`. The session token is read
// from stdin (piped or interactive prompt) to avoid exposing it in shell history.
func runSignupComplete(cmd *cobra.Command, args []string) error {
	name, _ := cmd.Flags().GetString("name")
	account, _ := cmd.Flags().GetString("account")
	write, _ := cmd.Flags().GetBool("write")

	sessionToken, err := requireSessionToken()
	if err != nil {
		return err
	}

	if name == "" && account == "" {
		return errors.NewInvalidArgsError("Either --name (new user) or --account (existing user) is required")
	}
	if name != "" && account != "" {
		return errors.NewInvalidArgsError("Use --name for new users or --account for existing users, not both")
	}

	sessions, err := newSessionsService()
	if err != nil {
		return err
	}

	var slug string
	if name != "" {
		if err = sessions.CompleteSignup(cmd.Context(), sessionToken, name); err != nil {
			return errors.NewError(fmt.Sprintf("Failed to complete signup: %v", err))
		}

		identity, identityErr := sessions.Identity(cmd.Context(), sessionToken)
		if identityErr != nil {
			return errors.NewError(fmt.Sprintf("Failed to fetch account: %v", identityErr))
		}
		if len(identity.Accounts) == 0 {
			return errors.NewError("No accounts found after signup")
		}
		slug = mailpiece.NormalizeAccountSlug(identity.Accounts[0].Slug)
	} else {
		slug = mailpiece.NormalizeAccountSlug(account)
	}

	permission, err := saveEmailToken(cmd, sessions, sessionToken, slug, write)
	if err != nil {
		return err
	}

	result := map[string]any{
		"authenticated": true,
		"account":       slug,
		"permission":    permission,
	}
	if name != "" {
		result["is_new_user"] = true
	}

	breadcrumbs := []Breadcrumb{
		breadcrumb("status", "mailpiece auth status", "Check auth status"),
		breadcrumb("setup", "mailpiece setup", "Full interactive setup"),
	}
	printMutation(result, "Configuration saved.", breadcrumbs)
	return nil
}
