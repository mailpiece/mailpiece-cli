package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// snippetCmd is the newsletter's snippets: named pieces of text pasted into a post from the
// editor's menu. Admins only.
var snippetCmd = &cobra.Command{
	Use:   "snippet",
	Short: "Manage the newsletter's snippets",
	Long: "Commands for snippets: named pieces of text a newsletter keeps and pastes into its posts. Admins only.\n" +
		"A snippet is pasted as ordinary text, so changing or deleting one never changes a post already written.",
	Example: "$ mailpiece snippet list\n$ mailpiece snippet create --name Signature --content \"Thanks for reading, **Ana**\"\n$ mailpiece snippet update SNIPPET_ID --name \"Sign-off\"",
}

var snippetColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Name", Field: "name"},
	{Header: "Updated", Field: "updated_at"},
}

var snippetListCmd = &cobra.Command{
	Use:   "list",
	Short: "List snippets",
	Long:  "Lists every snippet, by name, each with its content.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		snippets, _, err := getSDK().Snippets().List(cmd.Context())
		if err != nil {
			return convertSDKError(err)
		}
		items := normalizeAny(snippets)
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", "mailpiece snippet show SNIPPET_ID", "View a snippet"),
			breadcrumb("create", "mailpiece snippet create --name NAME --content TEXT", "Add a snippet"),
		}
		printList(items, snippetColumns, fmt.Sprintf("%d snippets", dataCount(items)), breadcrumbs)
		return nil
	},
}

var snippetShowCmd = &cobra.Command{
	Use:   "show SNIPPET_ID",
	Short: "Show a snippet",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := getSDK().Snippets().Get(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("update", fmt.Sprintf("mailpiece snippet update %s --content TEXT", args[0]), "Change the snippet"),
			breadcrumb("delete", fmt.Sprintf("mailpiece snippet delete %s", args[0]), "Delete the snippet"),
		}
		printDetail(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

const snippetContentUsage = "Content, as Markdown or HTML"

var snippetName, snippetContent, snippetContentFile string

// snippetRequestFromFlags reads what was passed, the content turned into HTML. With implicit,
// content no flag gives is read from piped stdin or $EDITOR, which starts from current.
func snippetRequestFromFlags(cmd *cobra.Command, implicit bool, current func() (string, error)) (*mailpiece.SnippetRequest, error) {
	req := &mailpiece.SnippetRequest{Name: changedString(cmd, "name", &snippetName)}
	content, given, err := richTextFromInput(cmd, "content", snippetContent, snippetContentFile, implicit, current)
	if err != nil {
		return nil, err
	}
	if given {
		req.Content = &content
	}
	return req, nil
}

var snippetCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Add a snippet",
	Long:  "Adds a snippet. With no --content or --content-file, the content is read from piped stdin, or written in $EDITOR on a terminal.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if snippetName == "" {
			return newRequiredFlagError("name")
		}
		req, err := snippetRequestFromFlags(cmd, true, nil)
		if err != nil {
			return err
		}
		data, resp, err := getSDK().Snippets().Create(cmd.Context(), req)
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", fmt.Sprintf("mailpiece snippet show %s", data.ID), "View the snippet"),
		}
		if location := resp.Headers.Get("Location"); location != "" {
			printMutationWithLocation(normalizeAny(data), location, breadcrumbs)
		} else {
			printMutation(normalizeAny(data), "", breadcrumbs)
		}
		return nil
	},
}

var snippetUpdateCmd = &cobra.Command{
	Use:   "update SNIPPET_ID",
	Short: "Change a snippet's name or content",
	Long: "Changes only what is passed. A post the snippet was pasted into does not change.\n" +
		"Given no field at all, the new content is read from piped stdin, or edited in $EDITOR on a terminal.",
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		implicit := noFlagChanged(cmd, "name")
		req, err := snippetRequestFromFlags(cmd, implicit, func() (string, error) {
			snippet, _, err := getSDK().Snippets().Get(cmd.Context(), args[0])
			if err != nil {
				return "", convertSDKError(err)
			}
			return snippet.ContentHTML, nil
		})
		if err != nil {
			return err
		}
		if implicit && req.Content == nil {
			return usageWithHint("Nothing to change",
				"cat signature.md | mailpiece snippet update "+args[0]+"  or  mailpiece snippet update "+args[0]+" --content-file signature.md")
		}
		data, _, err := getSDK().Snippets().Update(cmd.Context(), args[0], req)
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", nil)
		return nil
	},
}

var snippetDeleteCmd = &cobra.Command{
	Use:   "delete SNIPPET_ID",
	Short: "Delete a snippet",
	Long:  "Deletes a snippet for good. This cannot be undone. A post it was pasted into keeps the text.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		if _, err := getSDK().Snippets().Delete(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{breadcrumb("list", "mailpiece snippet list", "List remaining snippets")}
		printMutation(map[string]any{"deleted": true, "id": args[0]}, "", breadcrumbs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(snippetCmd)

	snippetCmd.AddCommand(snippetListCmd)
	snippetCmd.AddCommand(snippetShowCmd)

	snippetCreateCmd.Flags().StringVar(&snippetName, "name", "", "Snippet name (required)")
	registerRichTextFlags(snippetCreateCmd, &snippetContent, &snippetContentFile, "content", "c", snippetContentUsage)
	snippetCmd.AddCommand(snippetCreateCmd)

	snippetUpdateCmd.Flags().StringVar(&snippetName, "name", "", "Snippet name")
	registerRichTextFlags(snippetUpdateCmd, &snippetContent, &snippetContentFile, "content", "c", snippetContentUsage)
	snippetCmd.AddCommand(snippetUpdateCmd)

	snippetCmd.AddCommand(snippetDeleteCmd)
}
