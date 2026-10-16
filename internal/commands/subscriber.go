package commands

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// subscriberCmd is the newsletter's list of subscribers. Admins only.
var subscriberCmd = &cobra.Command{
	Use:         "subscriber",
	Annotations: map[string]string{"group": "core"},
	Short:       "Manage the newsletter's subscribers",
	Long: "Commands for the people on the newsletter's list. Admins only.\n\n" +
		"A subscriber is in one state, read in this order: bounced (the mail server stopped\n" +
		"delivering to the address), deactivated (an admin stopped it), unsubscribed (the person\n" +
		"left), confirmed, or unconfirmed (it has not answered its confirmation yet). Only a\n" +
		"confirmed subscriber is mailed.\n\n" +
		"Only the person undoes an unsubscribe or a bounce, by subscribing again from the public\n" +
		"form and confirming from the link mailed to them. No command here does it for them.",
	Example: "$ mailpiece subscriber list\n" +
		"$ mailpiece subscriber list --status bounced\n" +
		"$ mailpiece subscriber show SUBSCRIBER_ID\n" +
		"$ mailpiece subscriber create --email ana@example.com\n" +
		"$ mailpiece subscriber deactivate SUBSCRIBER_ID\n" +
		"$ mailpiece subscriber bounce --email ana@example.com",
}

var subscriberColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Email", Field: "email"},
	{Header: "Status", Field: "status"},
	{Header: "Created", Field: "created_at"},
}

var (
	subscriberListPages  pageFlags
	subscriberListStatus string
)

var subscriberListCmd = &cobra.Command{
	Use:   "list",
	Short: "List subscribers, the ones mailed or those in another state",
	Long: "Lists the subscribers in one state. With no --status it lists the confirmed ones, the\n" +
		"people an issue is mailed to, newest first. Each other state has a list of its own:\n" +
		"unconfirmed, unsubscribed, bounced and deactivated.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		path, err := mailpiece.SubscribersPath(subscriberListStatus)
		if err != nil {
			return convertSDKError(err)
		}
		again, noun := "mailpiece subscriber list", "subscribers"
		if subscriberListStatus != "" {
			again += " --status " + subscriberListStatus
			noun = subscriberListStatus + " subscribers"
		}
		subscribers := mailpiece.NewSubscribersService(getSDK())
		return pagedList[mailpiece.Subscriber]{
			cols: subscriberColumns, noun: noun, again: again,
			breadcrumbs: []Breadcrumb{
				breadcrumb("show", "mailpiece subscriber show SUBSCRIBER_ID", "View a subscriber and its deliveries"),
				breadcrumb("create", "mailpiece subscriber create --email EMAIL", "Add an address"),
			},
		}.run(cmd, &subscriberListPages, path, func(ctx context.Context, path string) ([]mailpiece.Subscriber, *mailpiece.Response, error) {
			return subscribers.List(ctx, subscriberListStatus, path)
		})
	},
}

var subscriberShowPage int

var subscriberShowCmd = &cobra.Command{
	Use:   "show SUBSCRIBER_ID",
	Short: "Show a subscriber with the issues mailed to it",
	Long: "Shows a subscriber and its deliveries, newest first: each post mailed to it and whether\n" +
		"it was sent, refused, left unverified or is still pending. Deliveries are paged; pass\n" +
		"--page for the next ones.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if subscriberShowPage < 0 {
			return errors.NewInvalidArgsError("--page must be 1 or more")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, resp, err := mailpiece.NewSubscribersService(getSDK()).Get(cmd.Context(), args[0], subscriberShowPage)
		if err != nil {
			return convertSDKError(err)
		}

		breadcrumbs := subscriberBreadcrumbs(&data.Subscriber)
		// The app links a next page past the end of a list.
		linkNext := ""
		if len(data.Deliveries) > 0 {
			linkNext = resp.NextPageURL()
		}
		if linkNext != "" {
			next := fmt.Sprintf("mailpiece subscriber show %s --page %d", args[0], max(subscriberShowPage, 1)+1)
			breadcrumbs = append(breadcrumbs, breadcrumb("next", next, "Next page of deliveries"))
		}

		printDetailPaginated(normalizeAny(data), "", breadcrumbs, linkNext != "", linkNext)
		return nil
	},
}

