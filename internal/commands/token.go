package commands

import (
	"fmt"

	"github.com/mailpiece/mailpiece-cli/internal/render"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage personal access tokens",
	Long:  "Commands for managing your personal access tokens.",
}

var tokenColumns = render.Columns{
	{Header: "ID", Field: "id"},
	{Header: "Description", Field: "description"},
	{Header: "Permission", Field: "permission"},
	{Header: "Created", Field: "created_at"},
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List personal access tokens",
	Long:  "Lists your personal access tokens, newest first.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		if err := requireSDK(); err != nil {
			return err
		}

		tokens, _, err := getSDKClient().AccessTokens().List(cmd.Context())
		if err != nil {
			return convertSDKError(err)
		}

		items := normalizeAny(tokens)

		count := dataCount(items)
		summary := fmt.Sprintf("%d access tokens", count)

		breadcrumbs := []Breadcrumb{
			breadcrumb("create", "mailpiece token create --description DESCRIPTION --permission read|write", "Create a token"),
			breadcrumb("delete", "mailpiece token delete TOKEN_ID", "Delete a token"),
		}

		printList(items, tokenColumns, summary, breadcrumbs)
		return nil
	},
}

var (
	tokenCreateDescription string
	tokenCreatePermission  string
)

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a personal access token",
	Long:  "Creates a new personal access token. The token value is shown once at creation and cannot be retrieved later.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		if err := requireSDK(); err != nil {
			return err
		}

		if tokenCreateDescription == "" {
			return newRequiredFlagError("description")
		}
		if tokenCreatePermission == "" {
			return newRequiredFlagError("permission")
		}
		if err := checkFlagValue("permission", tokenCreatePermission, []string{"read", "write"}); err != nil {
			return err
		}

		req := &mailpiece.CreateAccessTokenRequest{
			Description: tokenCreateDescription,
			Permission:  tokenCreatePermission,
		}
		token, _, err := getSDKClient().AccessTokens().Create(cmd.Context(), req)
		if err != nil {
			return convertSDKError(err)
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("list", "mailpiece token list", "List tokens"),
		}
		if token.ID != "" {
			breadcrumbs = append(breadcrumbs, breadcrumb("delete", fmt.Sprintf("mailpiece token delete %s", token.ID), "Delete this token"))
		}

		printMutation(normalizeAny(token), "Save the token now: it will not be shown again.", breadcrumbs)
		return nil
	},
}

var tokenDeleteCmd = &cobra.Command{
	Use:   "delete TOKEN_ID",
	Short: "Delete a personal access token",
	Long:  "Deletes a personal access token by ID.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuth(); err != nil {
			return err
		}
		if err := requireSDK(); err != nil {
			return err
		}

		if _, err := getSDKClient().AccessTokens().Delete(cmd.Context(), args[0]); err != nil {
			return convertSDKError(err)
		}

		breadcrumbs := []Breadcrumb{
			breadcrumb("list", "mailpiece token list", "List remaining tokens"),
		}

		printMutation(map[string]any{"deleted": true, "id": args[0]}, "", breadcrumbs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tokenCmd)

	tokenCmd.AddCommand(tokenListCmd)

	tokenCreateCmd.Flags().StringVar(&tokenCreateDescription, "description", "", "Token description (required)")
	tokenCreateCmd.Flags().StringVar(&tokenCreatePermission, "permission", "", "Token permission: read or write (required)")
	tokenCmd.AddCommand(tokenCreateCmd)

	tokenCmd.AddCommand(tokenDeleteCmd)
}
