package commands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetupClaudeWithoutTheClaudeBinaryLinksTheSkill(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", home)
	result := SetTestMode()
	defer resetTest()

	cmd, _, err := rootCmd.Find([]string{"setup", "claude"})
	if err != nil || cmd.Name() != "claude" {
		t.Fatalf("find setup claude: %v", err)
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("setup claude: %v", err)
	}

	data := responseDataMap(t, result)
	if data["plugin_installed"] != false {
		t.Errorf("expected the plugin not installed, got %#v", data["plugin_installed"])
	}
	if _, ok := data["errors"]; ok {
		t.Errorf("expected no errors, got %#v", data["errors"])
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "mailpiece", "SKILL.md")); err != nil {
		t.Fatalf("expected the skill reachable through the link: %v", err)
	}
}

func TestJoinNames(t *testing.T) {
	tests := []struct {
		names []string
		want  string
	}{
		{nil, ""},
		{[]string{"A"}, "A"},
		{[]string{"A", "B"}, "A and B"},
		{[]string{"A", "B", "C"}, "A, B, and C"},
	}
	for _, tt := range tests {
		got := joinNames(tt.names)
		if got != tt.want {
			t.Errorf("joinNames(%v) = %q, want %q", tt.names, got, tt.want)
		}
	}
}
