package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

var journeyTriggersUsage = "TRIGGER is one of: " + strings.Join(mailpiece.JourneyTriggers, ", ") + "."

// journeyCmd is the newsletter's journeys: the emails mailed by themselves, in order, to a
// person after they confirm or after they unsubscribe. Admins only.
var journeyCmd = &cobra.Command{
	Use:         "journey",
	Annotations: map[string]string{"group": "core"},
	Short:       "Manage the newsletter's journeys and their emails",
	Long: "Commands for journeys: the emails mailed by themselves to a person after they confirm their subscription\n" +
		"(subscriber_confirmed, each email after its own delay) or after they unsubscribe (subscriber_unsubscribed,\n" +
		"one email, at once). A journey is known by its trigger and always exists. Admins only.",
	Example: "$ mailpiece journey list\n$ mailpiece journey show subscriber_confirmed\n$ mailpiece journey email create subscriber_confirmed --subject Welcome --body \"Glad you are here.\" --delay-days 0",
}

var journeyColumns = render.Columns{
	{Header: "Trigger", Field: "trigger"},
}

var journeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List journeys",
	Long:  "Lists every journey, each with its emails in full.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		journeys, _, err := mailpiece.NewJourneysService(getSDK()).List(cmd.Context())
		if err != nil {
			return convertSDKError(err)
		}
		items := normalizeAny(journeys)
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", "mailpiece journey show TRIGGER", "View a journey and its emails"),
		}
		printList(items, journeyColumns, fmt.Sprintf("%d journeys", dataCount(items)), breadcrumbs)
		return nil
	},
}

var journeyShowCmd = &cobra.Command{
	Use:   "show TRIGGER",
	Short: "Show a journey and its emails",
	Long:  "Shows a journey with its emails in full. " + journeyTriggersUsage,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewJourneysService(getSDK()).Get(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("email", fmt.Sprintf("mailpiece journey email show %s EMAIL_ID", args[0]), "View one email"),
			breadcrumb("create", fmt.Sprintf("mailpiece journey email create %s --subject SUBJECT --body TEXT", args[0]), "Add an email"),
			breadcrumb("activate", fmt.Sprintf("mailpiece journey email activate %s EMAIL_ID", args[0]), "Switch an email on"),
		}
		printDetail(normalizeAny(data), fmt.Sprintf("%d emails", len(data.Emails)), breadcrumbs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(journeyCmd)

	journeyCmd.AddCommand(journeyListCmd)
	journeyCmd.AddCommand(journeyShowCmd)
}
