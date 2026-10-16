package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// setupClaudeCmd installs the embedded Mailpiece skill for Claude Code and
// links it into ~/.claude/skills/mailpiece, as `mailpiece setup` offers to do
// for detected agents. There is no Claude Code plugin marketplace entry for
// Mailpiece yet, so, unlike Fizzy's `fizzy setup claude`, this never runs
// `claude plugin marketplace add` or `claude plugin install`; it only
// installs and links the skill.
var setupClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Install the Mailpiece agent skill for Claude Code",
	Long:  "Installs the embedded Mailpiece skill to ~/.agents/skills/mailpiece and links it into ~/.claude/skills/mailpiece. There is no Claude Code plugin marketplace entry for Mailpiece yet, so no marketplace or plugin install is attempted.",
	RunE:  runSetupClaude,
}

func init() {
	setupCmd.AddCommand(setupClaudeCmd)
}

func runSetupClaude(cmd *cobra.Command, args []string) error {
	w := cmd.OutOrStdout()
	human := !IsMachineOutput()

	if human {
		fmt.Fprintln(w, "  No Claude Code marketplace entry for Mailpiece yet: skipping plugin install.")
	}

	skillPath, err := installSkillFiles()
	if err != nil {
		return err
	}
	if human {
		fmt.Fprintf(w, "  ✓ Skill installed to %s\n", skillPath)
	}

	symlinkPath, notice, err := linkSkillToClaude()
	if err != nil {
		return err
	}
	if human {
		fmt.Fprintf(w, "  ✓ Linked into %s\n", symlinkPath)
		if notice != "" {
			fmt.Fprintf(w, "  ⚠ %s\n", notice)
		}
	}

	result := map[string]any{
		"marketplace_installed": false,
		"skill_path":            skillPath,
		"symlink_path":          symlinkPath,
	}
	if notice != "" {
		result["notice"] = notice
	}

	printMutation(result, "Mailpiece skill linked for Claude Code", nil)
	return nil
}
