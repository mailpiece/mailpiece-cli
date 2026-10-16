package commands

import (
	"context"
	stderrors "errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/basecamp/cli/output"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

const (
	testBaseURL   = "https://mailpiece.example.com"
	testAccountID = "12345"
)

// rawRequest is a request a rawAPIServer received.
type rawRequest struct {
	Method        string
	Path          string
	Authorization string
	Body          string
}

// rawAPIServer answers every request with one status and body, and records
// what it was sent, account prefix included.
type rawAPIServer struct {
	*httptest.Server
	mu       sync.Mutex
	requests []rawRequest
}

func newRawAPIServer(t *testing.T, status int, body string) *rawAPIServer {
	t.Helper()
	s := &rawAPIServer{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		path := r.URL.Path
		if r.URL.RawQuery != "" {
			path += "?" + r.URL.RawQuery
		}
		s.mu.Lock()
		s.requests = append(s.requests, rawRequest{Method: r.Method, Path: path, Authorization: r.Header.Get("Authorization"), Body: string(raw)})
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *rawAPIServer) recorded() []rawRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]rawRequest(nil), s.requests...)
}

// setRawAPITest points the SDK and the config at server, on account.
func setRawAPITest(server *rawAPIServer, account string) *CommandResult {
	result := SetTestMode()
	SetTestSDK(server.URL)
	SetTestConfig("test-token", account, server.URL)
	setContextOnAll(context.Background(), rootCmd)
	return result
}

func TestParsePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"subscribers.json", "subscribers.json"},
		{"/subscribers.json", "subscribers.json"},
		{"subscribers/abc/links.json", "subscribers/abc/links.json"},
		{"/subscribers/abc/links.json", "subscribers/abc/links.json"},
		// Same-host absolute URLs: extract the path, dropping the account
		// segment when it matches the configured account.
		{"https://mailpiece.example.com/12345/subscribers.json", "/subscribers.json"},
		{"https://mailpiece.example.com/12345/posts/p7.json", "/posts/p7.json"},
		// A page outside any account keeps its first word.
		{"https://mailpiece.example.com/my/identity.json", "/my/identity.json"},
		// Same host but no account segment: accepted, path used as-is.
		{"https://mailpiece.example.com/subscribers.json", "/subscribers.json"},
		// Same-host URL with no path at all normalizes to the account root.
		{"https://mailpiece.example.com", "/"},
		// Same-host query-only URL normalizes to root and keeps the query.
		{"https://mailpiece.example.com?foo=bar", "/?foo=bar"},
		// A bare "/<account>" strips to the account root path.
		{"https://mailpiece.example.com/12345", "/"},
		{"https://mailpiece.example.com/12345?foo=bar", "/?foo=bar"},
		// Query strings are preserved.
		{"https://mailpiece.example.com/12345/subscribers.json?page=2", "/subscribers.json?page=2"},
		// Schemes are case-insensitive (RFC 3986).
		{"HTTPS://mailpiece.example.com/12345/subscribers.json", "/subscribers.json"},
		// An explicit default port is still the configured host.
		{"https://mailpiece.example.com:443/12345/subscribers.json", "/subscribers.json"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parsePath(tt.input, testBaseURL, testAccountID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestParsePathRejectsForeignAccount guards against silent account
// retargeting: the SDK re-prefixes the configured account.
func TestParsePathRejectsForeignAccount(t *testing.T) {
	for _, input := range []string{
		"https://mailpiece.example.com/999/subscribers.json",
		"https://mailpiece.example.com/acme-news/subscribers.json",
		"https://mailpiece.example.com/999/posts/p7.json",
		"https://mailpiece.example.com/999/subscribers.json?page=2",
		"https://mailpiece.example.com/999",
	} {
		t.Run(input, func(t *testing.T) {
			got, err := parsePath(input, testBaseURL, testAccountID)
			if err == nil || !strings.Contains(err.Error(), "refusing to retarget") {
				t.Fatalf("expected a retarget refusal, got %q, %v", got, err)
			}
			if got != "" {
				t.Errorf("expected no path, got %q", got)
			}
		})
	}

	t.Run("empty configured account strips the segment as before", func(t *testing.T) {
		got, err := parsePath("https://mailpiece.example.com/999/subscribers.json", testBaseURL, "")
		if err != nil || got != "/subscribers.json" {
			t.Errorf("got %q, %v", got, err)
		}
	})
}

// TestParsePathRejectsForeignHosts guards against bearer-token exfiltration: an
// absolute URL whose host differs from the configured base URL is refused
// before it reaches the SDK, which would send the token with it.
func TestParsePathRejectsForeignHosts(t *testing.T) {
	bad := []string{
		"https://evil.com/subscribers.json",
		"http://evil.com/subscribers.json",
		"https://evil.com/",
		"https://evil.com",
		"https://evil.example/999/subscribers.json",
		"http://127.0.0.1:9999/subscribers.json",
		"HTTPS://evil.example/subscribers.json",
		"HtTpS://evil.example/subscribers.json",
		"/https://evil.example/subscribers.json",
		"/HTTPS://evil.example/subscribers.json",
		"//https://evil.example/subscribers.json",
		"///https://evil.example/x",
		"https://mailpiece.example.com:8443/subscribers.json",
		"https://mailpiece.example.com:443.evil.com/subscribers.json",
		"https://mailpiece.example.com:abc/x",
	}

	for _, input := range bad {
		t.Run(input, func(t *testing.T) {
			got, err := parsePath(input, testBaseURL, testAccountID)
			if err == nil {
				t.Fatalf("expected a refusal, got %q", got)
			}
			if got != "" {
				t.Errorf("expected no path, got %q", got)
			}
		})
	}
}

func TestAPIPathArgs(t *testing.T) {
	if err := apiPathArgs(apiGetCmd, nil); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Errorf("no args: got %v", err)
	}
	if err := apiPathArgs(apiGetCmd, []string{"subscribers.json"}); err != nil {
		t.Errorf("one arg: got %v", err)
	}
	if err := apiPathArgs(apiGetCmd, []string{"a", "b"}); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Errorf("two args: got %v", err)
	}
}

