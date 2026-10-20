package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
	"github.com/spf13/cobra"
)

func TestDoctorResultSummary(t *testing.T) {
	tests := []struct {
		name   string
		result DoctorResult
		want   string
	}{
		{name: "all passed", result: DoctorResult{Passed: 4}, want: "All 4 checks passed"},
		{name: "all passed with skipped", result: DoctorResult{Passed: 4, Skipped: 1}, want: "All 4 checks passed, 1 skipped"},
		{name: "mixed", result: DoctorResult{Passed: 3, Failed: 1, Warned: 2, Skipped: 1}, want: "3 passed, 1 failed, 2 warnings, 1 skipped"},
		{name: "one warning", result: DoctorResult{Passed: 3, Warned: 1}, want: "3 passed, 1 warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.Summary(); got != tt.want {
				t.Fatalf("Summary() = %q, want %q", got, tt.want)
			}
		})
	}
}

// doctorIdentityMock answers doctor's identity and account checks for acme.
func doctorIdentityMock() *MockClient {
	mock := NewMockClient()
	mock.OnGet("/my/identity.json", &APIResponse{StatusCode: 200, Data: map[string]any{
		"id":       "identity-123",
		"accounts": []any{map[string]any{"id": "1", "slug": "/acme", "name": "Acme", "user": map[string]any{"id": "u1", "name": "Ana", "role": "owner"}}},
	}})
	mock.OnGet("/users/u1.json", &APIResponse{StatusCode: 200, Data: map[string]any{"id": "u1", "name": "Ana", "role": "owner"}})
	return mock
}

func findDoctorCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd, _, err := rootCmd.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("find doctor command: %v", err)
	}
	return cmd
}

func doctorCheckStatuses(t *testing.T, checks any) map[string]map[string]any {
	t.Helper()
	items, ok := checks.([]any)
	if !ok {
		t.Fatalf("expected checks array, got %#v", checks)
	}
	found := map[string]map[string]any{}
	for _, item := range items {
		check := item.(map[string]any)
		found[check["name"].(string)] = check
	}
	return found
}

func TestDoctorCommandNoCredentials(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	if result.Response == nil || !result.Response.OK {
		t.Fatalf("expected OK response, got %#v", result.Response)
	}

	data, ok := result.Response.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map response data, got %#v", result.Response.Data)
	}
	checks := doctorCheckStatuses(t, data["checks"])
	check, ok := checks["Credentials"]
	if !ok {
		t.Fatal("expected Credentials check in doctor output")
	}
	if check["status"] != "fail" {
		t.Fatalf("expected credentials fail, got %#v", check)
	}
	if !strings.Contains(check["message"].(string), "No credentials found") {
		t.Fatalf("expected missing credentials message, got %#v", check)
	}
	for _, name := range []string{"Authentication", "Account Access"} {
		if checks[name]["status"] != "skip" {
			t.Fatalf("expected %s skipped, got %#v", name, checks[name])
		}
	}
}

func TestDoctorCommandHealthySetup(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := doctorIdentityMock()
	result := SetTestModeWithSDK(mock)
	SetTestConfig("test-token", "acme", testHTTPServer.URL)
	t.Setenv("MAILPIECE_TOKEN", "test-token")
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	if result.Response == nil || !result.Response.OK {
		t.Fatalf("expected OK response, got %#v", result.Response)
	}

	data := result.Response.Data.(map[string]any)
	checks := doctorCheckStatuses(t, data["checks"])
	for _, name := range []string{"Credentials", "API Reachability", "Authentication", "Account Access"} {
		if got := checks[name]["status"]; got != "pass" {
			t.Fatalf("expected %s to pass, got %#v", name, checks[name])
		}
	}
	if msg := checks["Account Access"]["message"].(string); !strings.HasPrefix(msg, "Account acme accessible") {
		t.Fatalf("expected account acme accessible, got %q", msg)
	}
}

