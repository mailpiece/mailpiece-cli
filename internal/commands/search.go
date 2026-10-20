package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/internal/errors"
)

var searchCmd = &cobra.Command{
	Use:   "search QUERY...",
	Short: "Search the newsletter's posts and subscribers",
	Long: "Searches the newsletter and answers with the posts and the subscribers the query matches,\n" +
		"newest first. The app pages the subscribers, and every page of them is fetched, so both\n" +
		"lists are whole. The query is sent as a single string. Admins only.",
	Example: "$ mailpiece search welcome\n$ mailpiece search ana@example.com",
	Args:    cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		query := strings.TrimSpace(strings.Join(args, " "))
		if query == "" {
			return errors.NewInvalidArgsError("A search needs a query")
		}

		results, _, err := getSDK().Search().Search(cmd.Context(), query)
		if err != nil {
			return convertSDKError(err)
		}

		summary := fmt.Sprintf("%d posts and %d subscribers for %q", len(results.Posts), len(results.Subscribers), query)
		breadcrumbs := []Breadcrumb{
			breadcrumb("show", "mailpiece post show POST_ID", "View a post"),
		}
		printDetail(normalizeAny(results), summary, breadcrumbs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
