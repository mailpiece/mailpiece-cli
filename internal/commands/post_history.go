package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// postHistoryCmd is a post's earlier versions.
var postHistoryCmd = &cobra.Command{
	Use:   "history",
	Short: "Read a post's earlier versions",
	Long: "Commands for a post's revisions: its title and text as they stood before an edit. Edits made\n" +
		"within minutes of each other share one revision. Revisions are read only.",
}

var postHistoryColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Created", Field: "created_at"},
	{Header: "Updated", Field: "updated_at"},
	{Header: "By", Field: "creator.name"},
}

var postHistoryListCmd = &cobra.Command{
	Use:   "list POST_ID",
	Short: "List a post's earlier versions",
	Long:  "Lists a post's revisions, oldest first, without their text.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Revisions(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		items := normalizeAny(data)
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", fmt.Sprintf("mailpiece post history show %s REVISION_ID", args[0]), "Read a revision"),
			breadcrumb("latest", fmt.Sprintf("mailpiece post history show %s latest", args[0]), "Read the newest revision"),
		}
		printList(items, postHistoryColumns, fmt.Sprintf("%d revisions", dataCount(items)), breadcrumbs)
		return nil
	},
}

var postHistoryShowCmd = &cobra.Command{
	Use:   "show POST_ID REVISION_ID|latest",
	Short: "Read one earlier version of a post",
	Long: "Shows a revision with its title and text: body as plain text and body_html as it was\n" +
		"written, audience boxes included. 'latest' names the newest; a post never edited has none.",
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewPostsService(getSDK()).Revision(cmd.Context(), args[0], args[1])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("list", "mailpiece post history list "+args[0], "List the post's revisions"),
		}
		printDetail(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

func init() {
	postCmd.AddCommand(postHistoryCmd)
	postHistoryCmd.AddCommand(postHistoryListCmd)
	postHistoryCmd.AddCommand(postHistoryShowCmd)
}
