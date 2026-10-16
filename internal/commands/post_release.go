package commands

import (
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// Releasing a post: its publication, schedule, delivery, test delivery and listing.

const (
	postDayUsage  = "Day, YYYY-MM-DD, read with --hour in the app's time zone"
	postHourUsage = "Whole hour of --day, 0 to 23 (default 0)"
)

var (
	postPublishDestination string
	postPublishAt          string
	postPublishDay         string
	postPublishHour        int
)

var postPublishCmd = &cobra.Command{
	Use:   "publish POST_ID",
	Short: "Publish a post on the web, mailing nobody",
	Long: "Gives a post its page. Nobody is mailed: use 'mailpiece post deliver' for that.\n\n" +
		"A draft is published now with its own visibility, or with the one --destination names: web\n" +
		"(listed) or link (unlisted, reached by its link only). With --published-at, or --day and\n" +
		"--hour, it is released under that past date instead, as an old issue taking its place in the\n" +
		"archive.\n\n" +
		"An issue mailed by email only is given a page, listed. A post that already has its page is\n" +
		"answered as it is. An email-only draft is refused (403): it is never published without\n" +
		"being sent.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.PublishPostRequest{
			Destination: postPublishDestination, PublishedAt: postPublishAt,
			Day: postPublishDay, Hour: changedInt(cmd, "hour", &postPublishHour),
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Publish(cmd.Context(), args[0], req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", postBreadcrumbs(data))
		return nil
	},
}

var postUnpublishCmd = &cobra.Command{
	Use:   "unpublish POST_ID",
	Short: "Take a post's page down",
	Long: "Takes a post's page down. A post never mailed is a draft again. An issue already mailed is\n" +
		"never a draft and never mailed twice: it stays sent, by email only, and its links stop\n" +
		"answering until 'mailpiece post publish' gives it a page again.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewPostsService(getSDK()).Unpublish(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"unpublished": true, "id": args[0]}, "",
			[]Breadcrumb{breadcrumb("show", "mailpiece post show "+args[0], "See what is left of it")})
		return nil
	},
}

var (
	postScheduleDestination string
	postScheduleAt          string
	postScheduleDay         string
	postScheduleHour        int
	postScheduleDeliver     bool
)

var postScheduleCmd = &cobra.Command{
	Use:   "schedule POST_ID",
	Short: "Set when a draft is released by itself",
	Long: "Schedules a draft for a time still to come, given as --at or as --day and --hour. It is\n" +
		"still a draft until then. Scheduling again moves the time.\n\n" +
		"Without --destination the post keeps its visibility and is published only; --deliver also\n" +
		"mails it to the whole list at that time, which cannot be undone once it runs. A named\n" +
		"--destination decides both by itself: web_and_email, email (no page), web or link (reached\n" +
		"by its link only). The two that mail are offered only while the newsletter has a sender and\n" +
		"someone active on its list.\n\n" +
		"Refused with 403 for a post that is not a draft or a destination not offered, and with 422\n" +
		"for a time not ahead or an email-only post that would not be mailed.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if postScheduleAt == "" && postScheduleDay == "" {
			return errors.NewInvalidArgsError("A schedule needs a time: --at, or --day and --hour")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.SchedulePostRequest{
			Destination: postScheduleDestination, ScheduledPostingAt: postScheduleAt,
			Day: postScheduleDay, Hour: changedInt(cmd, "hour", &postScheduleHour),
			ScheduledDelivery: postScheduleDeliver,
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Schedule(cmd.Context(), args[0], req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "",
			[]Breadcrumb{breadcrumb("unschedule", "mailpiece post unschedule "+args[0], "Cancel the schedule")})
		return nil
	},
}

var postUnscheduleCmd = &cobra.Command{
	Use:   "unschedule POST_ID",
	Short: "Cancel a draft's schedule",
	Long:  "Cancels a draft's schedule. The post stays a draft, released by nothing until it is asked again.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewPostsService(getSDK()).Unschedule(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"unscheduled": true, "id": args[0]}, "", nil)
		return nil
	},
}

