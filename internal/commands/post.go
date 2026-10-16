package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// postCmd is a newsletter's posts. Admins only.
var postCmd = &cobra.Command{
	Use:         "post",
	Annotations: map[string]string{"group": "core"},
	Short:       "Write and release the newsletter's posts",
	Long: "Commands for the newsletter's posts: writing a draft, releasing it to the web and to the\n" +
		"list, and the links and earlier versions a post keeps. Admins only.\n\n" +
		"A post is published once and sent once. A scheduled post is still a draft. A published\n" +
		"post can still be edited: its page shows the correction, the email already sent does not\n" +
		"change.",
	Example: "$ mailpiece post list\n" +
		"$ mailpiece post create -t \"Hello\" --body-file hello.md\n" +
		"$ cat issue.md | mailpiece post create -t \"Shipping week\" --attach chart.png\n" +
		"$ mailpiece post test POST_ID\n" +
		"$ mailpiece post publish POST_ID\n" +
		"$ mailpiece post deliver POST_ID",
}

var postColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Title", Field: "title"},
	{Header: "Visibility", Field: "visibility"},
	{Header: "Published", Field: "published_at"},
	{Header: "Sent", Field: "sent_at"},
	{Header: "Scheduled", Field: "scheduled_posting_at"},
}

const postBodyUsage = "The post's text, as Markdown or HTML. An <aside data-audience=\"web\"> or \"email\" box is kept as written"

var (
	postListPages     pageFlags
	postListPublished bool
	postListSent      bool
)

var postListCmd = &cobra.Command{
	Use:   "list",
	Short: "List posts: what is ahead, what is published, or what was sent",
	Long: "Lists the drafts, newest first, then the scheduled posts, soonest first, all on one page.\n" +
		"With --published, the posts published and not mailed; with --sent, the posts mailed to the\n" +
		"list. Those two are paged, newest first.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if postListPublished && postListSent {
			return errors.NewInvalidArgsError("--published and --sent cannot be used together")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		posts := mailpiece.NewPostsService(getSDK())
		breadcrumbs := make([]Breadcrumb, 0, 3)
		breadcrumbs = append(breadcrumbs, breadcrumb("show", "mailpiece post show POST_ID", "View a post"))

		switch {
		case postListPublished:
			return pagedList[mailpiece.Post]{
				cols: postColumns, noun: "published posts", again: "mailpiece post list --published", breadcrumbs: breadcrumbs,
			}.run(cmd, &postListPages, "/posts/published.json", posts.Published)
		case postListSent:
			return pagedList[mailpiece.Post]{
				cols: postColumns, noun: "sent posts", again: "mailpiece post list --sent", breadcrumbs: breadcrumbs,
			}.run(cmd, &postListPages, "/posts/sent.json", posts.Sent)
		}

		if postListPages.page != 0 {
			return errors.NewInvalidArgsError("--page goes with --published or --sent: drafts and scheduled posts come on one page")
		}
		data, _, err := posts.List(cmd.Context())
		if err != nil {
			return convertSDKError(err)
		}
		items := normalizeAny(data)
		breadcrumbs = append(breadcrumbs,
			breadcrumb("published", "mailpiece post list --published", "List the published posts"),
			breadcrumb("sent", "mailpiece post list --sent", "List the sent posts"),
		)
		printList(items, postColumns, fmt.Sprintf("%d drafts and scheduled posts", dataCount(items)), breadcrumbs)
		return nil
	},
}

var postShowCmd = &cobra.Command{
	Use:   "show POST_ID",
	Short: "Show a post",
	Long: "Shows a post in any state. body is its text as plain text and body_html the text as it was\n" +
		"written, audience boxes included.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Get(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		printDetail(normalizeAny(data), "", postBreadcrumbs(data))
		return nil
	},
}

var (
	postCreateTitle      string
	postCreateSummary    string
	postCreateBody       string
	postCreateBodyFile   string
	postCreateVisibility string
	postCreateAttach     []string
)

var postCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Write a draft",
	Long: "Writes a draft. Nothing is published or mailed: release it with 'mailpiece post publish',\n" +
		"'mailpiece post schedule' or 'mailpiece post deliver'.\n\n" +
		"With no --body or --body-file, the body is read from piped stdin, or written in $EDITOR on a\n" +
		"terminal. --attach uploads a file and adds it at the end of the body, as the editor does.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		body, bodyGiven, err := richTextFromInput(cmd, "body", postCreateBody, postCreateBodyFile, true, nil)
		if err != nil {
			return err
		}
		files, err := readAttachments(postCreateAttach)
		if err != nil {
			return err
		}
		if postCreateTitle == "" && postCreateSummary == "" && !bodyGiven && len(files) == 0 {
			return usageWithHint("A post needs a title, a summary or a body",
				`cat issue.md | mailpiece post create -t "Title"  or  mailpiece post create -t "Title" --body-file issue.md`)
		}
		if err = requireAuthAndAccount(); err != nil {
			return err
		}
		if body, err = appendAttachments(cmd.Context(), body, files); err != nil {
			return err
		}
		req := &mailpiece.CreatePostRequest{
			Title: postCreateTitle, Summary: postCreateSummary, Body: body, Visibility: postCreateVisibility,
		}
		data, resp, err := mailpiece.NewPostsService(getSDK()).Create(cmd.Context(), req)
		if err != nil {
			return convertSDKError(err)
		}
		if location := resp.Headers.Get("Location"); location != "" {
			printMutationWithLocation(normalizeAny(data), location, postBreadcrumbs(data))
		} else {
			printMutation(normalizeAny(data), "", postBreadcrumbs(data))
		}
		return nil
	},
}

var (
	postUpdateTitle       string
	postUpdateSummary     string
	postUpdateBody        string
	postUpdateBodyFile    string
	postUpdateVisibility  string
	postUpdatePublishedAt string
	postUpdateAttach      []string
)

var postUpdateCmd = &cobra.Command{
	Use:   "update POST_ID",
	Short: "Edit a post",
	Long: "Changes only what is passed. A published post can be edited: its page shows the correction,\n" +
		"and the email already sent does not change. --published-at corrects the date of a published\n" +
		"post that is on the web, to now or earlier; for any other post the app drops it without a word.\n\n" +
		"Given no field at all, the new body is read from piped stdin, or edited in $EDITOR on a terminal.\n" +
		"--attach uploads a file and adds it at the end of the body, as the editor does.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		implicit := noFlagChanged(cmd, "title", "summary", "visibility", "published-at", "attach")
		currentBody := func() (string, error) { return postBodyHTML(cmd, args[0]) }
		body, bodyGiven, err := richTextFromInput(cmd, "body", postUpdateBody, postUpdateBodyFile, implicit, currentBody)
		if err != nil {
			return err
		}
		if implicit && !bodyGiven {
			return usageWithHint("Nothing to change",
				"cat issue.md | mailpiece post update "+args[0]+"  or  mailpiece post update "+args[0]+" --body-file issue.md")
		}
		files, err := readAttachments(postUpdateAttach)
		if err != nil {
			return err
		}
		if err = requireAuthAndAccount(); err != nil {
			return err
		}
		if len(files) > 0 {
			if !bodyGiven {
				if body, err = currentBody(); err != nil {
					return err
				}
			}
			if body, err = appendAttachments(cmd.Context(), body, files); err != nil {
				return err
			}
			bodyGiven = true
		}
		req := &mailpiece.UpdatePostRequest{
			Title:       changedString(cmd, "title", &postUpdateTitle),
			Summary:     changedString(cmd, "summary", &postUpdateSummary),
			Visibility:  changedString(cmd, "visibility", &postUpdateVisibility),
			PublishedAt: changedString(cmd, "published-at", &postUpdatePublishedAt),
		}
		if bodyGiven {
			req.Body = &body
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Update(cmd.Context(), args[0], req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", postBreadcrumbs(data))
		return nil
	},
}

var postDeleteCmd = &cobra.Command{
	Use:   "delete POST_ID",
	Short: "Delete a post for good",
	Long: "Deletes a post in any state, with its revisions, deliveries and links: the links in an issue\n" +
		"already mailed stop answering. This cannot be undone.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := mailpiece.NewPostsService(getSDK()).Delete(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		printMutation(map[string]any{"deleted": true, "id": args[0]}, "", nil)
		return nil
	},
}

