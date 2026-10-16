package commands

import (
	"testing"

	"github.com/basecamp/cli/output"
)

func TestVersionOutputMarkdownPlainText(t *testing.T) {
	SetTestMode()
	SetTestFormat(output.FormatMarkdown)
	defer resetTest()

	if err := versionCmd.RunE(versionCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := TestOutput(); got != versionLine()+"\n" {
		t.Errorf("expected plain version line, got %q", got)
	}
}

func TestSetVersion(t *testing.T) {
	defer SetVersion("dev")

	SetVersion("1.4.0")
	if currentVersion() != "1.4.0" || rootCmd.Version != "1.4.0" {
		t.Fatalf("expected 1.4.0, got %q and %q", currentVersion(), rootCmd.Version)
	}

	SetVersion("")
	if currentVersion() != "1.4.0" {
		t.Fatalf("an empty version must leave the version alone, got %q", currentVersion())
	}
}

func TestVersionReportsSetVersion(t *testing.T) {
	result := SetTestMode()
	defer resetTest()
	SetVersion("2.1.0")
	defer SetVersion("dev")

	if err := versionCmd.RunE(versionCmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := responseDataMap(t, result)
	if data["version"] != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %#v", data["version"])
	}
}

func TestVersionLineNamesCommitAndDate(t *testing.T) {
	defer SetVersion("dev")
	defer SetBuildInfo("", "")

	SetVersion("dev")
	if got := versionLine(); got != "mailpiece version dev (built from source)" {
		t.Errorf("dev build: got %q", got)
	}

	SetVersion("1.2.0")
	SetBuildInfo("abc1234", "2026-10-15T12:00:00Z")
	if got := versionLine(); got != "mailpiece version 1.2.0 (commit abc1234, built 2026-10-15T12:00:00Z)" {
		t.Errorf("release build: got %q", got)
	}

	SetBuildInfo("", "")
	if got := versionLine(); got != "mailpiece version 1.2.0" {
		t.Errorf("go install build: got %q", got)
	}
}