var subscriberCreateEmail string

var subscriberCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Add one address to the list, already confirmed",
	Long: "Adds one address to the list as confirmed: no confirmation is mailed to it, and the\n" +
		"confirmation's journey does not start, since the person did not confirm it themselves.\n" +
		"An address already on the list is answered as it is, and one that had not confirmed yet\n" +
		"is confirmed.\n\n" +
		"It never brings back an address that unsubscribed, bounced or was stopped: that one is\n" +
		"refused (422), as is one that is not an address, and the error says which of the four it\n" +
		"was. Only the person undoes an unsubscribe or a bounce, from the public form; a stopped\n" +
		"subscriber is resumed with `mailpiece subscriber activate`.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if strings.TrimSpace(subscriberCreateEmail) == "" {
			return newRequiredFlagError("email")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.CreateSubscriberRequest{Email: subscriberCreateEmail}
		data, _, err := mailpiece.NewSubscribersService(getSDK()).Create(cmd.Context(), req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", subscriberBreadcrumbs(data))
		return nil
	},
}

var subscriberDeleteCmd = &cobra.Command{
	Use:   "delete SUBSCRIBER_ID",
	Short: "Delete a subscriber for good",
	Long: "Deletes a subscriber for good, with its deliveries and its journey history. This changes\n" +
		"the record and cannot be undone. An agent must confirm with you before running it.\n\n" +
		"Nothing then remembers that the address unsubscribed or bounced, so it can be added again\n" +
		"as new: deleting is the one way an admin gets past either. To stop mailing someone and\n" +
		"keep their history, use `mailpiece subscriber deactivate`.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewSubscribersService(getSDK()).Delete(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("list", "mailpiece subscriber list", "List the subscribers mailed"),
		}
		printMutation(map[string]any{"deleted": true, "id": args[0]}, "", breadcrumbs)
		return nil
	},
}

var subscriberDeactivateCmd = &cobra.Command{
	Use:   "deactivate SUBSCRIBER_ID",
	Short: "Stop mailing a subscriber",
	Long: "Stops a subscriber: it stays on record, with its history, and is not mailed until it is\n" +
		"resumed with `mailpiece subscriber activate`. This is the admin's own switch. Only a\n" +
		"confirmed or unconfirmed subscriber is stopped; one that unsubscribed or bounced is\n" +
		"refused, since it is not mailed as it is. Stopping one already stopped changes nothing.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewSubscribersService(getSDK()).Deactivate(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", "mailpiece subscriber show "+args[0], "View the subscriber and its deliveries"),
			breadcrumb("activate", "mailpiece subscriber activate "+args[0], "Resume it"),
		}
		printMutation(map[string]any{"deactivated": true, "id": args[0]}, "", breadcrumbs)
		return nil
	},
}

var subscriberActivateCmd = &cobra.Command{
	Use:   "activate SUBSCRIBER_ID",
	Short: "Resume a subscriber an admin stopped",
	Long: "Resumes a stopped subscriber, which goes back to the state it was in: confirmed and\n" +
		"mailed again, or still unconfirmed. It reverses the admin's stop and nothing else: it\n" +
		"undoes neither an unsubscribe nor a bounce, which only the person does. Resuming one\n" +
		"that was not stopped changes nothing, and the answer says so.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, resp, err := mailpiece.NewSubscribersService(getSDK()).Activate(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		// The app answers 201 only when it resumed the person.
		summary := "Nothing changed: it was not stopped, and is still " + data.Status
		if resp.StatusCode == http.StatusCreated {
			summary = "Resumed; it is " + data.Status + " again"
		}
		printMutation(normalizeAny(data), summary, subscriberBreadcrumbs(data))
		return nil
	},
}