// postBodyHTML is a post's body as it was written, for the editor or --attach to start from.
func postBodyHTML(cmd *cobra.Command, postID string) (string, error) {
	if err := requireAuthAndAccount(); err != nil {
		return "", err
	}
	post, _, err := mailpiece.NewPostsService(getSDK()).Get(cmd.Context(), postID)
	if err != nil {
		return "", convertSDKError(err)
	}
	return post.BodyHTML, nil
}

// postBreadcrumbs are a post's next steps, by its state.
func postBreadcrumbs(post *mailpiece.Post) []Breadcrumb {
	if post == nil || post.ID == "" {
		return nil
	}
	id := post.ID
	switch {
	case post.SentAt != nil:
		return []Breadcrumb{
			breadcrumb("links", "mailpiece post link list "+id, "See the links it was mailed with"),
			breadcrumb("history", "mailpiece post history list "+id, "See its earlier versions"),
		}
	case post.Published:
		return []Breadcrumb{
			breadcrumb("deliver", "mailpiece post deliver "+id, "Mail it to the whole list (cannot be undone)"),
			breadcrumb("unpublish", "mailpiece post unpublish "+id, "Take its page down"),
		}
	}
	return []Breadcrumb{
		breadcrumb("update", "mailpiece post update "+id, "Edit it"),
		breadcrumb("test", "mailpiece post test "+id, "Mail yourself a test"),
		breadcrumb("publish", "mailpiece post publish "+id, "Publish it on the web, mailing nobody"),
		breadcrumb("deliver", "mailpiece post deliver "+id, "Publish it and mail the whole list (cannot be undone)"),
	}
}

func init() {
	rootCmd.AddCommand(postCmd)

	postListPages.register(postListCmd)
	postListCmd.Flags().BoolVar(&postListPublished, "published", false, "List the posts published and not mailed")
	postListCmd.Flags().BoolVar(&postListSent, "sent", false, "List the posts mailed to the list")
	postCmd.AddCommand(postListCmd)

	postCmd.AddCommand(postShowCmd)

	visibilityUsage := "Visibility: listed, unlisted (reached by its link only) or email_only (no page)"

	attachUsage := "Upload a file and add it at the end of the body (repeatable)"

	postCreateCmd.Flags().StringVarP(&postCreateTitle, "title", "t", "", "Title, which is also the email's subject")
	postCreateCmd.Flags().StringVar(&postCreateSummary, "summary", "", "The writer's own line about the post, shown in place of how it begins")
	registerRichTextFlags(postCreateCmd, &postCreateBody, &postCreateBodyFile, "body", "c", postBodyUsage)
	postCreateCmd.Flags().StringArrayVar(&postCreateAttach, "attach", nil, attachUsage)
	postCreateCmd.Flags().StringVar(&postCreateVisibility, "visibility", "", visibilityUsage+" (default listed)")
	postCmd.AddCommand(postCreateCmd)

	postUpdateCmd.Flags().StringVarP(&postUpdateTitle, "title", "t", "", "Title, which is also the email's subject")
	postUpdateCmd.Flags().StringVar(&postUpdateSummary, "summary", "", "The writer's own line about the post, shown in place of how it begins")
	registerRichTextFlags(postUpdateCmd, &postUpdateBody, &postUpdateBodyFile, "body", "c", postBodyUsage)
	postUpdateCmd.Flags().StringArrayVar(&postUpdateAttach, "attach", nil, attachUsage)
	postUpdateCmd.Flags().StringVar(&postUpdateVisibility, "visibility", "", visibilityUsage)
	postUpdateCmd.Flags().StringVar(&postUpdatePublishedAt, "published-at", "", "Corrected date of a published post on the web, ISO 8601, now or earlier")
	postCmd.AddCommand(postUpdateCmd)

	postCmd.AddCommand(postDeleteCmd)
}
