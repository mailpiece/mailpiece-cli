package commands

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var cliUXConfigured bool

type helpLink struct {
	Command     string
	Description string
}

type helpCommandGroup struct {
	Title    string
	Commands []*cobra.Command
}

func configureCLIUX() {
	if cliUXConfigured {
		return
	}
	cliUXConfigured = true

	applyHelpMetadata(rootCmd)
	installHumanHelp()
}

func applyHelpMetadata(root *cobra.Command) {
	setRootHelpMetadata(root)
	walkCommandTree(root, func(cmd *cobra.Command) {
		applyGenericAliases(cmd)
		if cmd.Example == "" {
			if ex := commandExamples[cmd.CommandPath()]; ex != "" {
				cmd.Example = ex
			}
		}
	})
}

func installHumanHelp() {
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		// executeRoot turns this into a usage error; help here would mix in.
		if isBareGroupWithUnknownArg(cmd) {
			return
		}
		if cfgAgent {
			agentHelp(cmd, args)
			return
		}
		renderHelp(cmd, outWriter)
	})
}

func renderHelp(cmd *cobra.Command, w io.Writer) {
	if w == nil {
		w = outWriter
	}
	if w == nil {
		w = io.Discard
	}

	if cmd == rootCmd {
		renderRootHelp(cmd, w)
		return
	}
	renderCommandHelp(cmd, w)
}

func renderRootHelp(cmd *cobra.Command, w io.Writer) {
	cmd.InitDefaultHelpFlag()
	cmd.InitDefaultVersionFlag()

	fmt.Fprintln(w, strings.TrimSpace(cmd.Long))
	fmt.Fprintln(w)
	fmt.Fprintln(w, "USAGE")
	fmt.Fprintf(w, "  %s <command> [flags]\n", cmd.Name())

	width := commandNameWidth(visibleSubcommands(cmd))
	for _, group := range groupedRootCommands(cmd) {
		fmt.Fprintln(w)
		fmt.Fprintln(w, group.Title)
		printCommandList(w, group.Commands, width)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "FLAGS")
	printNamedFlags(w, cmd.PersistentFlags(), []string{"json", "jq", "quiet", "account", "profile", "verbose"})
	if flags := rootLocalFlags(cmd); len(flags) > 0 {
		printFlags(w, flags)
	}

	if cmd.Example != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "EXAMPLES")
		printExampleBlock(w, cmd.Example)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "LEARN MORE")
	fmt.Fprintln(w, "  Use `mailpiece commands` to see the full command catalog.")
	fmt.Fprintln(w, "  Use `mailpiece <command> --help` for more information about a command.")
	fmt.Fprintln(w, "  Use `mailpiece commands --json` for a structured command catalog.")
}

func renderCommandHelp(cmd *cobra.Command, w io.Writer) {
	cmd.InitDefaultHelpFlag()

	desc := strings.TrimSpace(cmd.Long)
	if desc == "" {
		desc = strings.TrimSpace(cmd.Short)
	}
	if desc != "" {
		fmt.Fprintln(w, desc)
		fmt.Fprintln(w)
	}

	subs := visibleSubcommands(cmd)
	usageLine := cmd.UseLine()
	if len(subs) > 0 && !cmd.Runnable() {
		usageLine = cmd.CommandPath() + " <command> [flags]"
	}

	fmt.Fprintln(w, "USAGE")
	fmt.Fprintf(w, "  %s\n", usageLine)

	if len(cmd.Aliases) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "ALIASES")
		fmt.Fprintf(w, "  %s\n", strings.Join(cmd.Aliases, ", "))
	}

	if len(subs) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "AVAILABLE COMMANDS")
		printCommandList(w, subs, commandNameWidth(subs))
	}

	if flags := visibleFlags(cmd.LocalFlags()); len(flags) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "FLAGS")
		printFlags(w, flags)
	}

	if cmd.HasInheritedFlags() {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "GLOBAL OUTPUT FLAGS")
		printNamedFlags(w, cmd.InheritedFlags(), []string{"json", "quiet", "styled", "markdown", "ids-only", "count", "limit"})

		fmt.Fprintln(w)
		fmt.Fprintln(w, "GLOBAL CONFIG FLAGS")
		printNamedFlags(w, cmd.InheritedFlags(), []string{"account", "profile", "token", "api-url", "verbose", "agent"})
	}

	if cmd.Example != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "EXAMPLES")
		printExampleBlock(w, cmd.Example)
	}

	if links := relatedCommands[cmd.CommandPath()]; len(links) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "SEE ALSO")
		width := 34
		for _, link := range links {
			width = max(width, len(link.Command))
		}
		for _, link := range links {
			fmt.Fprintf(w, "  %-*s %s\n", width, link.Command, link.Description)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "LEARN MORE")
	fmt.Fprintf(w, "  Use `mailpiece %s --help` for more information about this command.\n", strings.TrimPrefix(cmd.CommandPath(), "mailpiece "))
}

