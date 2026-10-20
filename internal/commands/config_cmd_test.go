package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
)

func TestConfigShow(t *testing.T) {
	configDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "acme",
		BaseURL: "https://profile.example.com",
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileStore.SetDefault("acme"); err != nil {
		t.Fatalf("set default profile: %v", err)
	}

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestProfiles(profileStore)
	SetTestConfig("", "", "https://mailpiece.example.com")
	t.Setenv("MAILPIECE_TOKEN", "env-token")
	defer resetTest()
	if err := resolveProfile(); err != nil {
		t.Fatalf("resolve profile: %v", err)
	}

	err := configShowCmd.RunE(configShowCmd, []string{})
	assertExitCode(t, err, 0)

	data, ok := result.Response.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map response data, got %#v", result.Response.Data)
	}
	if data["profile"] != "acme" {
		t.Fatalf("expected profile acme, got %#v", data["profile"])
	}
	if data["api_url"] != "https://profile.example.com" {
		t.Fatalf("expected profile api url, got %#v", data["api_url"])
	}
	if data["account"] != "acme" {
		t.Fatalf("expected account acme from the profile, got %#v", data["account"])
	}
	token, ok := data["token"].(map[string]any)
	if !ok {
		t.Fatalf("expected token map, got %#v", data["token"])
	}
	if token["configured"] != true {
		t.Fatalf("expected configured token, got %#v", token)
	}
	if token["source"] != "environment variable" {
		t.Fatalf("expected env token source, got %#v", token)
	}
	for _, old := range []string{"token_configured", "token_source"} {
		if _, ok := data[old]; ok {
			t.Fatalf("expected no %s key, got %#v", old, data)
		}
	}
	profiles, ok := data["profiles"].([]string)
	if !ok {
		items, okAny := data["profiles"].([]any)
		if !okAny {
			t.Fatalf("expected profiles slice, got %#v", data["profiles"])
		}
		profiles = make([]string, 0, len(items))
		for _, item := range items {
			profiles = append(profiles, item.(string))
		}
	}
	if len(profiles) != 1 || profiles[0] != "acme (default)" {
		t.Fatalf("expected saved profile list, got %#v", profiles)
	}
	if data["global_config"] != filepath.Join(configDir, "config.yaml") {
		t.Fatalf("expected global config path, got %#v", data["global_config"])
	}
	if result.Response.Summary != "Effective configuration" {
		t.Fatalf("summary: got %q", result.Response.Summary)
	}
}

func TestConfigShowTokenNotConfigured(t *testing.T) {
	config.SetTestConfigDir(t.TempDir())
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	result := SetTestMode()
	SetTestConfig("", "acme", "https://mailpiece.example.com")
	defer resetTest()

	assertExitCode(t, configShowCmd.RunE(configShowCmd, []string{}), 0)

	token := responseDataMap(t, result)["token"].(map[string]any)
	if token["configured"] != false || token["source"] != "not configured" {
		t.Fatalf("expected token {configured: false, source: not configured}, got %#v", token)
	}
}

func TestConfigShowTokenFromCredentialFile(t *testing.T) {
	config.SetTestConfigDir(t.TempDir())
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "acme", "stored-token")
	result := SetTestMode()
	SetTestCreds(store)
	SetTestConfig("", "acme", "https://mailpiece.example.com")
	activeProfile = "acme"
	defer resetTest()

	assertExitCode(t, configShowCmd.RunE(configShowCmd, []string{}), 0)

	token := responseDataMap(t, result)["token"].(map[string]any)
	if token["configured"] != true || token["source"] != "fallback credential file" {
		t.Fatalf("expected token from the fallback credential file, got %#v", token)
	}
}

func TestConfigExplainShowsPrecedence(t *testing.T) {
	configDir := t.TempDir()
	workDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	config.SetTestWorkingDir(workDir)
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte("account: global\napi_url: https://global.example.com\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, config.LocalConfigFile), []byte("account: local\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "acme",
		BaseURL: "https://profile.example.com",
		Extra: map[string]json.RawMessage{
			"account": json.RawMessage(`"1"`),
		},
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileStore.SetDefault("acme"); err != nil {
		t.Fatalf("set default profile: %v", err)
	}

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestProfiles(profileStore)
	t.Setenv("MAILPIECE_PROFILE", "acme")
	t.Setenv("MAILPIECE_API_URL", "https://env.example.com")
	cfg = config.Load()
	defer resetTest()
	if err := resolveProfile(); err != nil {
		t.Fatalf("resolve profile: %v", err)
	}

	err := configExplainCmd.RunE(configExplainCmd, []string{})
	assertExitCode(t, err, 0)

	data, ok := result.Response.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map response data, got %#v", result.Response.Data)
	}

	profileField := data["profile"].(map[string]any)
	if profileField["source"] != "env MAILPIECE_PROFILE" {
		t.Fatalf("expected env profile source, got %#v", profileField)
	}
	accountField := data["account"].(map[string]any)
	if accountField["value"] != "1" || accountField["source"] != "profile acme" {
		t.Fatalf("expected account 1 from profile acme, got %#v", accountField)
	}
	apiURLField := data["api_url"].(map[string]any)
	if apiURLField["source"] != "env MAILPIECE_API_URL" {
		t.Fatalf("expected env api url source, got %#v", apiURLField)
	}

	candidates := apiURLField["candidates"].([]any)
	var sawEnvSelected bool
	for _, item := range candidates {
		candidate := item.(map[string]any)
		if candidate["source"] == "env MAILPIECE_API_URL" && candidate["selected"] == true {
			sawEnvSelected = true
		}
	}
	if !sawEnvSelected {
		t.Fatalf("expected env API URL candidate to be selected, got %#v", candidates)
	}

	accountSources := map[string]any{}
	for _, item := range accountField["candidates"].([]any) {
		candidate := item.(map[string]any)
		accountSources[candidate["source"].(string)] = candidate["value"]
	}
	if accountSources["local config"] != "local" || accountSources["global config"] != "global" {
		t.Fatalf("expected local and global account candidates, got %#v", accountSources)
	}

	tokenField := data["token"].(map[string]any)
	if tokenField["configured"] != false || tokenField["source"] != "not configured" {
		t.Fatalf("expected an unconfigured token, got %#v", tokenField)
	}
}

