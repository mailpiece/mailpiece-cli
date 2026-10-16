package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	cfgpkg "github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

type configExplainCandidate struct {
	Source   string `json:"source"`
	Value    string `json:"value,omitempty"`
	Selected bool   `json:"selected,omitempty"`
}

type configExplainField struct {
	Value      any                      `json:"value,omitempty"`
	Source     string                   `json:"source"`
	Configured *bool                    `json:"configured,omitempty"`
	Candidates []configExplainCandidate `json:"candidates"`
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Inspect resolved configuration",
	Long:  "Inspect effective configuration values and explain why specific values win.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show effective configuration",
	Long: `Show the currently effective configuration after applying precedence rules
(flags, environment variables, profile settings, local config, and global config).`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data := configShowData()
		breadcrumbs := []Breadcrumb{
			breadcrumb("doctor", "mailpiece doctor", "Run a full health check"),
			breadcrumb("explain", "mailpiece config explain", "Explain configuration precedence"),
			breadcrumb("profiles", "mailpiece auth list", "List saved profiles"),
		}

		switch out.EffectiveFormat() {
		case output.FormatStyled:
			writeOutputString(renderConfigShowHuman(data, false))
			captureResponse()
			return nil
		case output.FormatMarkdown:
			writeOutputString(renderConfigShowHuman(data, true))
			captureResponse()
			return nil
		default:
			printDetail(data, "Effective configuration", breadcrumbs)
			return nil
		}
	},
}

