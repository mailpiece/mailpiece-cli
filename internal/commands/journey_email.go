package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// journeyEmailCmd is the emails of a journey.
var journeyEmailCmd = &cobra.Command{
	Use:   "email",
	Short: "Read, write, switch on and test a journey's emails",
	Long: "Commands for the emails of a journey. `mailpiece journey show TRIGGER` lists them, with their ids.\n" +
		"An email is a draft until activated; an active one is mailed by itself to the people its journey reaches.\n" +
		journeyTriggersUsage,
	Example: "$ mailpiece journey email show subscriber_confirmed EMAIL_ID\n$ mailpiece journey email create subscriber_confirmed --subject Welcome --body \"Glad you are here.\"\n$ mailpiece journey email test subscriber_confirmed EMAIL_ID\n$ mailpiece journey email activate subscriber_confirmed EMAIL_ID",
}

var (
	journeyEmailSubject, journeyEmailSummary, journeyEmailBody, journeyEmailBodyFile string
	journeyEmailDelayDays, journeyEmailSendHour                                      int64
	journeyEmailAttach                                                               []string
)

func registerJourneyEmailFlags(cmd *cobra.Command, subjectUsage string) {
	cmd.Flags().StringVarP(&journeyEmailSubject, "subject", "t", "", subjectUsage)
	cmd.Flags().StringVar(&journeyEmailSummary, "summary", "", "The writer's own line about the email, shown by an inbox beside the subject")
	registerRichTextFlags(cmd, &journeyEmailBody, &journeyEmailBodyFile, "body", "c", "Body, as Markdown or HTML")
	cmd.Flags().StringArrayVar(&journeyEmailAttach, "attach", nil, "Upload a file and add it at the end of the body (repeatable)")
	cmd.Flags().Int64Var(&journeyEmailDelayDays, "delay-days", 0, "Days to wait after the trigger before mailing it (subscriber_confirmed only)")
	cmd.Flags().Int64Var(&journeyEmailSendHour, "send-hour", 0, "Hour of the day to mail it at, 0 to 23 (subscriber_confirmed only)")
}

// journeyEmailRequestFromFlags reads what was passed, the body turned into HTML, and the
// --attach files. With implicit, a body no flag gives is read from piped stdin or $EDITOR,
// which starts from current.
func journeyEmailRequestFromFlags(cmd *cobra.Command, implicit bool, current func() (string, error)) (*mailpiece.JourneyEmailRequest, []attachmentFile, error) {
	files, err := readAttachments(journeyEmailAttach)
	if err != nil {
		return nil, nil, err
	}
	req := &mailpiece.JourneyEmailRequest{
		Subject:   changedString(cmd, "subject", &journeyEmailSubject),
		Summary:   changedString(cmd, "summary", &journeyEmailSummary),
		DelayDays: changedInt(cmd, "delay-days", &journeyEmailDelayDays),
		SendHour:  changedInt(cmd, "send-hour", &journeyEmailSendHour),
	}
	body, given, err := richTextFromInput(cmd, "body", journeyEmailBody, journeyEmailBodyFile, implicit, current)
	if err != nil {
		return nil, nil, err
	}
	if given {
		req.Body = &body
	}
	return req, files, nil
}

// appendJourneyEmailAttachments uploads files and adds them at the end of the body passed,
// or with none passed, of current's.
func appendJourneyEmailAttachments(cmd *cobra.Command, req *mailpiece.JourneyEmailRequest, files []attachmentFile, current func() (string, error)) error {
	if len(files) == 0 {
		return nil
	}
	var body string
	if req.Body != nil {
		body = *req.Body
	} else if current != nil {
		var err error
		if body, err = current(); err != nil {
			return err
		}
	}
	body, err := appendAttachments(cmd.Context(), body, files)
	if err != nil {
		return err
	}
	req.Body = &body
	return nil
}