func TestDoctorAuthenticationFailure(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	mock.OnGet("/my/identity.json", &APIResponse{StatusCode: 401, Data: map[string]any{"error": "Unauthorized"}})
	result := SetTestModeWithSDK(mock)
	SetTestConfig("bad-token", "acme", testHTTPServer.URL)
	t.Setenv("MAILPIECE_TOKEN", "bad-token")
	defer resetTest()

	cmd := findDoctorCmd(t)
	assertExitCode(t, cmd.RunE(cmd, []string{}), 0)

	checks := doctorCheckStatuses(t, responseDataMap(t, result)["checks"])
	if checks["Authentication"]["status"] != "fail" {
		t.Fatalf("expected authentication to fail, got %#v", checks["Authentication"])
	}
	if checks["Account Access"]["status"] != "skip" || checks["Account Access"]["message"] != "Skipped (authentication failed)" {
		t.Fatalf("expected account access skipped, got %#v", checks["Account Access"])
	}
}

func TestDoctorDetectsInvalidLocalConfig(t *testing.T) {
	workDir := t.TempDir()
	configDir := t.TempDir()
	config.SetTestWorkingDir(workDir)
	config.SetTestConfigDir(configDir)
	defer config.ResetTestWorkingDir()
	defer config.ResetTestConfigDir()

	if err := os.WriteFile(filepath.Join(workDir, config.LocalConfigFile), []byte("token: [broken"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	data := result.Response.Data.(map[string]any)
	check, ok := doctorCheckStatuses(t, data["checks"])["Local Config"]
	if !ok {
		t.Fatal("expected Local Config check in output")
	}
	if check["status"] != "fail" {
		t.Fatalf("expected Local Config fail, got %#v", check)
	}
	if !strings.Contains(check["message"].(string), "Invalid YAML") {
		t.Fatalf("expected invalid yaml message, got %#v", check)
	}
}

func TestDoctorWarnsOnLocalTokenStorage(t *testing.T) {
	workDir := t.TempDir()
	configDir := t.TempDir()
	config.SetTestWorkingDir(workDir)
	config.SetTestConfigDir(configDir)
	defer config.ResetTestWorkingDir()
	defer config.ResetTestConfigDir()

	mock := doctorIdentityMock()
	result := SetTestModeWithSDK(mock)
	localConfig := "token: local-token\naccount: acme\napi_url: " + testHTTPServer.URL + "\n"
	if err := os.WriteFile(filepath.Join(workDir, config.LocalConfigFile), []byte(localConfig), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	cfg = config.Load()
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	data := result.Response.Data.(map[string]any)
	check, ok := doctorCheckStatuses(t, data["checks"])["Credential Storage"]
	if !ok {
		t.Fatal("expected Credential Storage check in output")
	}
	if check["status"] != "warn" {
		t.Fatalf("expected Credential Storage warn, got %#v", check)
	}
	if !strings.Contains(check["message"].(string), "local project config") {
		t.Fatalf("expected local config storage warning, got %#v", check)
	}
}

func TestDoctorStyledOutputIncludesNextSteps(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("", "", testHTTPServer.URL)
	SetTestFormat(output.FormatStyled)
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	raw := TestOutput()
	if !strings.Contains(raw, "Mailpiece CLI Doctor") {
		t.Fatalf("expected styled title, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Next steps") {
		t.Fatalf("expected next steps section, got:\n%s", raw)
	}
	if !strings.Contains(raw, "mailpiece auth login <token>") {
		t.Fatalf("expected auth hint command, got:\n%s", raw)
	}
}

func TestDoctorMarkdownOutput(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("", "", testHTTPServer.URL)
	SetTestFormat(output.FormatMarkdown)
	defer resetTest()

	cmd := findDoctorCmd(t)
	assertExitCode(t, cmd.RunE(cmd, []string{}), 0)

	raw := TestOutput()
	for _, want := range []string{"# Mailpiece CLI Doctor", "**Credentials:** No credentials found", "**Summary:**", "## Next steps", "- `mailpiece auth login <token>`"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("expected %q in markdown doctor, got:\n%s", want, raw)
		}
	}
}

func TestDoctorShowsSavedProfileNames(t *testing.T) {
	configDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	if err := profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testHTTPServer.URL}); err != nil {
		t.Fatalf("create acme profile: %v", err)
	}
	if err := profileStore.Create(&profile.Profile{Name: "staging", BaseURL: testHTTPServer.URL}); err != nil {
		t.Fatalf("create staging profile: %v", err)
	}
	if err := profileStore.SetDefault("acme"); err != nil {
		t.Fatalf("set default profile: %v", err)
	}
	SetTestProfiles(profileStore)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	cmd := findDoctorCmd(t)
	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	data := result.Response.Data.(map[string]any)
	check, ok := doctorCheckStatuses(t, data["checks"])["Saved Profiles"]
	if !ok {
		t.Fatal("expected Saved Profiles check in output")
	}
	msg := check["message"].(string)
	if !strings.Contains(msg, "acme (default)") || !strings.Contains(msg, "staging") {
		t.Fatalf("expected saved profile names in message, got %#v", check)
	}
	if !strings.Contains(check["hint"].(string), "--all-profiles") {
		t.Fatalf("expected --all-profiles hint, got %#v", check)
	}
}

func TestDoctorAllProfilesIncludesPerProfileResults(t *testing.T) {
	configDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	store := newTestCredStore(t, t.TempDir())
	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))

	mock := doctorIdentityMock()
	result := SetTestModeWithSDK(mock)
	if err := profileStore.Create(&profile.Profile{Name: "walter", BaseURL: testHTTPServer.URL, Extra: map[string]json.RawMessage{"account": json.RawMessage(`"acme"`)}}); err != nil {
		t.Fatalf("create walter profile: %v", err)
	}
	if err := profileStore.Create(&profile.Profile{Name: "staging", BaseURL: testHTTPServer.URL}); err != nil {
		t.Fatalf("create staging profile: %v", err)
	}
	if err := profileStore.SetDefault("walter"); err != nil {
		t.Fatalf("set default profile: %v", err)
	}
	saveTestToken(t, store, "walter", "test-token")
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	cmd := findDoctorCmd(t)
	if err := cmd.Flags().Set("all-profiles", "true"); err != nil {
		t.Fatalf("set all-profiles flag: %v", err)
	}
	defer cmd.Flags().Set("all-profiles", "false")

	err := cmd.RunE(cmd, []string{})
	assertExitCode(t, err, 0)

	data := result.Response.Data.(map[string]any)
	profiles, ok := data["profiles"].([]any)
	if !ok {
		t.Fatalf("expected profiles array, got %#v", data["profiles"])
	}
	if len(profiles) != 2 {
		t.Fatalf("expected 2 profile results, got %d", len(profiles))
	}

	found := map[string]map[string]string{}
	defaults := map[string]bool{}
	for _, item := range profiles {
		profileResult := item.(map[string]any)
		name := profileResult["name"].(string)
		defaults[name] = profileResult["default"] == true
		statuses := map[string]string{}
		for checkName, check := range doctorCheckStatuses(t, profileResult["checks"]) {
			statuses[checkName] = check["status"].(string)
		}
		found[name] = statuses
	}

	if found["walter"]["Authentication"] != "pass" || found["walter"]["Account Access"] != "pass" {
		t.Fatalf("expected walter authentication and account access to pass, got %#v", found["walter"])
	}
	if found["walter"]["Credential Storage"] != "warn" {
		t.Fatalf("expected walter's fallback credential file to warn, got %#v", found["walter"])
	}
	if found["staging"]["Credentials"] != "fail" {
		t.Fatalf("expected staging credentials to fail, got %#v", found["staging"])
	}
	if !defaults["walter"] || defaults["staging"] {
		t.Fatalf("expected walter as the default, got %#v", defaults)
	}
	if _, ok := doctorCheckStatuses(t, data["checks"])["Credentials"]; ok {
		t.Fatal("expected no top-level Credentials check in a profile sweep")
	}
}

