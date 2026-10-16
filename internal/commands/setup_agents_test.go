package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupClaudeInstallsAndLinksTheSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	result := SetTestMode()
	defer resetTest()

	var buf bytes.Buffer
	setupClaudeCmd.SetOut(&buf)
	defer setupClaudeCmd.SetOut(nil)

	if err := runSetupClaude(setupClaudeCmd, nil); err != nil {
		t.Fatalf("runSetupClaude() error = %v", err)
	}

	data := responseDataMap(t, result)
	if data["marketplace_installed"] != false {
		t.Errorf("expected no marketplace install, got %#v", data["marketplace_installed"])
	}
	wantSkill := filepath.Join(home, ".agents", "skills", "mailpiece", "SKILL.md")
	if data["skill_path"] != wantSkill {
		t.Errorf("skill_path = %#v, want %q", data["skill_path"], wantSkill)
	}
	wantLink := filepath.Join(home, ".claude", "skills", "mailpiece")
	if data["symlink_path"] != wantLink {
		t.Errorf("symlink_path = %#v, want %q", data["symlink_path"], wantLink)
	}
	if _, ok := data["notice"]; ok {
		t.Errorf("expected no notice, got %#v", data["notice"])
	}
	if result.Response.Summary != "Mailpiece skill linked for Claude Code" {
		t.Errorf("summary = %q", result.Response.Summary)
	}

	if _, err := os.Stat(filepath.Join(wantLink, "SKILL.md")); err != nil {
		t.Fatalf("expected the skill reachable through the link: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no human lines in machine mode, got %q", buf.String())
	}
}

func TestSetupClaudeIsASubcommandOfSetup(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"setup", "claude"})
	if err != nil {
		t.Fatalf("find command: %v", err)
	}
	if cmd != setupClaudeCmd {
		t.Fatalf("expected setup claude, got %s", cmd.CommandPath())
	}
}