var journeyEmailShowCmd = &cobra.Command{
	Use:   "show TRIGGER EMAIL_ID",
	Short: "Show an email of a journey",
	Long:  "Shows one email of a journey in full, active or not. " + journeyTriggersUsage,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		data, _, err := getSDK().Journeys().GetEmail(cmd.Context(), args[0], args[1])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("update", fmt.Sprintf("mailpiece journey email update %s %s", args[0], args[1]), "Change the email"),
			breadcrumb("test", fmt.Sprintf("mailpiece journey email test %s %s", args[0], args[1]), "Mail yourself a test"),
		}
		printDetail(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

var journeyEmailCreateCmd = &cobra.Command{
	Use:   "create TRIGGER",
	Short: "Add an email to a journey, as a draft",
	Long: "Adds an email to a journey. It is a draft, mailed to nobody, until activated.\n" +
		"The subscriber_unsubscribed journey holds one email: a second is refused.\n" +
		"With no --body or --body-file, the body is read from piped stdin, or written in $EDITOR on a terminal.\n" +
		"--attach uploads a file and adds it at the end of the body, as the editor does.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		if journeyEmailSubject == "" {
			return newRequiredFlagError("subject")
		}
		if err := checkJourneyEmailDelay(cmd, args[0]); err != nil {
			return err
		}
		req, files, err := journeyEmailRequestFromFlags(cmd, true, nil)
		if err != nil {
			return err
		}
		if err = appendJourneyEmailAttachments(cmd, req, files, nil); err != nil {
			return err
		}
		data, _, err := getSDK().Journeys().CreateEmail(cmd.Context(), args[0], req)
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("test", fmt.Sprintf("mailpiece journey email test %s %s", args[0], data.ID), "Mail yourself a test"),
			breadcrumb("activate", fmt.Sprintf("mailpiece journey email activate %s %s", args[0], data.ID), "Switch the email on"),
		}
		printMutation(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

var journeyEmailUpdateCmd = &cobra.Command{
	Use:   "update TRIGGER EMAIL_ID",
	Short: "Change an email of a journey",
	Long: "Changes only what is passed. An active email is changed for everyone it has yet to reach,\n" +
		"and cannot lose its subject or its body.\n" +
		"Given no field at all, the new body is read from piped stdin, or edited in $EDITOR on a terminal.\n" +
		"--attach uploads a file and adds it at the end of the body, as the editor does.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		if err := checkJourneyEmailDelay(cmd, args[0]); err != nil {
			return err
		}
		implicit := noFlagChanged(cmd, "subject", "summary", "delay-days", "send-hour", "attach")
		currentBody := func() (string, error) {
			email, _, err := getSDK().Journeys().GetEmail(cmd.Context(), args[0], args[1])
			if err != nil {
				return "", convertSDKError(err)
			}
			return email.BodyHTML, nil
		}
		req, files, err := journeyEmailRequestFromFlags(cmd, implicit, currentBody)
		if err != nil {
			return err
		}
		if implicit && req.Body == nil {
			return usageWithHint("Nothing to change", fmt.Sprintf(
				"cat welcome.md | mailpiece journey email update %[1]s %[2]s  or  mailpiece journey email update %[1]s %[2]s --body-file welcome.md",
				args[0], args[1]))
		}
		if err = appendJourneyEmailAttachments(cmd, req, files, currentBody); err != nil {
			return err
		}
		data, _, err := getSDK().Journeys().UpdateEmail(cmd.Context(), args[0], args[1], req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", nil)
		return nil
	},
}

var journeyEmailDeleteCmd = &cobra.Command{
	Use:   "delete TRIGGER EMAIL_ID",
	Short: "Delete an email of a journey",
	Long:  "Deletes an email for good, active or not. This cannot be undone.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		if _, err := getSDK().Journeys().DeleteEmail(cmd.Context(), args[0], args[1]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", fmt.Sprintf("mailpiece journey show %s", args[0]), "View the journey"),
		}
		printMutation(map[string]any{"deleted": true, "id": args[1]}, "", breadcrumbs)
		return nil
	},
}

