package commands

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// settingsCmd is the newsletter's own configuration. Each part is a resource of its own in the
// app, so each is a subcommand of its own here, with show and update. Admins only.
var settingsCmd = &cobra.Command{
	Use:         "settings",
	Annotations: map[string]string{"group": "admin"},
	Short:       "Manage the newsletter's settings",
	Long: "Commands for the newsletter's own settings, one subcommand per part: its name and language, the address it\n" +
		"answers at, who its mail is from, its subscription form, where the mail server reports bounces, the code\n" +
		"people join its team with, and what you are emailed about. Admins only.",
	Example: "$ mailpiece settings newsletter show\n$ mailpiece settings newsletter update --name \"Acme News\" --locale es\n$ mailpiece settings sending update --from-name Acme --from-address news@acme.example\n$ mailpiece settings join-code show\n$ mailpiece settings notifications update --new-subscriber=false",
}

// settingsShow is a part's show command: it prints what get answers.
func settingsShow[T any](short string, get func(context.Context, *mailpiece.SettingsService) (*T, *mailpiece.Response, error), breadcrumbs ...Breadcrumb) *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAuthAndAccount(); err != nil {
				return err
			}
			data, _, err := get(cmd.Context(), mailpiece.NewSettingsService(getSDK()))
			if err != nil {
				return convertSDKError(err)
			}
			printDetail(normalizeAny(data), "", breadcrumbs)
			return nil
		},
	}
}

// settingsUpdate is a part's update command: it sends what request builds from the flags and
// prints the part as the app answers it.
func settingsUpdate[T any](short, long string, update func(context.Context, *mailpiece.SettingsService, *cobra.Command) (*T, *mailpiece.Response, error)) *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireAuthAndAccount(); err != nil {
				return err
			}
			data, _, err := update(cmd.Context(), mailpiece.NewSettingsService(getSDK()), cmd)
			if err != nil {
				return convertSDKError(err)
			}
			printMutation(normalizeAny(data), "", nil)
			return nil
		},
	}
}

var settingsNewsletterCmd = &cobra.Command{
	Use:   "newsletter",
	Short: "The newsletter's name, description and language",
}

var settingsNewsletterShowCmd = settingsShow("Show the newsletter's name, description and language",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.Newsletter, *mailpiece.Response, error) {
		return s.Newsletter(ctx)
	},
	breadcrumb("update", "mailpiece settings newsletter update --name NAME", "Change it"),
)

var settingsNewsletterName, settingsNewsletterDescription, settingsNewsletterDescriptionFile, settingsNewsletterLocale string

var settingsNewsletterUpdateCmd = settingsUpdate("Change the newsletter's name, description or language",
	"Changes only what is passed. The name and description are public, and the language is the one every page and\n"+
		"every email of the newsletter is written in.",
	func(ctx context.Context, s *mailpiece.SettingsService, cmd *cobra.Command) (*mailpiece.Newsletter, *mailpiece.Response, error) {
		req := &mailpiece.NewsletterRequest{
			Name:   changedString(cmd, "name", &settingsNewsletterName),
			Locale: changedString(cmd, "locale", &settingsNewsletterLocale),
		}
		description, given, err := richTextFromFlags(cmd, "description", settingsNewsletterDescription, settingsNewsletterDescriptionFile)
		if err != nil {
			return nil, nil, err
		}
		if given {
			req.Description = &description
		}
		return s.UpdateNewsletter(ctx, req)
	})

var settingsHandleCmd = &cobra.Command{
	Use:   "handle",
	Short: "The handle the newsletter answers at",
}

var settingsHandleShowCmd = settingsShow("Show the newsletter's handle",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.Handle, *mailpiece.Response, error) {
		return s.Handle(ctx)
	})

var settingsHandleHandle string

var settingsHandleUpdateCmd = settingsUpdate("Change or clear the newsletter's handle",
	"Sets the word the newsletter answers at beside its number. An empty value clears it, and the newsletter answers\n"+
		"at its number alone. The old handle is free for another newsletter at once, and every link already mailed with\n"+
		"it stops working. An agent must confirm with you before running it.",
	func(ctx context.Context, s *mailpiece.SettingsService, cmd *cobra.Command) (*mailpiece.Handle, *mailpiece.Response, error) {
		if !cmd.Flags().Changed("handle") {
			return nil, nil, newRequiredFlagError("handle")
		}
		return s.UpdateHandle(ctx, &mailpiece.HandleRequest{Handle: settingsHandleHandle})
	})

