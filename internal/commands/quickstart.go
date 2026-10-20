package commands

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

type quickStartResponse struct {
	Version  string                 `json:"version"`
	Auth     quickStartAuthInfo     `json:"auth"`
	Commands quickStartCommandsInfo `json:"commands"`
}

type quickStartAuthInfo struct {
	Status  string `json:"status"`
	Profile string `json:"profile,omitempty"`
	Account string `json:"account,omitempty"`
}

type quickStartCommandsInfo struct {
	QuickStart []string `json:"quick_start"`
	Common     []string `json:"common"`
}

func runRootDefault(cmd *cobra.Command, args []string) error {
	if isHumanOutput() {
		return cmd.Help()
	}

	auth := quickStartAuthInfo{Status: "unauthenticated"}
	if profileName := firstNonEmpty(activeProfile, cfgProfile); profileName != "" {
		auth.Profile = profileName
	}
	if cfg != nil {
		if cfg.Account != "" {
			auth.Account = cfg.Account
		}
		if cfg.Token != "" {
			auth.Status = "authenticated"
		}
	}

	resp := quickStartResponse{
		Version: currentVersion(),
		Auth:    auth,
		Commands: quickStartCommandsInfo{
			QuickStart: []string{"mailpiece doctor", "mailpiece config show", "mailpiece post list", "mailpiece subscriber list"},
			Common:     []string{"mailpiece auth status", "mailpiece config explain", "mailpiece doctor", "mailpiece post list", "mailpiece subscriber list"},
		},
	}

	summary := fmt.Sprintf("mailpiece %s: not logged in", currentVersion())
	if auth.Status == "authenticated" {
		summary = fmt.Sprintf("mailpiece %s: logged in", currentVersion())
		if auth.Account != "" {
			summary += " @ " + auth.Account
		}
	}
	if auth.Profile != "" {
		summary += fmt.Sprintf(" (profile: %s)", auth.Profile)
	}

	breadcrumbs := []Breadcrumb{
		breadcrumb("doctor", "mailpiece doctor", "Check CLI health"),
		breadcrumb("config", "mailpiece config show", "Show the effective config"),
		breadcrumb("list_posts", "mailpiece post list", "List the drafts and the scheduled posts"),
		breadcrumb("list_subscribers", "mailpiece subscriber list", "List the subscribers"),
	}
	if auth.Status == "unauthenticated" {
		breadcrumbs = append(breadcrumbs,
			breadcrumb("authenticate", authLoginHint(), "Authenticate with a token"),
			breadcrumb("authenticate_email", "mailpiece auth email", "Sign in by email"),
		)
	}

	printSuccessWithBreadcrumbs(resp, summary, breadcrumbs)
	return nil
}

func authLoginHint() string {
	parts := []string{"mailpiece", "auth", "login", "<token>"}
	if strings.TrimSpace(cfgProfile) != "" {
		parts = append(parts, "--profile", cfgProfile)
	}
	return strings.Join(parts, " ")
}
