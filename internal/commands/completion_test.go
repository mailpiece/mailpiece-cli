package commands

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureCompletionStdout runs fn with os.Stdout on a pipe, reading it as it
// is written so a long script cannot fill the pipe.
func captureCompletionStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		body, _ := io.ReadAll(r)
		done <- string(body)
	}()
	runErr := fn()
	_ = w.Close()
	os.Stdout = oldStdout
	return <-done, runErr
}

func TestCompletionGeneratesEachShell(t *testing.T) {
	tests := []struct {
		shell string
		want  string
	}{
		{"bash", "mailpiece"},
		{"zsh", "#compdef mailpiece"},
		{"fish", "complete -c mailpiece"},
		{"powershell", "mailpiece"},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			SetTestMode()
			defer resetTest()

			script, err := captureCompletionStdout(t, func() error {
				return completionCmd.RunE(completionCmd, []string{tt.shell})
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(script, tt.want) {
				t.Fatalf("expected %s script to contain %q, got:\n%.300s", tt.shell, tt.want, script)
			}
		})
	}
}

func TestCompletionRejectsUnknownShell(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("completion", "tcsh")
	if err == nil || !strings.Contains(err.Error(), "invalid argument") {
		t.Fatalf("expected an invalid argument error, got %v", err)
	}
}

func TestCompletionRequiresOneShell(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	if _, err := runCobraWithArgs("completion"); err == nil {
		t.Fatal("expected an error without a shell")
	}
}