var settingsSendingCmd = &cobra.Command{
	Use:   "sending",
	Short: "Who the newsletter's mail is from",
}

var settingsSendingShowCmd = settingsShow("Show who the newsletter's mail is from",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.Sender, *mailpiece.Response, error) {
		return s.Sender(ctx)
	},
	breadcrumb("update", "mailpiece settings sending update --from-name NAME --from-address ADDRESS", "Change it"),
)

var settingsSendingFromName, settingsSendingFromAddress, settingsSendingReplyTo string

var settingsSendingUpdateCmd = settingsUpdate("Change who the newsletter's mail is from",
	"Changes only what is passed. A newsletter mails nothing until it has a from name and a from address.",
	func(ctx context.Context, s *mailpiece.SettingsService, cmd *cobra.Command) (*mailpiece.Sender, *mailpiece.Response, error) {
		return s.UpdateSender(ctx, &mailpiece.SenderRequest{
			FromName:       changedString(cmd, "from-name", &settingsSendingFromName),
			FromAddress:    changedString(cmd, "from-address", &settingsSendingFromAddress),
			ReplyToAddress: changedString(cmd, "reply-to-address", &settingsSendingReplyTo),
		})
	})

var settingsSubscribeFormCmd = &cobra.Command{
	Use:   "subscribe-form",
	Short: "Where the embedded subscription form sends a person back to",
}

var settingsSubscribeFormShowCmd = settingsShow("Show where the embedded subscription form sends a person back to",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.SubscribeForm, *mailpiece.Response, error) {
		return s.SubscribeForm(ctx)
	})

var settingsSubscribeFormReturnURL string

var settingsSubscribeFormUpdateCmd = settingsUpdate("Set where the embedded subscription form sends a person back to",
	"Sets the address a person lands on after subscribing from the form embedded in another site. An empty value clears it.",
	func(ctx context.Context, s *mailpiece.SettingsService, cmd *cobra.Command) (*mailpiece.SubscribeForm, *mailpiece.Response, error) {
		if !cmd.Flags().Changed("return-url") {
			return nil, nil, newRequiredFlagError("return-url")
		}
		return s.UpdateSubscribeForm(ctx, &mailpiece.SubscribeFormRequest{SubscriptionReturnURL: settingsSubscribeFormReturnURL})
	})

var settingsBouncesCmd = &cobra.Command{
	Use:   "bounces",
	Short: "Where the mail server reports bounces",
}

var settingsBouncesShowCmd = settingsShow("Show the webhook address to give the mail server",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.Bounces, *mailpiece.Response, error) {
		return s.Bounces(ctx)
	})

var settingsJoinCodeCmd = &cobra.Command{
	Use:   "join-code",
	Short: "The code people join the newsletter's team with",
}

var settingsJoinCodeShowCmd = settingsShow("Show the join code, its link and how often it has been used",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.JoinCode, *mailpiece.Response, error) {
		return s.JoinCode(ctx)
	},
	breadcrumb("update", "mailpiece settings join-code update --usage-limit N", "Change how many times it can be used"),
	breadcrumb("reset", "mailpiece settings join-code reset", "Retire it and issue a new one"),
)

var settingsJoinCodeUsageLimit int64

var settingsJoinCodeUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Change how many times the join code can be used",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("usage-limit") {
			return newRequiredFlagError("usage-limit")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.JoinCodeRequest{UsageLimit: settingsJoinCodeUsageLimit}
		if _, err := mailpiece.NewSettingsService(getSDK()).UpdateJoinCode(cmd.Context(), req); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"updated": true, "usage_limit": settingsJoinCodeUsageLimit}, "", nil)
		return nil
	},
}

var settingsJoinCodeResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Retire the join code and issue a new one",
	Long: "Retires the join code and issues a new one, used no times. The old code and every link carrying it stop\n" +
		"working. This cannot be undone.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewSettingsService(getSDK()).ResetJoinCode(cmd.Context()); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{breadcrumb("show", "mailpiece settings join-code show", "See the new code")}
		printMutation(map[string]any{"reset": true}, "", breadcrumbs)
		return nil
	},
}

