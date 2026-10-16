package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/basecamp/cli/profile"
	"github.com/mailpiece/mailpiece-cli/internal/config"
	"gopkg.in/yaml.v3"
)

// newTestSessionServer creates an httptest server that simulates the Mailpiece
// sign-in API. It tracks state across requests via cookies, mirroring the
// Rails signed cookie flow.
func newTestSessionServer(t *testing.T, opts testSessionServerOpts) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// POST /session.json: send the one-time code
	mux.HandleFunc("POST /session.json", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:  "pending_authentication_token",
			Value: "signed-pending-token-value",
			Path:  "/",
		})
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Magic-Link-Code", "TEST01")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"pending_authentication_token": "raw-pending-token-value",
		})
	})

	// POST /session/magic_link.json: verify the code
	mux.HandleFunc("POST /session/magic_link.json", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)

		code, _ := body["code"].(string)
		if code != "VALID1" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]any{
				"message": "Try another code.",
			})
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:  "session_token",
			Value: "signed-session-token-value",
			Path:  "/",
		})
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"session_token":              "raw-session-token-value",
			"requires_signup_completion": opts.requiresCompletion,
		})
	})

	// GET /my/identity.json: the identity's accounts
	mux.HandleFunc("GET /my/identity.json", func(w http.ResponseWriter, r *http.Request) {
		if !hasSessionCookie(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		accounts := opts.accounts
		if accounts == nil {
			accounts = []any{
				map[string]any{"id": "acct-1", "name": "Test Account", "slug": "/123456"},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"id": "identity-1", "accounts": accounts})
	})

	// POST /signup/completion.json: finish a new identity's signup
	mux.HandleFunc("POST /signup/completion.json", func(w http.ResponseWriter, r *http.Request) {
		if !hasSessionCookie(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if opts.signups != nil {
			opts.signups.record(r.URL.Path, body)
		}
		if signup, _ := body["signup"].(map[string]any); strings.TrimSpace(fmt.Sprint(signup["full_name"])) == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]any{"errors": []string{"Full name can't be blank"}})
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	// POST /{account}/my/access_tokens.json: generate the access token
	mux.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/my/access_tokens.json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if !hasSessionCookie(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		// Verify the URL doesn't have double slashes (slug normalization)
		if strings.Contains(r.URL.Path, "//") {
			t.Errorf("URL contains double slash: %s", r.URL.Path)
		}

		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if opts.requests != nil {
			opts.requests.record(r.URL.Path, body)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{
			"id":          "token-1",
			"token":       opts.accessToken,
			"description": "Mailpiece CLI",
			"permission":  "read",
		})
	})

	return httptest.NewServer(mux)
}

type testSessionServerOpts struct {
	requiresCompletion bool
	accessToken        string
	accounts           []any
	requests           *tokenRequests
	signups            *tokenRequests
}

// tokenRequests records the access token requests a session server receives.
type tokenRequests struct {
	mu     sync.Mutex
	paths  []string
	bodies []map[string]any
}

func (r *tokenRequests) record(path string, body map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, path)
	r.bodies = append(r.bodies, body)
}

// hasSessionCookie is true when the request carries the session cookie the
// server set, so a token read from stdin is the one that signs the request.
func hasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	return err == nil && cookie.Value == "signed-session-token-value"
}

// pipeSessionToken replaces os.Stdin with a pipe containing the session token.
// Returns a cleanup function that restores the original stdin.
func pipeSessionToken(token string) func() {
	r, w, _ := os.Pipe()
	w.WriteString(token + "\n")
	w.Close()
	origStdin := os.Stdin
	os.Stdin = r
	return func() {
		os.Stdin = origStdin
		r.Close()
	}
}

// resetAuthEmailFlags clears cobra flag values between test runs to prevent cross-contamination.
func resetAuthEmailFlags() {
	authEmailStartCmd.Flags().Set("email", "")
	authEmailVerifyCmd.Flags().Set("code", "")
	authEmailVerifyCmd.Flags().Set("pending-token", "")
	authEmailCompleteCmd.Flags().Set("account", "")
	authEmailCompleteCmd.Flags().Set("write", "false")
}