func TestDoctorAllProfilesWithNoProfiles(t *testing.T) {
	configDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	defer config.ResetTestConfigDir()

	mock := NewMockClient()
	result := SetTestModeWithSDK(mock)
	SetTestProfiles(profile.NewStore(filepath.Join(t.TempDir(), "config.json")))
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	cmd := findDoctorCmd(t)
	if err := cmd.Flags().Set("all-profiles", "true"); err != nil {
		t.Fatalf("set all-profiles flag: %v", err)
	}
	defer cmd.Flags().Set("all-profiles", "false")

	assertExitCode(t, cmd.RunE(cmd, []string{}), 0)

	check, ok := doctorCheckStatuses(t, responseDataMap(t, result)["checks"])["Profile Sweep"]
	if !ok || check["status"] != "skip" || check["message"] != "No saved profiles to check" {
		t.Fatalf("expected a skipped profile sweep, got %#v", check)
	}
}

func TestDoctorAllProfilesRefusals(t *testing.T) {
	tests := []struct {
		name string
		set  func()
		want string
	}{
		{"profile", func() { cfgProfile = "acme" }, "--all-profiles cannot be used with --profile"},
		{"token", func() { cfgToken = "t" }, "--all-profiles cannot be used with --token"},
		{"api-url", func() { cfgAPIURL = "https://mailpiece.example.com" }, "--all-profiles cannot be used with --api-url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := NewMockClient()
			result := SetTestModeWithSDK(mock)
			SetTestConfig("", "", testHTTPServer.URL)
			defer resetTest()

			cmd := findDoctorCmd(t)
			if err := cmd.Flags().Set("all-profiles", "true"); err != nil {
				t.Fatalf("set all-profiles flag: %v", err)
			}
			defer cmd.Flags().Set("all-profiles", "false")
			tt.set()

			err := cmd.RunE(cmd, []string{})
			assertExitCode(t, err, 1)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
			if result.Response != nil {
				t.Fatalf("expected no doctor output, got %#v", result.Response)
			}
			if len(mock.GetCalls) != 0 {
				t.Fatalf("expected no request, got %#v", mock.GetCalls)
			}
		})
	}
}

