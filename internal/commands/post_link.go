package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// postLinkCmd is the links a post's issue was mailed with.
var postLinkCmd = &cobra.Command{
	Use:   "link",
	Short: "See and correct the links a post was mailed with",
	Long: "Commands for the links of an issue already mailed. The email cannot change, so each of its\n" +
		"links passes through the newsletter, and where one sends a reader can still be corrected.",
}

var postLinkColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Text", Field: "text"},
	{Header: "Mailed URL", Field: "original_url"},
	{Header: "URL", Field: "url"},
	{Header: "Stale", Field: "stale"},
}

var postLinkListCmd = &cobra.Command{
	Use:   "list POST_ID",
	Short: "List the links a post was mailed with",
	Long: "Lists the links of a post's mailed issue: original_url is what the email has and url where\n" +
		"it sends a reader now. A stale link is one the post's text no longer has and that was never\n" +
		"corrected. A post never mailed has none.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := getSDK().Posts().Links(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		items := normalizeAny(data)
		breadcrumbs := []Breadcrumb{
			breadcrumb("update", fmt.Sprintf("mailpiece post link update %s LINK_ID --url URL", args[0]), "Correct where a link goes"),
		}
		printList(items, postLinkColumns, fmt.Sprintf("%d links", dataCount(items)), breadcrumbs)
		return nil
	},
}

var postLinkUpdateURL string

var postLinkUpdateCmd = &cobra.Command{
	Use:   "update POST_ID LINK_ID",
	Short: "Correct where a mailed link sends a reader",
	Long: "Changes the address a mailed link sends a reader to from now on. The email already sent\n" +
		"keeps its text. The address must be a web address (http or https).",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if postLinkUpdateURL == "" {
			return newRequiredFlagError("url")
		}
		req := &mailpiece.UpdatePostLinkRequest{URL: postLinkUpdateURL}
		data, _, err := getSDK().Posts().UpdateLink(cmd.Context(), args[0], args[1], req)
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("list", "mailpiece post link list "+args[0], "List the post's links"),
		}
		printMutation(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

func init() {
	postCmd.AddCommand(postLinkCmd)
	postLinkCmd.AddCommand(postLinkListCmd)

	postLinkUpdateCmd.Flags().StringVar(&postLinkUpdateURL, "url", "", "Where the link sends a reader from now on (required)")
	postLinkCmd.AddCommand(postLinkUpdateCmd)
}