var journeyEmailActivateCmd = &cobra.Command{
	Use:   "activate TRIGGER EMAIL_ID",
	Short: "Switch an email on, so it is mailed",
	Long: "Switches an email on. From then on it is mailed by itself to real people as they reach it in the journey,\n" +
		"and mail that has gone out cannot be taken back. An agent must confirm with you before running it.\n" +
		"An email with no subject or no body is refused.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		data, _, err := getSDK().Journeys().ActivateEmail(cmd.Context(), args[0], args[1])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("deactivate", fmt.Sprintf("mailpiece journey email deactivate %s %s", args[0], args[1]), "Switch the email off"),
		}
		printMutation(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

var journeyEmailDeactivateCmd = &cobra.Command{
	Use:   "deactivate TRIGGER EMAIL_ID",
	Short: "Switch an email off, back to a draft",
	Long:  "Switches an email off: it is a draft again and is mailed to nobody until activated.",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		if _, err := getSDK().Journeys().DeactivateEmail(cmd.Context(), args[0], args[1]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("activate", fmt.Sprintf("mailpiece journey email activate %s %s", args[0], args[1]), "Switch the email on"),
		}
		printMutation(map[string]any{"deactivated": true, "id": args[1]}, "", breadcrumbs)
		return nil
	},
}

var journeyEmailTestTo string

var journeyEmailTestCmd = &cobra.Command{
	Use:   "test TRIGGER EMAIL_ID",
	Short: "Mail one test copy of an email",
	Long: "Mails one copy of an email, active or not, to --to or to your own address. It is real mail, from the\n" +
		"newsletter's sender, and cannot be taken back. Refused while the newsletter has no sender, and after 10 in 3 minutes.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if err := checkJourneyTrigger(args[0]); err != nil {
			return err
		}
		req := &mailpiece.JourneyEmailTestRequest{EmailAddress: journeyEmailTestTo}
		if _, err := getSDK().Journeys().TestEmail(cmd.Context(), args[0], args[1], req); err != nil {
			return convertSDKError(err)
		}
		result := map[string]any{"test_sent": true, "id": args[1]}
		if journeyEmailTestTo != "" {
			result["email_address"] = journeyEmailTestTo
		}
		printMutation(result, "", nil)
		return nil
	},
}

// checkJourneyEmailDelay refuses a delay on the journey that takes none, which the app
// would drop without a word.
func checkJourneyEmailDelay(cmd *cobra.Command, trigger string) error {
	if trigger == mailpiece.JourneyTriggerWithoutDelay && (cmd.Flags().Changed("delay-days") || cmd.Flags().Changed("send-hour")) {
		return errors.NewInvalidArgsError("The " + trigger + " journey mails its one email at once: it takes no --delay-days or --send-hour")
	}
	return nil
}

func init() {
	journeyCmd.AddCommand(journeyEmailCmd)

	journeyEmailCmd.AddCommand(journeyEmailShowCmd)

	registerJourneyEmailFlags(journeyEmailCreateCmd, "Subject (required)")
	journeyEmailCmd.AddCommand(journeyEmailCreateCmd)

	registerJourneyEmailFlags(journeyEmailUpdateCmd, "Subject")
	journeyEmailCmd.AddCommand(journeyEmailUpdateCmd)

	journeyEmailCmd.AddCommand(journeyEmailDeleteCmd)
	journeyEmailCmd.AddCommand(journeyEmailActivateCmd)
	journeyEmailCmd.AddCommand(journeyEmailDeactivateCmd)

	journeyEmailTestCmd.Flags().StringVar(&journeyEmailTestTo, "to", "", "Address to mail the test to (default: your own)")
	journeyEmailCmd.AddCommand(journeyEmailTestCmd)
}