func TestDoctorTokenlessAccountProfileStillChecksReachability(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	eff := doctorEffectiveConfig{
		ProfileName:    "account",
		Account:        "account",
		APIURL:         server.URL,
		APIURLSource:   "profile store",
		TokenSource:    "not configured",
		TokenSourceRaw: "none",
	}
	_ = runDoctorTargetChecks(context.Background(), eff, false)
	if got := requests.Load(); got == 0 {
		t.Fatal("doctor skipped reachability for account-named profile")
	}
}

func TestDoctorTokenlessAliasSendsNoToken(t *testing.T) {
	var authorized atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			authorized.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	config.SetTestConfigDir(t.TempDir())
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "other", "other-token")
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "agent",
		BaseURL: server.URL,
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"1"`)},
	}); err != nil {
		t.Fatalf("create alias: %v", err)
	}
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	defer resetTest()

	targets := doctorTargetsFromProfileStore()
	if len(targets) != 1 {
		t.Fatalf("targets: want 1, got %d", len(targets))
	}
	if targets[0].Token != "" {
		t.Fatalf("alias borrowed another profile's credential %q", targets[0].Token)
	}
	if targets[0].Account != "1" {
		t.Fatalf("alias account: want 1, got %q", targets[0].Account)
	}
	checks := runDoctorTargetChecks(context.Background(), targets[0], false)
	if checks[1].Name != "Credentials" || checks[1].Status != "fail" {
		t.Fatalf("expected credentials to fail, got %#v", checks[1])
	}
	if got := authorized.Load(); got != 0 {
		t.Fatalf("doctor sent a token in %d request(s) for a tokenless alias", got)
	}
}

func TestDoctorInvalidAliasMetadataDoesNotSendRequests(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config.SetTestConfigDir(t.TempDir())
	config.SetTestWorkingDir(t.TempDir())
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	store := newTestCredStore(t, t.TempDir())
	saveTestToken(t, store, "agent", "agent-token")
	profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))
	if err := profileStore.Create(&profile.Profile{
		Name:    "agent",
		BaseURL: server.URL,
		Extra:   map[string]json.RawMessage{"account": json.RawMessage(`"../other"`)},
	}); err != nil {
		t.Fatalf("create invalid alias: %v", err)
	}
	SetTestCreds(store)
	SetTestProfiles(profileStore)
	defer resetTest()

	targets := doctorTargetsFromProfileStore()
	if len(targets) != 1 || targets[0].ConfigError == "" {
		t.Fatalf("expected invalid target, got %#v", targets)
	}
	checks := runDoctorTargetChecks(context.Background(), targets[0], false)
	if checks[0].Status != "fail" {
		t.Fatalf("effective config check: want fail, got %#v", checks[0])
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("doctor sent %d request(s) for invalid alias", got)
	}
}

func TestDoctorAccountAccessRoutesAliasThroughAccount(t *testing.T) {
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u1"}`))
	}))
	defer server.Close()

	// The identity names the account by its handle while the profile holds its number.
	identity := &mailpiece.Identity{Accounts: []mailpiece.IdentityAccount{{Slug: "/acme-news", User: mailpiece.User{ID: "u1"}}}}
	check := checkDoctorAccountAccess(context.Background(), doctorEffectiveConfig{
		ProfileName:   "walter",
		Account:       "acme",
		ProfileSource: "profile store",
		APIURL:        server.URL,
		Token:         "test-token",
	}, identity, false)
	if check.Status != "pass" {
		t.Fatalf("account access: want pass, got %#v", check)
	}
	if path := <-paths; path != "/acme/users/u1.json" {
		t.Fatalf("request path: want /acme/users/u1.json, got %q", path)
	}
}