func setRootHelpMetadata(cmd *cobra.Command) {
	cmd.Example = strings.Join([]string{
		"$ mailpiece auth status",
		"$ mailpiece doctor",
		"$ mailpiece identity",
	}, "\n")
}

func applyGenericAliases(cmd *cobra.Command) {
	switch cmd.Name() {
	case "list":
		appendAlias(cmd, "ls")
	case "show":
		appendAlias(cmd, "view")
	case "delete":
		appendAlias(cmd, "rm")
	}
}

func appendAlias(cmd *cobra.Command, alias string) {
	for _, existing := range cmd.Aliases {
		if existing == alias {
			return
		}
	}
	cmd.Aliases = append(cmd.Aliases, alias)
}

func walkCommandTree(cmd *cobra.Command, fn func(*cobra.Command)) {
	fn(cmd)
	for _, sub := range cmd.Commands() {
		if sub.Hidden {
			continue
		}
		walkCommandTree(sub, fn)
	}
}

// groupedRootCommands groups the top-level commands as `mailpiece commands`
// does, by the group each one sets for itself.
func groupedRootCommands(cmd *cobra.Command) []helpCommandGroup {
	groups := make([]helpCommandGroup, 0, len(commandCatalogOrder))
	for _, key := range commandCatalogOrder {
		var commands []*cobra.Command
		for _, sub := range visibleSubcommands(cmd) {
			if commandGroup(sub) == key {
				commands = append(commands, sub)
			}
		}
		if len(commands) == 0 {
			continue
		}
		groups = append(groups, helpCommandGroup{Title: commandCatalogTitles[key], Commands: commands})
	}
	return groups
}

func visibleSubcommands(cmd *cobra.Command) []*cobra.Command {
	var subs []*cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		subs = append(subs, sub)
	}
	sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
	return subs
}

func commandNameWidth(commands []*cobra.Command) int {
	width := 14
	for _, sub := range commands {
		if len(sub.Name()) > width {
			width = len(sub.Name())
		}
	}
	return width
}

func printCommandList(w io.Writer, commands []*cobra.Command, width int) {
	for _, sub := range commands {
		fmt.Fprintf(w, "  %-*s %s\n", width, sub.Name(), sub.Short)
	}
}

func printNamedFlags(w io.Writer, flags *pflag.FlagSet, names []string) {
	selected := make([]*pflag.Flag, 0, len(names))
	for _, name := range names {
		if f := flags.Lookup(name); f != nil && !f.Hidden {
			selected = append(selected, f)
		}
	}
	printFlags(w, selected)
}

func rootLocalFlags(cmd *cobra.Command) []*pflag.Flag {
	excluded := map[string]bool{
		"account": true, "agent": true, "api-url": true, "count": true,
		"ids-only": true, "jq": true, "json": true, "limit": true,
		"markdown": true, "profile": true, "quiet": true, "styled": true,
		"token": true, "verbose": true,
	}

	flags := visibleFlags(cmd.Flags())
	result := make([]*pflag.Flag, 0, len(flags))
	for _, f := range flags {
		if excluded[f.Name] {
			continue
		}
		result = append(result, f)
	}
	return result
}

func visibleFlags(flags *pflag.FlagSet) []*pflag.Flag {
	var result []*pflag.Flag
	flags.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		result = append(result, f)
	})
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func printFlags(w io.Writer, flags []*pflag.Flag) {
	for _, f := range flags {
		fmt.Fprintf(w, "  %-26s %s\n", flagDisplayName(f), flagUsage(f))
	}
}

func flagDisplayName(f *pflag.Flag) string {
	parts := make([]string, 0, 2)
	if f.Shorthand != "" {
		parts = append(parts, "-"+f.Shorthand)
	}
	name := "--" + f.Name
	if f.Value.Type() != "bool" {
		name += " <" + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_")) + ">"
	}
	parts = append(parts, name)
	return strings.Join(parts, ", ")
}

