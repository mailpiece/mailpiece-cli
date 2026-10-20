package commands

import (
	"strings"

	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <url>",
	Short: "Show any record by its URL",
	Long: `Show any Mailpiece record by the URL of its page in the app, as JSON.

The account comes from the URL, and the record is read through the page's
.json path on the configured API host, whatever host the URL names.

  mailpiece show https://mailpiece.example.com/834206594/posts/<id>`,
	Example: "$ mailpiece show https://mailpiece.example.com/834206594/posts/<id>\n$ mailpiece show https://mailpiece.example.com/my/identity",
	Args:    cobra.ExactArgs(1),
	RunE:    runShow,
}

func init() {
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}
	if err := requireSDK(); err != nil {
		return err
	}
	target, err := parseAppURL(args[0])
	if err != nil {
		return err
	}

	var resp *mailpiece.Response
	if target.Account != "" {
		resp, err = getSDKClient().ForAccount(target.Account).Get(cmd.Context(), target.Path)
	} else {
		resp, err = getSDKClient().Get(cmd.Context(), target.Path)
	}
	if err != nil {
		return convertSDKError(err)
	}

	data := normalizeAny(resp.Data)
	summary := apiSummary(data)
	var breadcrumbs []Breadcrumb
	if target.Account != "" {
		breadcrumbs = append(breadcrumbs, breadcrumb("api", "mailpiece api get "+strings.TrimPrefix(target.Path, "/")+" --profile "+target.Account, "The same read through the raw API"))
	}
	if list, ok := data.([]map[string]any); ok {
		printList(list, inferColumns(list), summary, breadcrumbs)
		return nil
	}
	printDetail(data, summary, breadcrumbs)
	return nil
}