func TestDoctorTargetsFromProfileStoreUsesYAMLTokenFallback(t *testing.T) {
	configDir := t.TempDir()
	workDir := t.TempDir()
	profileDir := t.TempDir()
	config.SetTestConfigDir(configDir)
	config.SetTestWorkingDir(workDir)
	defer config.ResetTestConfigDir()
	defer config.ResetTestWorkingDir()

	globalConfig := "token: global-token\napi_url: https://global.example.com\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(globalConfig), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	profileStore := profile.NewStore(filepath.Join(profileDir, "config.json"))

	mock := NewMockClient()
	SetTestModeWithSDK(mock)

	if err := profileStore.Create(&profile.Profile{Name: "acme", BaseURL: testHTTPServer.URL}); err != nil {
		t.Fatalf("create acme profile: %v", err)
	}
	if err := profileStore.SetDefault("acme"); err != nil {
		t.Fatalf("set default profile: %v", err)
	}
	SetTestCreds(nil)
	SetTestProfiles(profileStore)
	SetTestConfig("", "", testHTTPServer.URL)
	defer resetTest()

	targets := doctorTargetsFromProfileStore()
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}

	target := targets[0]
	if target.APIURL != testHTTPServer.URL {
		t.Fatalf("expected APIURL from profile store, got %q", target.APIURL)
	}
	if target.APIURLSource != "profile store" {
		t.Fatalf("expected APIURLSource 'profile store', got %q", target.APIURLSource)
	}
	if target.Account != "acme" || !target.Default {
		t.Fatalf("expected default profile bound to acme, got %#v", target)
	}
	if target.Token != "global-token" || target.TokenSourceRaw != "global-config" {
		t.Fatalf("expected token from global config, got %q (%s)", target.Token, target.TokenSourceRaw)
	}
}

func TestDoctorStoredTokenSourceIgnoresAccountMismatch(t *testing.T) {
	globalCfg := &config.Config{Token: "yaml-token", Account: "other-account"}
	localCfg := (*config.Config)(nil)

	savedCreds := creds
	creds = nil
	defer func() { creds = savedCreds }()

	raw, source, token := doctorStoredTokenSourceForProfile("acme", localCfg, globalCfg)
	if token != "yaml-token" {
		t.Fatalf("expected yaml-token, got %q", token)
	}
	if raw != "global-config" {
		t.Fatalf("expected raw source 'global-config', got %q", raw)
	}
	if source != "global config file" {
		t.Fatalf("expected source 'global config file', got %q", source)
	}
}

func TestDoctorStoredTokenSourceLocalBeforeGlobal(t *testing.T) {
	localCfg := &config.Config{Token: "local-token"}
	globalCfg := &config.Config{Token: "global-token"}

	savedCreds := creds
	creds = nil
	defer func() { creds = savedCreds }()

	raw, _, token := doctorStoredTokenSourceForProfile("acme", localCfg, globalCfg)
	if token != "local-token" {
		t.Fatalf("expected local-token, got %q", token)
	}
	if raw != "local-config" {
		t.Fatalf("expected raw source 'local-config', got %q", raw)
	}
}