var settingsNotificationsCmd = &cobra.Command{
	Use:   "notifications",
	Short: "What you are emailed about",
}

var settingsNotificationsShowCmd = settingsShow("Show whether you are emailed when someone confirms a subscription",
	func(ctx context.Context, s *mailpiece.SettingsService) (*mailpiece.Notifications, *mailpiece.Response, error) {
		return s.Notifications(ctx)
	},
	breadcrumb("update", "mailpiece settings notifications update --new-subscriber=false", "Stop these emails"),
)

var settingsNotificationsNewSubscriber bool

var settingsNotificationsUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Set whether you are emailed when someone confirms a subscription",
	Long:  "Sets it for your own user on this newsletter, and for nobody else. It is on until switched off.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !cmd.Flags().Changed("new-subscriber") {
			return newRequiredFlagError("new-subscriber")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		req := &mailpiece.NotificationsRequest{NewSubscriberNotifications: settingsNotificationsNewSubscriber}
		if _, err := mailpiece.NewSettingsService(getSDK()).UpdateNotifications(cmd.Context(), req); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"updated": true, "new_subscriber_notifications": settingsNotificationsNewSubscriber}, "", nil)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(settingsCmd)

	settingsNewsletterUpdateCmd.Flags().StringVar(&settingsNewsletterName, "name", "", "Newsletter name")
	registerRichTextFlags(settingsNewsletterUpdateCmd, &settingsNewsletterDescription, &settingsNewsletterDescriptionFile, "description", "", "Description, as Markdown or HTML")
	settingsNewsletterUpdateCmd.Flags().StringVar(&settingsNewsletterLocale, "locale", "", "Language the newsletter is written in: "+strings.Join(mailpiece.NewsletterLocales, " or "))
	settingsNewsletterCmd.AddCommand(settingsNewsletterShowCmd, settingsNewsletterUpdateCmd)

	settingsHandleUpdateCmd.Flags().StringVar(&settingsHandleHandle, "handle", "", "Handle: 3 to 40 lowercase letters, digits and hyphens, starting with a letter (required; empty clears it)")
	settingsHandleCmd.AddCommand(settingsHandleShowCmd, settingsHandleUpdateCmd)

	settingsSendingUpdateCmd.Flags().StringVar(&settingsSendingFromName, "from-name", "", "Name the mail is from")
	settingsSendingUpdateCmd.Flags().StringVar(&settingsSendingFromAddress, "from-address", "", "Address the mail is from")
	settingsSendingUpdateCmd.Flags().StringVar(&settingsSendingReplyTo, "reply-to-address", "", "Address replies go to (empty clears it)")
	settingsSendingCmd.AddCommand(settingsSendingShowCmd, settingsSendingUpdateCmd)

	settingsSubscribeFormUpdateCmd.Flags().StringVar(&settingsSubscribeFormReturnURL, "return-url", "", "Address to send a person back to (required; empty clears it)")
	settingsSubscribeFormCmd.AddCommand(settingsSubscribeFormShowCmd, settingsSubscribeFormUpdateCmd)

	settingsBouncesCmd.AddCommand(settingsBouncesShowCmd)

	settingsJoinCodeUpdateCmd.Flags().Int64Var(&settingsJoinCodeUsageLimit, "usage-limit", 0, "Times the code can be used (required)")
	settingsJoinCodeCmd.AddCommand(settingsJoinCodeShowCmd, settingsJoinCodeUpdateCmd, settingsJoinCodeResetCmd)

	settingsNotificationsUpdateCmd.Flags().BoolVar(&settingsNotificationsNewSubscriber, "new-subscriber", false, "Email you when someone confirms a subscription: true or false (required)")
	settingsNotificationsCmd.AddCommand(settingsNotificationsShowCmd, settingsNotificationsUpdateCmd)

	settingsCmd.AddCommand(settingsNewsletterCmd, settingsHandleCmd, settingsSendingCmd, settingsSubscribeFormCmd, settingsBouncesCmd, settingsJoinCodeCmd, settingsNotificationsCmd)
}