func TestAPIGet(t *testing.T) {
	t.Run("reads the path under the configured account", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{"id":"c1","name":"Acme"}`)
		result := setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		err := apiGetCmd.RunE(apiGetCmd, []string{"subscribers/c1.json"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Method != "GET" || reqs[0].Path != "/12345/subscribers/c1.json" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
		if result.Response.Summary != "Acme" {
			t.Errorf("expected the record's name as summary, got %q", result.Response.Summary)
		}
	})

	t.Run("summarizes a list by its count", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `[{"id":"c1"},{"id":"c2"}]`)
		result := setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		err := apiGetCmd.RunE(apiGetCmd, []string{"subscribers.json"})
		assertExitCode(t, err, 0)
		if result.Response.Summary != "2 items" {
			t.Errorf("expected '2 items', got %q", result.Response.Summary)
		}
	})

	t.Run("refuses a URL on another host without a request", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{}`)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		err := apiGetCmd.RunE(apiGetCmd, []string{"https://evil.example/12345/subscribers.json"})
		assertExitCode(t, err, 1)
		if n := len(server.recorded()); n != 0 {
			t.Errorf("expected no request, got %d", n)
		}
	})
}

func TestAPIWrites(t *testing.T) {
	t.Run("post sends the JSON body", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusCreated, `{"id":"s1","title":"Hello"}`)
		result := setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		apiPostCmd.Flags().Set("data", `{"post":{"title":"Hello"}}`)
		defer apiPostCmd.Flags().Set("data", "")
		err := apiPostCmd.RunE(apiPostCmd, []string{"posts.json"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Method != "POST" || reqs[0].Path != "/12345/posts.json" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
		if reqs[0].Body != `{"post":{"title":"Hello"}}` {
			t.Errorf("unexpected body %q", reqs[0].Body)
		}
		if result.Response.Summary != "POST posts.json: Hello" {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
	})

	t.Run("put reads the body from stdin with --data -", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusOK, `{"id":"s1"}`)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		apiPutCmd.SetIn(strings.NewReader(`{"post":{"title":"Hello again"}}`))
		defer apiPutCmd.SetIn(nil)
		apiPutCmd.Flags().Set("data", "-")
		defer apiPutCmd.Flags().Set("data", "")
		err := apiPutCmd.RunE(apiPutCmd, []string{"posts/s1.json"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Method != "PUT" || reqs[0].Body != `{"post":{"title":"Hello again"}}` {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
	})

	t.Run("delete sends no body", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusNoContent, ``)
		result := setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		err := apiDeleteCmd.RunE(apiDeleteCmd, []string{"posts/s1/all_access.json"})
		assertExitCode(t, err, 0)

		reqs := server.recorded()
		if len(reqs) != 1 || reqs[0].Method != "DELETE" || reqs[0].Path != "/12345/posts/s1/all_access.json" {
			t.Fatalf("unexpected requests: %#v", reqs)
		}
		if result.Response.Summary != "DELETE posts/s1/all_access.json" {
			t.Errorf("unexpected summary %q", result.Response.Summary)
		}
	})

	t.Run("a read token's write carries the read-token hint", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusUnauthorized, ``)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		apiPostCmd.Flags().Set("data", `{}`)
		defer apiPostCmd.Flags().Set("data", "")
		err := apiPostCmd.RunE(apiPostCmd, []string{"posts.json"})
		assertExitCode(t, err, 3)
		var outErr *output.Error
		if !stderrors.As(err, &outErr) || outErr.Hint != mailpiece.ReadOnlyTokenHint {
			t.Errorf("expected the read-token hint, got %v", err)
		}
	})

	t.Run("requires --data", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusCreated, `{}`)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		err := apiPostCmd.RunE(apiPostCmd, []string{"posts.json"})
		assertExitCode(t, err, 1)
		if n := len(server.recorded()); n != 0 {
			t.Errorf("expected no request, got %d", n)
		}
	})

	t.Run("refuses invalid JSON without a request", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusCreated, `{}`)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		apiPostCmd.Flags().Set("data", `{not json`)
		defer apiPostCmd.Flags().Set("data", "")
		err := apiPostCmd.RunE(apiPostCmd, []string{"posts.json"})
		assertExitCode(t, err, 1)
		if n := len(server.recorded()); n != 0 {
			t.Errorf("expected no request, got %d", n)
		}
	})

	t.Run("refuses a foreign host before reading stdin", func(t *testing.T) {
		server := newRawAPIServer(t, http.StatusCreated, `{}`)
		setRawAPITest(server, testAccountID)
		defer ResetTestMode()

		stdin := strings.NewReader(`{"post":{}}`)
		apiPostCmd.SetIn(stdin)
		defer apiPostCmd.SetIn(nil)
		apiPostCmd.Flags().Set("data", "-")
		defer apiPostCmd.Flags().Set("data", "")
		err := apiPostCmd.RunE(apiPostCmd, []string{"https://evil.example/posts.json"})
		assertExitCode(t, err, 1)
		if stdin.Len() == 0 {
			t.Error("stdin was drained for a request that cannot be sent")
		}
		if n := len(server.recorded()); n != 0 {
			t.Errorf("expected no request, got %d", n)
		}
	})
}

func TestAPISummary(t *testing.T) {
	cases := []struct {
		data any
		want string
	}{
		{[]map[string]any{{"id": "1"}}, "1 items"},
		{map[string]any{"name": "Acme"}, "Acme"},
		{map[string]any{"title": "First issue", "name": "x"}, "First issue"},
		{map[string]any{"name": strings.Repeat("ñ", 60)}, strings.Repeat("ñ", 47) + "..."},
		{map[string]any{"id": "1"}, "API response"},
		{nil, "API response"},
	}
	for _, tc := range cases {
		if got := apiSummary(tc.data); got != tc.want {
			t.Errorf("apiSummary(%v) = %q, want %q", tc.data, got, tc.want)
		}
	}
}
