package commands

import (
	"github.com/spf13/cobra"
)

var identityCmd = &cobra.Command{
	Annotations: map[string]string{"group": "admin"},
	Use:         "identity",
	Short:       "Show your identity and accessible accounts",
	Long:        "Displays your identity and every account your token reaches, with your user and role on each.",
	RunE:        runIdentityShow,
}

var identityShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show your identity and accessible accounts",
	Long:  "Displays your identity and every account your token reaches, with your user and role on each.",
	RunE:  runIdentityShow,
}

func runIdentityShow(cmd *cobra.Command, args []string) error {
	if err := requireAuth(); err != nil {
		return err
	}
	if err := requireSDK(); err != nil {
		return err
	}

	data, _, err := getSDKClient().Identity().GetMyIdentity(cmd.Context())
	if err != nil {
		return convertSDKError(err)
	}

	breadcrumbs := []Breadcrumb{
		breadcrumb("status", "mailpiece auth status", "Auth status"),
		breadcrumb("doctor", "mailpiece doctor", "Check CLI health"),
	}

	printDetail(normalizeAny(data), "", breadcrumbs)
	return nil
}

func init() {
	rootCmd.AddCommand(identityCmd)
	identityCmd.AddCommand(identityShowCmd)
}
