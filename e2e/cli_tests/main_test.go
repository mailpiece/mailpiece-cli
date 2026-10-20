// Package clitests contains owner-only end-to-end tests for the Mailpiece CLI.
//
// Required environment variables:
//   - MAILPIECE_TEST_TOKEN
//   - MAILPIECE_TEST_ACCOUNT
//
// Optional:
//   - MAILPIECE_TEST_API_URL
//   - MAILPIECE_TEST_BINARY
//   - MAILPIECE_TEST_WRITE_ACCOUNT: the scratch account tests that write run
//     against. Without it they skip; the demo account is refused.
package clitests

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mailpiece/mailpiece-cli/e2e/harness"
)

var cfg *harness.Config

func TestMain(m *testing.M) {
	os.Exit(runMain(m))
}

func runMain(m *testing.M) int {
	cfg = harness.LoadConfig()
	if missing := cfg.MissingVars(); len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Skipping CLI e2e tests: missing env vars: %v\n", missing)
		fmt.Fprintln(os.Stderr, "Set MAILPIECE_TEST_TOKEN and MAILPIECE_TEST_ACCOUNT to run these tests.")
		return 0
	}

	if !fileExists(cfg.BinaryPath) {
		repoRoot, err := harness.RepoRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		tmpDir, err := os.MkdirTemp("", "mailpiece-e2e-cli-build-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		defer os.RemoveAll(tmpDir)
		binPath := filepath.Join(tmpDir, "mailpiece")
		cmd := exec.CommandContext(context.Background(), "go", "build", "-o", binPath, "./cmd/mailpiece")
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to build binary: %v\n%s\n", err, string(out))
			return 1
		}
		_ = os.Setenv("MAILPIECE_TEST_BINARY", binPath)
		cfg.BinaryPath = binPath
	}

	if err := cfg.WriteAccountError(); err != nil {
		fmt.Fprintf(os.Stderr, "CLI e2e: reads only (%v)\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "CLI e2e: writes go to scratch account %s\n", cfg.WriteAccount)
	}

	return m.Run()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