func TestAuthEmailStart(t *testing.T) {
	server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
	defer server.Close()

	t.Run("returns signed cookie value not JSON body value", func(t *testing.T) {
		resetAuthEmailFlags()
		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		authEmailStartCmd.Flags().Set("email", "test@example.com")
		err := authEmailStartCmd.RunE(authEmailStartCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		pendingToken := data["pending_authentication_token"].(string)

		if pendingToken != "signed-pending-token-value" {
			t.Errorf("expected signed cookie value 'signed-pending-token-value', got '%s'", pendingToken)
		}
		if pendingToken == "raw-pending-token-value" {
			t.Error("got raw JSON value instead of signed cookie value")
		}

		// Development servers include the one-time code
		code, _ := data["code"].(string)
		if code != "TEST01" {
			t.Errorf("expected code 'TEST01', got '%s'", code)
		}
		if result.Response.Summary != "Code sent. Check your email." {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
	})
}

func TestAuthEmailVerify(t *testing.T) {
	t.Run("returns signed session cookie", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		authEmailVerifyCmd.Flags().Set("code", "VALID1")
		authEmailVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := authEmailVerifyCmd.RunE(authEmailVerifyCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)

		// Session token should be signed cookie value
		sessionToken := data["session_token"].(string)
		if sessionToken != "signed-session-token-value" {
			t.Errorf("expected signed cookie value 'signed-session-token-value', got '%s'", sessionToken)
		}
		if sessionToken == "raw-session-token-value" {
			t.Error("got raw JSON value instead of signed cookie value")
		}
		if data["requires_signup_completion"] != false {
			t.Errorf("expected requires_signup_completion=false, got %v", data["requires_signup_completion"])
		}

		accounts, _ := data["accounts"].([]any)
		if len(accounts) != 1 || accounts[0].(map[string]any)["slug"] != "123456" {
			t.Errorf("expected the one account with its slug normalized, got %#v", data["accounts"])
		}

		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || crumbs[0].Cmd != "mailpiece auth email complete --account 123456" {
			t.Errorf("expected the complete breadcrumb, got %#v", crumbs)
		}
		if !strings.Contains(crumbs[0].Description, "stdin") {
			t.Errorf("expected the breadcrumb to say the session token goes on stdin, got %q", crumbs[0].Description)
		}
	})

	t.Run("points a new identity to signup complete", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			requiresCompletion: true,
			accessToken:        "mailpiece_test",
		})
		defer server.Close()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		authEmailVerifyCmd.Flags().Set("code", "VALID1")
		authEmailVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := authEmailVerifyCmd.RunE(authEmailVerifyCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["requires_signup_completion"] != true {
			t.Errorf("expected requires_signup_completion=true, got %v", data["requires_signup_completion"])
		}
		if _, ok := data["accounts"]; ok {
			t.Error("a new identity reaches no account yet")
		}
		crumbs := result.Response.Breadcrumbs
		if len(crumbs) != 1 || !strings.HasPrefix(crumbs[0].Cmd, "mailpiece signup complete --name") {
			t.Errorf("expected the signup complete breadcrumb, got %#v", crumbs)
		}
	})

	t.Run("fails with invalid code", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		authEmailVerifyCmd.Flags().Set("code", "BADCODE")
		authEmailVerifyCmd.Flags().Set("pending-token", "signed-pending-token-value")
		err := authEmailVerifyCmd.RunE(authEmailVerifyCmd, []string{})

		assertExitCode(t, err, 7)
		if err != nil && !strings.Contains(err.Error(), "Try another code.") {
			t.Errorf("expected the app's message, got %q", err.Error())
		}
	})
}