func flagUsage(f *pflag.Flag) string {
	usage := f.Usage
	if shouldShowDefault(f) {
		usage += fmt.Sprintf(" (default: %s)", f.DefValue)
	}
	return usage
}

func shouldShowDefault(f *pflag.Flag) bool {
	if f.DefValue == "" {
		return false
	}
	switch f.Value.Type() {
	case "bool":
		return f.DefValue == "true"
	case "int":
		return f.DefValue != "0"
	case "stringSlice", "stringArray":
		return f.DefValue != "[]"
	default:
		return true
	}
}

func printExampleBlock(w io.Writer, example string) {
	style := lipgloss.NewStyle()
	if f, ok := w.(*os.File); ok {
		if isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd()) {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
		}
	}
	for _, line := range strings.Split(strings.TrimSpace(example), "\n") {
		fmt.Fprintf(w, "%s\n", style.Render("  "+line))
	}
}

var commandExamples = map[string]string{ //nolint:gosec // G101: keyed by command path; "mailpiece token" names a command, not a credential
	"mailpiece auth":             "$ mailpiece auth status\n$ ... | mailpiece auth login --with-token --account 834206594\n$ mailpiece auth email start --email you@example.com",
	"mailpiece auth status":      "$ mailpiece auth status",
	"mailpiece auth list":        "$ mailpiece auth list\n$ mailpiece auth switch 834206594",
	"mailpiece auth switch":      "$ mailpiece auth switch 834206594",
	"mailpiece auth logout":      "$ mailpiece auth logout\n$ mailpiece auth logout --profile 834206594",
	"mailpiece token":            "$ mailpiece token list\n$ mailpiece token create --description \"Laptop\" --permission read\n$ mailpiece token delete <token_id>",
	"mailpiece token list":       "$ mailpiece token list",
	"mailpiece token create":     "$ mailpiece token create --description \"Laptop\" --permission read",
	"mailpiece token delete":     "$ mailpiece token delete <token_id>",
	"mailpiece identity show":    "$ mailpiece identity show",
	"mailpiece config":           "$ mailpiece config show\n$ mailpiece config explain",
	"mailpiece config show":      "$ mailpiece config show",
	"mailpiece config explain":   "$ mailpiece config explain\n$ mailpiece config explain --profile 834206594",
	"mailpiece doctor":           "$ mailpiece doctor\n$ mailpiece doctor --profile 834206594\n$ mailpiece doctor --all-profiles\n$ mailpiece doctor --verbose\n$ mailpiece doctor --json",
	"mailpiece setup":            "$ mailpiece setup\n$ mailpiece setup claude",
	"mailpiece mcp":              "$ mailpiece mcp\n$ mailpiece mcp --writes --domains posts,subscribers",
	"mailpiece skill":            "$ mailpiece skill\n$ mailpiece skill install",
	"mailpiece skill install":    "$ mailpiece skill install",
	"mailpiece auth email start": "$ mailpiece auth email start --email you@example.com",
}

var relatedCommands = map[string][]helpLink{
	"mailpiece auth": {
		{Command: "mailpiece identity show", Description: "View your identity and accessible accounts"},
		{Command: "mailpiece doctor", Description: "Run a full health check after logging in"},
	},
	"mailpiece auth status": {
		{Command: "mailpiece doctor", Description: "Run a full CLI health check"},
		{Command: "mailpiece identity show", Description: "View your identity"},
		{Command: "mailpiece auth list", Description: "List saved profiles"},
	},
	"mailpiece config": {
		{Command: "mailpiece config show", Description: "Show the effective configuration"},
		{Command: "mailpiece config explain", Description: "Explain config precedence"},
		{Command: "mailpiece auth list", Description: "List saved profiles"},
	},
	"mailpiece config show": {
		{Command: "mailpiece config explain", Description: "Explain why these values won"},
		{Command: "mailpiece doctor", Description: "Run a full health check"},
	},
	"mailpiece config explain": {
		{Command: "mailpiece config show", Description: "Show only the effective values"},
		{Command: "mailpiece auth list", Description: "List saved profiles"},
		{Command: "mailpiece doctor", Description: "Run a full health check"},
	},
	"mailpiece doctor": {
		{Command: "mailpiece auth status", Description: "Check current authentication state"},
		{Command: "mailpiece config explain", Description: "Explain config precedence"},
		{Command: "mailpiece identity show", Description: "Verify identity and accessible accounts"},
		{Command: "mailpiece setup", Description: "Repair or re-run interactive setup"},
	},
}