func TestDoctorTokenSourcePrecedence(t *testing.T) {
	SetTestMode()
	defer resetTest()
	localCfg := &config.Config{Token: "local-token"}

	t.Setenv("MAILPIECE_TOKEN", "env-token")
	raw, source, token := doctorTokenSourceWithValue("acme", localCfg, nil)
	if raw != "env" || source != "environment variable" || token != "env-token" {
		t.Fatalf("expected the environment, got %s %q %q", raw, source, token)
	}

	cfgToken = "flag-token"
	raw, source, token = doctorTokenSourceWithValue("acme", localCfg, nil)
	if raw != "flag" || source != "CLI flag" || token != "flag-token" {
		t.Fatalf("expected the flag, got %s %q %q", raw, source, token)
	}
}

func TestDoctorSkillVersionAfterAnUpgrade(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	defer SetVersion("dev")

	SetVersion("1.0.0")
	if _, err := installSkillFiles(); err != nil {
		t.Fatal(err)
	}
	if check := checkDoctorSkillVersion(); check.Status != "pass" || check.Message != "Up to date (1.0.0)" {
		t.Fatalf("expected up to date, got %#v", check)
	}

	SetVersion("2.0.0")
	check := checkDoctorSkillVersion()
	if check.Status != "warn" || check.Message != "Stale (installed: 1.0.0, current: 2.0.0)" {
		t.Fatalf("expected a stale skill, got %#v", check)
	}
	if crumbs := buildDoctorBreadcrumbs([]DoctorCheck{check}); len(crumbs) != 1 || crumbs[0].Cmd != "mailpiece skill install" {
		t.Fatalf("expected the skill install breadcrumb, got %#v", crumbs)
	}
}

func TestBuildDoctorBreadcrumbsDeduplicates(t *testing.T) {
	crumbs := buildDoctorBreadcrumbs([]DoctorCheck{
		{Name: "Credentials", Status: "fail"},
		{Name: "Authentication", Status: "fail"},
		{Name: "API URL", Status: "warn"},
		{Name: "Global Config", Status: "warn"},
		{Name: "CLI Version", Status: "pass"},
	})
	cmds := make([]string, 0, len(crumbs))
	for _, crumb := range crumbs {
		cmds = append(cmds, crumb.Cmd)
	}
	want := []string{"mailpiece auth login <token>", "mailpiece auth status", "mailpiece setup"}
	if strings.Join(cmds, "|") != strings.Join(want, "|") {
		t.Fatalf("breadcrumbs: want %v, got %v", want, cmds)
	}
}

func TestCheckDoctorAPIURL(t *testing.T) {
	tests := []struct {
		url    string
		status string
	}{
		{"https://mailpiece.example.com", "pass"},
		{"http://app.mailpiece.localhost:3020", "warn"},
		{"http://mailpiece.example.com", "fail"},
		{"", "fail"},
	}
	for _, tt := range tests {
		if got := checkDoctorAPIURL(doctorEffectiveConfig{APIURL: tt.url}, false); got.Status != tt.status {
			t.Errorf("checkDoctorAPIURL(%q): want %s, got %#v", tt.url, tt.status, got)
		}
	}
}

func TestCheckDoctorVersionWarnsWhenANewerReleaseExists(t *testing.T) {
	defer SetVersion("dev")
	original := doctorVersionChecker
	defer func() { doctorVersionChecker = original }()
	doctorVersionChecker = func() (string, error) { return "v1.3.0", nil }

	SetVersion("1.2.0")
	check := checkDoctorVersion(false)
	if check.Status != "warn" || check.Message != "1.2.0 (latest: v1.3.0)" || check.Hint == "" {
		t.Fatalf("expected a warning naming the latest release, got %#v", check)
	}

	SetVersion("1.3.0")
	if check := checkDoctorVersion(false); check.Status != "pass" {
		t.Fatalf("expected pass on the latest release, got %#v", check)
	}

	SetVersion("dev")
	doctorVersionChecker = func() (string, error) { t.Fatal("a dev build must not check"); return "", nil }
	if check := checkDoctorVersion(false); check.Status != "pass" {
		t.Fatalf("expected pass on a dev build, got %#v", check)
	}
}
