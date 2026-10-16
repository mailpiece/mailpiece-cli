package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

func quickstartBreadcrumbCmds(t *testing.T) []string {
	t.Helper()
	if lastResult == nil || lastResult.Response == nil {
		t.Fatal("expected a quickstart response")
	}
	cmds := make([]string, 0, len(lastResult.Response.Breadcrumbs))
	for _, crumb := range lastResult.Response.Breadcrumbs {
		cmds = append(cmds, crumb.Cmd)
	}
	return cmds
}

func TestQuickstartAuthenticated(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs()
	assertExitCode(t, err, 0)

	data := responseDataMap(t, lastResult)
	auth := data["auth"].(map[string]any)
	if auth["status"] != "authenticated" || auth["account"] != "acme" {
		t.Fatalf("expected authenticated on acme, got %#v", auth)
	}
	if _, ok := auth["profile"]; ok {
		t.Fatalf("expected no profile, got %#v", auth)
	}
	if want := "mailpiece dev: logged in @ acme"; lastResult.Response.Summary != want {
		t.Fatalf("summary: want %q, got %q", want, lastResult.Response.Summary)
	}
	cmds := quickstartBreadcrumbCmds(t)
	if len(cmds) != 4 || cmds[0] != "mailpiece doctor" {
		t.Fatalf("expected the four quick-start breadcrumbs, got %v", cmds)
	}
	commands := data["commands"].(map[string]any)
	if quick := commands["quick_start"].([]any); len(quick) == 0 || quick[0] != "mailpiece doctor" {
		t.Fatalf("expected quick_start commands, got %#v", commands)
	}
	if len(mock.GetWithPaginationCalls) != 0 {
		t.Fatalf("the quickstart sends no request, got %#v", mock.GetWithPaginationCalls)
	}
}

func TestQuickstartUnauthenticatedOffersLogin(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("", "", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs()
	assertExitCode(t, err, 0)

	data := responseDataMap(t, lastResult)
	auth := data["auth"].(map[string]any)
	if auth["status"] != "unauthenticated" {
		t.Fatalf("expected unauthenticated, got %#v", auth)
	}
	if want := "mailpiece dev: not logged in"; lastResult.Response.Summary != want {
		t.Fatalf("summary: want %q, got %q", want, lastResult.Response.Summary)
	}
	cmds := strings.Join(quickstartBreadcrumbCmds(t), "\n")
	if !strings.Contains(cmds, "mailpiece auth login <token>") || !strings.Contains(cmds, "mailpiece auth email") {
		t.Fatalf("expected login breadcrumbs, got:\n%s", cmds)
	}
}

func TestQuickstartNamesTheSelectedProfile(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("", "", "https://mailpiece.example.com")
	defer resetTest()

	_, err := runCobraWithArgs("--profile", "walter")
	assertExitCode(t, err, 0)

	data := responseDataMap(t, lastResult)
	auth := data["auth"].(map[string]any)
	if auth["profile"] != "walter" {
		t.Fatalf("expected profile walter, got %#v", auth)
	}
	if want := "mailpiece dev: not logged in (profile: walter)"; lastResult.Response.Summary != want {
		t.Fatalf("summary: want %q, got %q", want, lastResult.Response.Summary)
	}
	if cmds := strings.Join(quickstartBreadcrumbCmds(t), "\n"); !strings.Contains(cmds, "mailpiece auth login <token> --profile walter") {
		t.Fatalf("expected the login breadcrumb to keep the profile, got:\n%s", cmds)
	}
}

func TestQuickstartWithAgentIsRawData(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("--agent")
	assertExitCode(t, err, 0)

	var data map[string]any
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		t.Fatalf("expected JSON object, got %v:\n%s", err, raw)
	}
	if _, hasOK := data["ok"]; hasOK {
		t.Fatalf("agent output should not include the envelope, got %#v", data)
	}
	if data["version"] != "dev" {
		t.Fatalf("expected version dev, got %#v", data["version"])
	}
}

func TestQuickstartWithJQ(t *testing.T) {
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("token", "acme", "https://mailpiece.example.com")
	defer resetTest()

	raw, err := runCobraWithArgs("--jq", ".data.auth.status")
	assertExitCode(t, err, 0)
	if got := strings.TrimSpace(raw); got != "authenticated" {
		t.Fatalf("expected 'authenticated', got %q", got)
	}
}