func TestConfigExplainAccountCandidatesFollowProfileFallback(t *testing.T) {
	configDir := t.TempDir()
	workDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	config.SetTestWorkingDir(workDir)
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	if err := os.WriteFile(filepath.Join(workDir, config.LocalConfigFile), []byte("account: stale-local\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestProfiles(profileStore)
	t.Setenv("MAILPIECE_PROFILE", "new-account")
	cfg = config.Load()
	defer resetTest()

	if err := resolveProfile(); err != nil {
		t.Fatalf("resolve profile fallback: %v", err)
	}
	field := configExplainData()["account"].(configExplainField)
	if field.Value != "new-account" || field.Source != "env MAILPIECE_PROFILE" {
		t.Fatalf("effective account: want new-account from env, got %#v", field)
	}

	selected := ""
	for _, candidate := range field.Candidates {
		if candidate.Selected {
			selected = candidate.Source
		}
	}
	if selected != "env MAILPIECE_PROFILE" {
		t.Fatalf("selected account candidate: want env MAILPIECE_PROFILE, got %q (%#v)", selected, field.Candidates)
	}
}

func TestConfigShowStyledIncludesProfiles(t *testing.T) {
	configDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{Name: "acme", BaseURL: "https://acme.example.com"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileStore.SetDefault("acme"); err != nil {
		t.Fatalf("set default: %v", err)
	}

	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestProfiles(profileStore)
	SetTestFormat(output.FormatStyled)
	SetTestConfig("", "acme", "https://acme.example.com")
	defer resetTest()

	err := configShowCmd.RunE(configShowCmd, []string{})
	assertExitCode(t, err, 0)

	raw := TestOutput()
	if !strings.Contains(raw, "acme") {
		t.Fatalf("expected saved profile 'acme' in styled output, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Saved Profiles") {
		t.Fatalf("expected 'Saved Profiles' label in styled output, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Token          not configured") {
		t.Fatalf("expected the token line, got:\n%s", raw)
	}
}

func TestConfigShowJSONIncludesProfiles(t *testing.T) {
	configDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))
	if err := profileStore.Create(&profile.Profile{Name: "staging", BaseURL: "https://staging.example.com"}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := profileStore.SetDefault("staging"); err != nil {
		t.Fatalf("set default: %v", err)
	}

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestProfiles(profileStore)
	SetTestConfig("", "staging", "https://staging.example.com")
	defer resetTest()

	err := configShowCmd.RunE(configShowCmd, []string{})
	assertExitCode(t, err, 0)

	data, ok := result.Response.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map response data, got %#v", result.Response.Data)
	}
	profiles, ok := data["profiles"]
	if !ok {
		t.Fatalf("expected 'profiles' key in JSON response, got keys: %v", mapKeys(data))
	}
	items, ok := profiles.([]string)
	if !ok {
		anyItems, okAny := profiles.([]any)
		if !okAny {
			t.Fatalf("expected profiles slice, got %#v", profiles)
		}
		items = make([]string, 0, len(anyItems))
		for _, item := range anyItems {
			items = append(items, item.(string))
		}
	}
	if len(items) != 1 || !strings.Contains(items[0], "staging") {
		t.Fatalf("expected saved profile 'staging', got %#v", items)
	}
}

func mapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func TestConfigExplainStyledOutput(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestFormat(output.FormatStyled)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	err := configExplainCmd.RunE(configExplainCmd, []string{})
	assertExitCode(t, err, 0)

	raw := TestOutput()
	if !strings.Contains(raw, "Mailpiece Config Explain") {
		t.Fatalf("expected explain heading, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Next steps") {
		t.Fatalf("expected next steps section, got:\n%s", raw)
	}
}

func TestConfigShowMarkdownOutput(t *testing.T) {
	config.SetTestConfigDir(t.TempDir())
	defer config.ResetTestConfigDir()

	SetTestMode()
	SetTestFormat(output.FormatMarkdown)
	SetTestConfig("", "acme", "https://mailpiece.example.com")
	t.Setenv("MAILPIECE_TOKEN", "env-token")
	defer resetTest()

	assertExitCode(t, configShowCmd.RunE(configShowCmd, []string{}), 0)

	raw := TestOutput()
	for _, want := range []string{"# Mailpiece Config", "- **Account:** `acme`", "- **Token:** configured (environment variable)", "## Next steps"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in markdown config, got:\n%s", want, raw)
		}
	}
}