func TestAuthEmailComplete(t *testing.T) {
	t.Run("reads the session token from stdin and saves the token under the account's profile", func(t *testing.T) {
		resetAuthEmailFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_generated_token",
			requests:    requests,
		})
		defer server.Close()

		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()
		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		result := SetTestMode()
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "123456")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["authenticated"] != true {
			t.Errorf("expected authenticated=true, got %v", data["authenticated"])
		}
		if data["account"] != "123456" {
			t.Errorf("expected account '123456', got %v", data["account"])
		}
		if data["permission"] != "read" {
			t.Errorf("expected permission 'read', got %v", data["permission"])
		}
		if _, ok := data["token"]; ok {
			t.Error("the generated token must not be printed")
		}

		if len(requests.paths) != 1 || requests.paths[0] != "/123456/my/access_tokens.json" {
			t.Fatalf("expected one token request for /123456, got %v", requests.paths)
		}
		accessToken, _ := requests.bodies[0]["access_token"].(map[string]any)
		if accessToken["permission"] != "read" || accessToken["description"] != "Mailpiece CLI" {
			t.Errorf("unexpected access token request: %#v", requests.bodies[0])
		}

		loaded, err := credsLoadProfileToken("123456")
		if err != nil || loaded != "mailpiece_generated_token" {
			t.Fatalf("expected the generated token under profile 123456, got %q (%v)", loaded, err)
		}
		p, err := profileStore.Get("123456")
		if err != nil {
			t.Fatalf("expected profile '123456': %v", err)
		}
		if p.BaseURL != server.URL {
			t.Errorf("expected profile base_url %q, got %q", server.URL, p.BaseURL)
		}
	})

	t.Run("--write asks for a token that can write", func(t *testing.T) {
		resetAuthEmailFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_write_token",
			requests:    requests,
		})
		defer server.Close()

		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "123456")
		authEmailCompleteCmd.Flags().Set("write", "true")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		if data := result.Response.Data.(map[string]any); data["permission"] != "write" {
			t.Errorf("expected permission 'write', got %v", data["permission"])
		}
		accessToken, _ := requests.bodies[0]["access_token"].(map[string]any)
		if accessToken["permission"] != "write" {
			t.Errorf("expected a write token request, got %#v", requests.bodies[0])
		}
	})

	t.Run("saves the token to the global config without a credential store", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_generated_token",
		})
		defer server.Close()

		tempDir := t.TempDir()
		config.SetTestConfigDir(tempDir)
		defer config.ResetTestConfigDir()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "123456")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		// Verify config was saved
		configData, err := os.ReadFile(filepath.Join(tempDir, "config.yaml"))
		if err != nil {
			t.Fatalf("config file not created: %v", err)
		}

		var savedConfig config.Config
		yaml.Unmarshal(configData, &savedConfig)

		if savedConfig.Token != "mailpiece_generated_token" {
			t.Errorf("expected saved token 'mailpiece_generated_token', got '%s'", savedConfig.Token)
		}
		if savedConfig.Account != "123456" {
			t.Errorf("expected saved account '123456', got '%s'", savedConfig.Account)
		}
		if savedConfig.APIURL != server.URL {
			t.Errorf("expected saved api_url %q, got '%s'", server.URL, savedConfig.APIURL)
		}
	})

	t.Run("--profile names the profile the token is saved under", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_agent_token"})
		defer server.Close()

		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()
		store := newTestCredStore(t, t.TempDir())
		profileStore := profile.NewStore(filepath.Join(t.TempDir(), "config.json"))

		SetTestMode()
		SetTestCreds(store)
		SetTestProfiles(profileStore)
		SetTestConfig("", "", server.URL)
		cfgProfile = "agent"
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "123456")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		p, err := profileStore.Get("agent")
		if err != nil {
			t.Fatalf("expected profile 'agent': %v", err)
		}
		if account := profileAccount("agent", p); account != "123456" {
			t.Errorf("expected profile account '123456', got %q", account)
		}
		if loaded, err := credsLoadProfileToken("agent"); err != nil || loaded != "mailpiece_agent_token" {
			t.Errorf("expected the token under profile agent, got %q (%v)", loaded, err)
		}
	})

	t.Run("strips leading slash from account flag", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_test_token",
		})
		defer server.Close()

		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "/123456")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		data := result.Response.Data.(map[string]any)
		if data["account"] != "123456" {
			t.Errorf("expected normalized account '123456', got '%s'", data["account"])
		}
	})

	t.Run("takes the only account the identity reaches", func(t *testing.T) {
		resetAuthEmailFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_test_token",
			requests:    requests,
		})
		defer server.Close()

		config.SetTestConfigDir(t.TempDir())
		defer config.ResetTestConfigDir()

		result := SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 0)

		if data := result.Response.Data.(map[string]any); data["account"] != "123456" {
			t.Errorf("expected account '123456', got %v", data["account"])
		}
		if len(requests.paths) != 1 || requests.paths[0] != "/123456/my/access_tokens.json" {
			t.Errorf("expected one token request for /123456, got %v", requests.paths)
		}
	})

	t.Run("asks for --account when the identity reaches several", func(t *testing.T) {
		resetAuthEmailFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_test",
			requests:    requests,
			accounts: []any{
				map[string]any{"id": "acct-1", "name": "Test Account", "slug": "/123456"},
				map[string]any{"id": "acct-2", "name": "Other Account", "slug": "/789012"},
			},
		})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 1)
		if len(requests.paths) != 0 {
			t.Errorf("expected no token request, got %v", requests.paths)
		}
	})

	t.Run("refuses an identity that reaches no account", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{
			accessToken: "mailpiece_test",
			accounts:    []any{},
		})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 7)
	})

	t.Run("rejects an unsafe account before asking for a token", func(t *testing.T) {
		resetAuthEmailFlags()
		requests := &tokenRequests{}
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test", requests: requests})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("signed-session-token-value")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "../other")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 1)
		if len(requests.paths) != 0 {
			t.Errorf("expected no token request, got %v", requests.paths)
		}
	})

	t.Run("requires a session token on stdin", func(t *testing.T) {
		resetAuthEmailFlags()
		server := newTestSessionServer(t, testSessionServerOpts{accessToken: "mailpiece_test"})
		defer server.Close()

		SetTestMode()
		SetTestConfig("", "", server.URL)
		setContextOnAll(context.Background(), rootCmd)
		defer ResetTestMode()

		restoreStdin := pipeSessionToken("")
		defer restoreStdin()

		authEmailCompleteCmd.Flags().Set("account", "123456")
		err := authEmailCompleteCmd.RunE(authEmailCompleteCmd, []string{})
		assertExitCode(t, err, 1)
	})

	t.Run("has no session token flag", func(t *testing.T) {
		if authEmailCompleteCmd.Flags().Lookup("session-token") != nil {
			t.Error("--session-token lands in shell history; the token is read from stdin")
		}
	})
}

func TestAuthEmailWizardRequiresATerminal(t *testing.T) {
	SetTestMode()
	SetTestConfig("", "", testAPIURL)
	cfgAgent = true
	defer ResetTestMode()

	err := authEmailCmd.RunE(authEmailCmd, []string{})
	assertExitCode(t, err, 1)
}