var subscriberBounceEmail string

var subscriberBounceCmd = &cobra.Command{
	Use:   "bounce [SUBSCRIBER_ID]",
	Short: "Mark a subscriber bounced, by its id or its address",
	Long: "Marks a subscriber bounced: the mail server stopped delivering to the address, so it is\n" +
		"not mailed again. Give the subscriber's id, or its address with --email. Marking one\n" +
		"already bounced changes nothing.\n\n" +
		"This changes the record and cannot be undone: only the person undoes a bounce, by\n" +
		"subscribing again from the public form and confirming from the link mailed to them. An\n" +
		"agent must confirm with you before running it.\n" +
		"With --email, an address no subscriber has is not found (404).",
	Example: "$ mailpiece subscriber bounce SUBSCRIBER_ID\n" +
		"$ mailpiece subscriber bounce --email ana@example.com",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		email := strings.TrimSpace(subscriberBounceEmail)
		if (len(args) == 1) == (email != "") {
			return errors.NewInvalidArgsError("give a SUBSCRIBER_ID or --email, not both")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		subscribers := mailpiece.NewSubscribersService(getSDK())
		if email != "" {
			if _, err := subscribers.BounceAddress(cmd.Context(), &mailpiece.BounceAddressRequest{EmailAddress: email}); err != nil {
				return convertSDKError(err)
			}
			breadcrumbs := []Breadcrumb{
				breadcrumb("list", "mailpiece subscriber list --status bounced", "List the bounced subscribers"),
			}
			printMutation(map[string]any{"bounced": true, "email_address": email}, "", breadcrumbs)
			return nil
		}
		if _, err := subscribers.Bounce(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", "mailpiece subscriber show "+args[0], "View the subscriber and its deliveries"),
		}
		printMutation(map[string]any{"bounced": true, "id": args[0]}, "", breadcrumbs)
		return nil
	},
}

// subscriberBreadcrumbs are what can be done next with a subscriber as it now stands.
func subscriberBreadcrumbs(s *mailpiece.Subscriber) []Breadcrumb {
	if s.ID == "" {
		return nil
	}
	breadcrumbs := []Breadcrumb{
		breadcrumb("show", "mailpiece subscriber show "+s.ID, "View the subscriber and its deliveries"),
	}
	switch s.Status {
	case "confirmed", "unconfirmed":
		breadcrumbs = append(breadcrumbs, breadcrumb("deactivate", "mailpiece subscriber deactivate "+s.ID, "Stop mailing it"))
	case "deactivated":
		breadcrumbs = append(breadcrumbs, breadcrumb("activate", "mailpiece subscriber activate "+s.ID, "Resume it"))
	}
	return breadcrumbs
}

func init() {
	rootCmd.AddCommand(subscriberCmd)

	subscriberListPages.register(subscriberListCmd)
	subscriberListCmd.Flags().StringVar(&subscriberListStatus, "status", "", "State to list: "+strings.Join(mailpiece.SubscriberStatuses, ", ")+" (default confirmed)")
	subscriberCmd.AddCommand(subscriberListCmd)

	subscriberShowCmd.Flags().IntVar(&subscriberShowPage, "page", 0, "Page of deliveries")
	subscriberCmd.AddCommand(subscriberShowCmd)

	subscriberCreateCmd.Flags().StringVar(&subscriberCreateEmail, "email", "", "Email address (required)")
	subscriberCmd.AddCommand(subscriberCreateCmd)

	subscriberCmd.AddCommand(subscriberDeleteCmd)
	subscriberCmd.AddCommand(subscriberDeactivateCmd)
	subscriberCmd.AddCommand(subscriberActivateCmd)

	subscriberBounceCmd.Flags().StringVar(&subscriberBounceEmail, "email", "", "Address of the subscriber to mark bounced")
	subscriberCmd.AddCommand(subscriberBounceCmd)
}