var postDeliverCmd = &cobra.Command{
	Use:   "deliver POST_ID",
	Short: "Mail a post to the whole list",
	Long: "Mails a post to everyone active on the list, now. A draft is published first, with its own\n" +
		"visibility. This cannot be undone: the mail leaves, and a post is\n" +
		"sent once.\n\n" +
		"The answer says which happened: only this request starting the mailing mails anyone. A\n" +
		"post already sent is answered as it is and mailed to nobody again. With nobody active on\n" +
		"the list the post is published and stays unsent (sent_at is null). Refused with 422 while\n" +
		"the newsletter has no sender.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, resp, err := mailpiece.NewPostsService(getSDK()).Deliver(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), postDeliverySummary(data, resp), postBreadcrumbs(data))
		return nil
	},
}

// postDeliverySummary says what a delivery did. The app answers 201 only when this request
// started the mailing; a 200 started nothing, and sent_at tells why.
func postDeliverySummary(post *mailpiece.Post, resp *mailpiece.Response) string {
	switch {
	case resp.StatusCode == http.StatusCreated:
		return fmt.Sprintf("Mailing started, to %d subscribers", post.DeliveriesCount)
	case post.SentAt != nil:
		return "Already sent; nobody was mailed again"
	default:
		return "Published; nobody is active on the list, so nobody was mailed"
	}
}

var postTestTo string

var postTestCmd = &cobra.Command{
	Use:   "test POST_ID",
	Short: "Mail a test of a post to one address",
	Long: "Mails the post as it stands to one address: yours, or the one --to names. Nothing about the\n" +
		"post changes and the list is not mailed. Refused with 422 for an address that is not one or\n" +
		"while the newsletter has no sender; at most 10 tests in 3 minutes.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.TestPostDeliveryRequest{EmailAddress: postTestTo}
		if _, err := mailpiece.NewPostsService(getSDK()).DeliverTest(cmd.Context(), args[0], req); err != nil {
			return convertSDKError(err)
		}
		result := map[string]any{"test_sent": true, "id": args[0]}
		if postTestTo != "" {
			result["email_address"] = postTestTo
		}
		printMutation(result, "", nil)
		return nil
	},
}

var postRelistCmd = &cobra.Command{
	Use:   "relist POST_ID",
	Short: "Show a published post in the archive and the feed",
	Long: "Makes a published post that is on the web listed: shown in the archive and the feed.\n" +
		"Refused with 403 for a draft or an email-only post.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewPostsService(getSDK()).Relist(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"visibility": "listed", "id": args[0]}, "", nil)
		return nil
	},
}

var postUnlistCmd = &cobra.Command{
	Use:   "unlist POST_ID",
	Short: "Keep a published post's page and take it out of the archive and the feed",
	Long: "Makes a published post that is on the web unlisted: its page answers at its link and it is\n" +
		"shown in neither the archive nor the feed. Refused with 403 for a draft or an email-only post.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewPostsService(getSDK()).Unlist(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"visibility": "unlisted", "id": args[0]}, "", nil)
		return nil
	},
}

func init() {
	postPublishCmd.Flags().StringVar(&postPublishDestination, "destination", "", "Where it goes: web (listed) or link (unlisted). Default: the post's own visibility")
	postPublishCmd.Flags().StringVar(&postPublishAt, "published-at", "", "Release a draft under this past date, ISO 8601")
	postPublishCmd.Flags().StringVar(&postPublishDay, "day", "", postDayUsage+", to release a draft under a past date")
	postPublishCmd.Flags().IntVar(&postPublishHour, "hour", 0, postHourUsage)
	postCmd.AddCommand(postPublishCmd)

	postCmd.AddCommand(postUnpublishCmd)

	postScheduleCmd.Flags().StringVar(&postScheduleDestination, "destination", "", "Where it goes: web_and_email, email, web or link. Default: the post's own visibility")
	postScheduleCmd.Flags().StringVar(&postScheduleAt, "at", "", "When to release it, ISO 8601, still to come")
	postScheduleCmd.Flags().StringVar(&postScheduleDay, "day", "", postDayUsage)
	postScheduleCmd.Flags().IntVar(&postScheduleHour, "hour", 0, postHourUsage)
	postScheduleCmd.Flags().BoolVar(&postScheduleDeliver, "deliver", false, "Also mail it to the whole list then (without --destination)")
	postCmd.AddCommand(postScheduleCmd)

	postCmd.AddCommand(postUnscheduleCmd)
	postCmd.AddCommand(postDeliverCmd)

	postTestCmd.Flags().StringVar(&postTestTo, "to", "", "Address to mail the test to (default: your own)")
	postCmd.AddCommand(postTestCmd)

	postCmd.AddCommand(postRelistCmd)
	postCmd.AddCommand(postUnlistCmd)
}