var configExplainCmd = &cobra.Command{
	Use:   "explain",
	Short: "Explain configuration precedence",
	Long: `Explain why each effective configuration value won, listing every candidate
from flags, environment variables, profile settings, local config, and global config.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		data := configExplainData()
		breadcrumbs := []Breadcrumb{
			breadcrumb("doctor", "mailpiece doctor", "Run a full health check"),
			breadcrumb("show", "mailpiece config show", "Show the effective config only"),
			breadcrumb("profiles", "mailpiece auth list", "List saved profiles"),
		}

		switch out.EffectiveFormat() {
		case output.FormatStyled:
			writeOutputString(renderConfigExplainHuman(data, false))
			captureResponse()
			return nil
		case output.FormatMarkdown:
			writeOutputString(renderConfigExplainHuman(data, true))
			captureResponse()
			return nil
		default:
			recordOutputError(out.OK(data,
				output.WithSummary("Configuration precedence"),
				output.WithBreadcrumbs(breadcrumbs...),
			))
			captureResponse()
			return nil
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configExplainCmd)
}

func configShowData() map[string]any {
	eff := resolveDoctorEffectiveConfig()
	data := map[string]any{
		"api_url":        eff.APIURL,
		"account":        emptyToNil(eff.Account),
		"account_source": emptyToNil(accountSource),
		"profile":        emptyToNil(eff.ProfileName),
		"token":          map[string]any{"configured": eff.Token != "", "source": eff.TokenSource},
		"global_config":  globalConfigPathForDoctor(),
		"local_config":   emptyToNil(cfgpkg.LocalConfigPath()),
		"profiles":       savedProfileNames(),
	}
	if configuredAPIURL != "" && configuredAPIURL != eff.APIURL {
		data["api_url_configured"] = configuredAPIURL
	}
	if eff.Account == "" && eff.Token != "" {
		data["account_note"] = "resolved from the token's only account when a command needs one"
	}
	return data
}

func configExplainData() map[string]any {
	eff := resolveDoctorEffectiveConfig()
	globalCfg := loadConfigFileOrEmpty(globalConfigPathForDoctor())
	localCfg := loadConfigFileOrEmpty(cfgpkg.LocalConfigPath())
	_, defaultProfile := profileStoreInfo()
	profileAccount, profileURL := "", ""
	if activeProfile != "" && profiles != nil {
		if p, err := profiles.Get(activeProfile); err == nil && p != nil {
			if binding, err := resolveProfileAccountBinding(activeProfile, p); err == nil {
				profileAccount = binding.Account
			}
			profileURL = p.BaseURL
		}
	}

	// With no saved profile, the selected profile name is the account.
	selectedAccount := func(name string) string {
		if activeProfile == "" || len(savedProfileNames()) > 0 {
			return ""
		}
		return mailpiece.NormalizeAccountSlug(name)
	}

	configured := eff.Token != ""
	tokenField := configExplainField{
		Source:     displayTokenSource(eff.TokenSource),
		Configured: &configured,
		Candidates: presentCandidates([]configExplainCandidate{
			{Source: "flag --token", Value: configuredString(cfgToken != "", "configured via flag"), Selected: eff.TokenSourceRaw == "flag"},
			{Source: "env MAILPIECE_TOKEN", Value: configuredString(cfgpkg.Env("MAILPIECE_TOKEN") != "", "configured in environment"), Selected: eff.TokenSourceRaw == "env"},
			{Source: profileTokenSourceLabel(activeProfile), Value: configuredString(storedProfileToken() != "", profileCredentialValue()), Selected: eff.TokenSourceRaw == "keyring" || eff.TokenSourceRaw == "fallback-file"},
			{Source: "local config", Value: configuredString(localCfg.Token != "", "configured in local config"), Selected: eff.TokenSourceRaw == "local-config"},
			{Source: "global config", Value: configuredString(globalCfg.Token != "", "configured in global config"), Selected: eff.TokenSourceRaw == "global-config"},
		}),
	}

	// The address is chosen as it was configured; its account, if it carries one,
	// is the last place an account is looked for.
	_, addressAccount := splitAccountAddress(configuredAPIURL)
	apiURLField := explainField(firstNonEmpty(configuredAPIURL, eff.APIURL), []configExplainCandidate{
		{Source: "flag --api-url", Value: cfgAPIURL},
		{Source: "env MAILPIECE_API_URL", Value: cfgpkg.Env("MAILPIECE_API_URL")},
		{Source: profileSourceLabel(activeProfile), Value: profileURL},
		{Source: "local config", Value: localCfg.APIURL},
		{Source: "global config", Value: globalCfg.APIURL},
	})
	apiURLField.Value = emptyToNil(eff.APIURL)

	return map[string]any{
		"api_url": apiURLField,
		"account": explainField(eff.Account, []configExplainCandidate{
			{Source: "flag --account", Value: cfgAccount},
			{Source: "env MAILPIECE_ACCOUNT", Value: cfgpkg.Env("MAILPIECE_ACCOUNT")},
			{Source: profileSourceLabel(activeProfile), Value: profileAccount},
			{Source: "flag --profile", Value: selectedAccount(cfgProfile)},
			{Source: "env MAILPIECE_PROFILE", Value: selectedAccount(cfgpkg.Env("MAILPIECE_PROFILE"))},
			{Source: "local config", Value: localCfg.Account},
			{Source: "global config", Value: globalCfg.Account},
			{Source: "API URL " + configuredAPIURL, Value: addressAccount},
		}),
		"profile": explainField(eff.ProfileName, []configExplainCandidate{
			{Source: "flag --profile", Value: cfgProfile},
			{Source: "env MAILPIECE_PROFILE", Value: cfgpkg.Env("MAILPIECE_PROFILE")},
			{Source: "default profile", Value: defaultProfile},
		}),
		"token":          tokenField,
		"saved_profiles": savedProfileNames(),
	}
}

func explainField(effective string, candidates []configExplainCandidate) configExplainField {
	field := configExplainField{Value: emptyToNil(effective), Source: "not configured"}
	present := make([]configExplainCandidate, 0, len(candidates))
	selected := false
	for _, c := range candidates {
		if c.Value == "" {
			continue
		}
		if !selected && c.Value == effective {
			c.Selected = true
			selected = true
			field.Source = c.Source
		}
		present = append(present, c)
	}
	field.Candidates = present
	return field
}

func presentCandidates(candidates []configExplainCandidate) []configExplainCandidate {
	present := make([]configExplainCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Value != "" {
			present = append(present, c)
		}
	}
	return present
}

func renderConfigShowHuman(data map[string]any, markdown bool) string {
	var sb strings.Builder
	if markdown {
		sb.WriteString("# Mailpiece Config\n\n")
	} else {
		sb.WriteString("Mailpiece Config\n\n")
	}

	profileName := describeConfigShowValue(data["profile"])
	account := describeConfigShowValue(data["account"])
	if source, ok := data["account_source"].(string); ok && source != "" && account != "<unset>" {
		account += " (" + source + ")"
	}
	if note, ok := data["account_note"].(string); ok {
		account += ", " + note
	}
	apiURL := describeConfigShowValue(data["api_url"])
	if configured, ok := data["api_url_configured"].(string); ok {
		apiURL += " (from " + configured + ")"
	}
	token := describeConfigShowToken(data["token"])
	globalConfig := describeConfigShowValue(data["global_config"])
	localConfig := describeConfigShowValue(data["local_config"])
	savedProfiles := describeConfigShowProfiles(data["profiles"])

	if markdown {
		fmt.Fprintf(&sb, "- **Profile:** `%s`\n", profileName)
		fmt.Fprintf(&sb, "- **Account:** `%s`\n", account)
		fmt.Fprintf(&sb, "- **API URL:** `%s`\n", apiURL)
		fmt.Fprintf(&sb, "- **Token:** %s\n", token)
		fmt.Fprintf(&sb, "- **Global Config:** `%s`\n", globalConfig)
		fmt.Fprintf(&sb, "- **Local Config:** `%s`\n", localConfig)
		if savedProfiles != "" {
			fmt.Fprintf(&sb, "- **Saved Profiles:** %s\n", savedProfiles)
		}
		sb.WriteString("\n## Next steps\n")
		sb.WriteString("- `mailpiece config explain`: explain precedence\n")
		sb.WriteString("- `mailpiece doctor`: run a full health check\n")
		sb.WriteString("- `mailpiece auth list`: inspect saved profiles\n")
	} else {
		fmt.Fprintf(&sb, "Profile        %s\n", profileName)
		fmt.Fprintf(&sb, "Account        %s\n", account)
		fmt.Fprintf(&sb, "API URL        %s\n", apiURL)
		fmt.Fprintf(&sb, "Token          %s\n", token)
		fmt.Fprintf(&sb, "Global Config  %s\n", globalConfig)
		fmt.Fprintf(&sb, "Local Config   %s\n", localConfig)
		if savedProfiles != "" {
			fmt.Fprintf(&sb, "Saved Profiles %s\n", savedProfiles)
		}
		sb.WriteString("\nNext steps\n")
		sb.WriteString("  mailpiece config explain  # explain precedence\n")
		sb.WriteString("  mailpiece doctor          # run a full health check\n")
		sb.WriteString("  mailpiece auth list       # inspect saved profiles\n")
	}

	return sb.String()
}

func renderConfigExplainHuman(data map[string]any, markdown bool) string {
	var sb strings.Builder
	if markdown {
		sb.WriteString("# Mailpiece Config Explain\n\n")
	} else {
		sb.WriteString("Mailpiece Config Explain\n\n")
	}

	order := []struct {
		key   string
		label string
	}{
		{"profile", "Profile"},
		{"account", "Account"},
		{"api_url", "API URL"},
		{"token", "Token"},
	}

	for _, item := range order {
		field, ok := data[item.key].(configExplainField)
		if !ok {
			continue
		}
		if markdown {
			fmt.Fprintf(&sb, "## %s\n", item.label)
		} else {
			fmt.Fprintf(&sb, "%s\n", item.label)
		}

		value := explainFieldValue(field)
		if markdown {
			fmt.Fprintf(&sb, "- Effective: `%s`\n", value)
			fmt.Fprintf(&sb, "- Source: `%s`\n", field.Source)
			sb.WriteString("- Candidates:\n")
			for _, candidate := range field.Candidates {
				marker := ""
				if candidate.Selected {
					marker = " **(selected)**"
				}
				fmt.Fprintf(&sb, "  - `%s`: `%s`%s\n", candidate.Source, candidate.Value, marker)
			}
			sb.WriteString("\n")
		} else {
			fmt.Fprintf(&sb, "  Effective: %s\n", value)
			fmt.Fprintf(&sb, "  Source:    %s\n", field.Source)
			sb.WriteString("  Candidates:\n")
			for _, candidate := range field.Candidates {
				marker := ""
				if candidate.Selected {
					marker = "  ✓"
				}
				fmt.Fprintf(&sb, "    - %-24s %s%s\n", candidate.Source+":", candidate.Value, marker)
			}
			sb.WriteString("\n")
		}
	}

	if savedProfiles, ok := data["saved_profiles"].([]string); ok && len(savedProfiles) > 0 {
		if markdown {
			sb.WriteString("## Saved Profiles\n")
			for _, name := range savedProfiles {
				fmt.Fprintf(&sb, "- `%s`\n", name)
			}
		} else {
			sb.WriteString("Saved Profiles\n")
			for _, name := range savedProfiles {
				fmt.Fprintf(&sb, "  - %s\n", name)
			}
		}
		sb.WriteString("\n")
	}

	if markdown {
		sb.WriteString("## Next steps\n")
		sb.WriteString("- `mailpiece config show`: show the effective config only\n")
		sb.WriteString("- `mailpiece doctor`: run a full health check\n")
		sb.WriteString("- `mailpiece auth list`: inspect saved profiles\n")
	} else {
		sb.WriteString("Next steps\n")
		sb.WriteString("  mailpiece config show   # show the effective config only\n")
		sb.WriteString("  mailpiece doctor        # run a full health check\n")
		sb.WriteString("  mailpiece auth list     # inspect saved profiles\n")
	}

	return sb.String()
}

func describeConfigShowValue(v any) string {
	switch value := v.(type) {
	case nil:
		return "<unset>"
	case string:
		if strings.TrimSpace(value) == "" {
			return "<unset>"
		}
		return value
	default:
		return fmt.Sprintf("%v", value)
	}
}

func describeConfigShowToken(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	configured, _ := m["configured"].(bool)
	source, _ := m["source"].(string)
	if !configured {
		return "not configured"
	}
	if strings.TrimSpace(source) == "" {
		return "configured"
	}
	return fmt.Sprintf("configured (%s)", source)
}

func describeConfigShowProfiles(v any) string {
	if items, ok := v.([]string); ok {
		return strings.Join(items, ", ")
	}
	return ""
}

func explainFieldValue(field configExplainField) string {
	if field.Configured != nil {
		if *field.Configured {
			return "configured"
		}
		return "not configured"
	}
	if s, ok := field.Value.(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	return "<unset>"
}

func configuredString(ok bool, label string) string {
	if !ok {
		return ""
	}
	return label
}

func profileSourceLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "profile store"
	}
	return fmt.Sprintf("profile %s", name)
}

func profileTokenSourceLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "profile credential"
	}
	return fmt.Sprintf("profile credential %s", name)
}

func profileCredentialValue() string {
	if creds != nil && creds.UsingKeyring() {
		return "configured in system keyring"
	}
	return "configured in fallback credential file"
}

func displayTokenSource(source string) string {
	if strings.TrimSpace(source) == "" {
		return "not configured"
	}
	return source
}

func storedProfileToken() string {
	if creds == nil || activeProfile == "" {
		return ""
	}
	t, _ := credsLoadProfileToken(activeProfile)
	return t
}

func loadConfigFileOrEmpty(path string) *cfgpkg.Config {
	if c, err := loadDoctorConfigFile(path); err == nil {
		return c
	}
	return &cfgpkg.Config{}
}

func savedProfileNames() []string {
	allProfiles, defaultName := profileStoreInfo()
	names := make([]string, 0, len(allProfiles))
	for name := range allProfiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if name == defaultName {
			names[i] = name + " (default)"
		}
	}
	return names
}

func profileStoreInfo() (map[string]*profile.Profile, string) {
	if profiles == nil {
		return nil, ""
	}
	allProfiles, defaultName, err := profiles.List()
	if err != nil {
		return nil, ""
	}
	return allProfiles, defaultName
}

func emptyToNil(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}
