package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// userCmd is your own user on the account. A user reaches only their own profile.
var userCmd = &cobra.Command{
	Use:         "user",
	Annotations: map[string]string{"group": "admin"},
	Short:       "Show and rename your own user",
	Long: "Commands for your own user on the account. A user reaches only their own profile: anyone else's is out of reach.\n" +
		"`mailpiece identity` shows your user's ID on each account.",
	Example: "$ mailpiece identity --jq '.data.accounts[] | {slug, user: .user.id}'\n$ mailpiece user show USER_ID\n$ mailpiece user update USER_ID --name \"Ana Ruiz\"",
}

var userShowCmd = &cobra.Command{
	Use:   "show USER_ID",
	Short: "Show your user",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewUsersService(getSDK()).Get(cmd.Context(), args[0])
		if err != nil {
			return convertSDKError(err)
		}
		breadcrumbs := []Breadcrumb{
			breadcrumb("update", fmt.Sprintf("mailpiece user update %s --name NAME", args[0]), "Change your name"),
		}
		printDetail(normalizeAny(data), "", breadcrumbs)
		return nil
	},
}

var userUpdateName string

var userUpdateCmd = &cobra.Command{
	Use:   "update USER_ID",
	Short: "Change your name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if userUpdateName == "" {
			return newRequiredFlagError("name")
		}
		if err := requireAuthAndAccount(); err != nil {
			return err
		}
		data, _, err := mailpiece.NewUsersService(getSDK()).Update(cmd.Context(), args[0], &mailpiece.UserRequest{Name: userUpdateName})
		if err != nil {
			return convertSDKError(err)
		}
		printMutation(normalizeAny(data), "", nil)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(userCmd)

	userCmd.AddCommand(userShowCmd)

	userUpdateCmd.Flags().StringVar(&userUpdateName, "name", "", "Your name (required)")
	userCmd.AddCommand(userUpdateCmd)
}
