package mailpiece

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := NewClient(&Config{BaseURL: "https://api.example.com", Account: "acme"}, &StaticTokenProvider{Token: "token123"})

	if c.Config().BaseURL != "https://api.example.com" {
		t.Errorf("expected BaseURL 'https://api.example.com', got '%s'", c.Config().BaseURL)
	}
	if c.Config().Account != "acme" {
		t.Errorf("expected Account 'acme', got '%s'", c.Config().Account)
	}
	if c.userAgent != DefaultUserAgent {
		t.Errorf("expected User-Agent %q, got %q", DefaultUserAgent, c.userAgent)
	}
	if c.httpOpts != DefaultHTTPOptions() {
		t.Errorf("expected default HTTP options, got %+v", c.httpOpts)
	}
	if c.httpClient == nil || c.httpClient.Timeout != DefaultTimeout {
		t.Error("expected an HTTP client with the default timeout")
	}
	if _, ok := c.authStrategy.(*BearerAuth); !ok {
		t.Errorf("expected BearerAuth by default, got %T", c.authStrategy)
	}
	if _, ok := c.hooks.(NoopHooks); !ok {
		t.Errorf("expected NoopHooks by default, got %T", c.hooks)
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := NewClient(&Config{BaseURL: "https://api.example.com/"}, &StaticTokenProvider{Token: "token"})

	if c.Config().BaseURL != "https://api.example.com" {
		t.Errorf("expected BaseURL without trailing slash, got '%s'", c.Config().BaseURL)
	}
}

func TestNew_CopiesConfig(t *testing.T) {
	cfg := &Config{BaseURL: "https://api.example.com", Account: "acme"}
	c := NewClient(cfg, &StaticTokenProvider{Token: "token"})
	cfg.BaseURL = "https://changed.example.com"

	if c.Config().BaseURL != "https://api.example.com" {
		t.Errorf("the client should keep its own copy of the config, got %q", c.Config().BaseURL)
	}
}

func TestDefaultHTTPOptions(t *testing.T) {
	opts := DefaultHTTPOptions()
	if opts.Timeout != 30*time.Second || opts.MaxRetries != 3 || opts.BaseDelay != time.Second ||
		opts.MaxJitter != 100*time.Millisecond || opts.MaxPages != 10000 || opts.Transport != nil {
		t.Fatalf("got %+v", opts)
	}
}

func TestClientOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&strings.Builder{}, nil))
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"},
		WithUserAgent("mailpiece-cli/test"),
		WithTimeout(5*time.Second),
		WithMaxRetries(7),
		WithBaseDelay(2*time.Second),
		WithMaxJitter(time.Millisecond),
		WithMaxPages(4),
		WithLogger(logger),
	)

	if c.userAgent != "mailpiece-cli/test" {
		t.Errorf("userAgent = %q", c.userAgent)
	}
	if c.httpClient.Timeout != 5*time.Second {
		t.Errorf("timeout = %v", c.httpClient.Timeout)
	}
	if c.httpOpts.MaxRetries != 7 || c.httpOpts.BaseDelay != 2*time.Second || c.httpOpts.MaxJitter != time.Millisecond || c.httpOpts.MaxPages != 4 {
		t.Errorf("httpOpts = %+v", c.httpOpts)
	}
	if c.logger != logger {
		t.Error("WithLogger should set the logger")
	}

	kept := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"}, WithLogger(nil), WithHooks(nil))
	if kept.logger == nil {
		t.Error("WithLogger(nil) should keep the discarding logger")
	}
	if _, ok := kept.hooks.(NoopHooks); !ok {
		t.Errorf("WithHooks(nil) should mean NoopHooks, got %T", kept.hooks)
	}
}

func TestWithHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: 7 * time.Second}
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"}, WithHTTPClient(custom))
	if c.httpClient != custom {
		t.Fatal("WithHTTPClient should set the HTTP client")
	}
}

func TestWithTransport(t *testing.T) {
	var sawRequest bool
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		sawRequest = true
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.WriteString(`{"id":"s1"}`)
		return rec.Result(), nil
	})
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"}, WithTransport(transport))

	resp, err := c.Get(context.Background(), "/acme/posts/s1.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawRequest || string(resp.Data) != `{"id":"s1"}` {
		t.Fatalf("the custom transport should carry the request, got %q", resp.Data)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNewClientPanics(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		opts []ClientOption
	}{
		{"plain HTTP off localhost", Config{BaseURL: "http://mailpiece.example.com"}, nil},
		{"zero timeout", Config{BaseURL: "https://mailpiece.example.com"}, []ClientOption{WithTimeout(0)}},
		{"negative retries", Config{BaseURL: "https://mailpiece.example.com"}, []ClientOption{WithMaxRetries(-1)}},
		{"zero pages", Config{BaseURL: "https://mailpiece.example.com"}, []ClientOption{WithMaxPages(0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("expected NewClient to panic")
				}
			}()
			NewClient(&tc.cfg, &StaticTokenProvider{Token: "token"}, tc.opts...)
		})
	}
}

func TestNewClientAcceptsLocalhostOverHTTP(t *testing.T) {
	for _, base := range []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://app.localhost"} {
		c := NewClient(&Config{BaseURL: base}, &StaticTokenProvider{Token: "token"})
		if c.Config().BaseURL != base {
			t.Errorf("BaseURL = %q, want %q", c.Config().BaseURL, base)
		}
	}
}

func TestForAccount(t *testing.T) {
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"})

	if got := c.ForAccount("/834206594").AccountID(); got != "834206594" {
		t.Errorf("AccountID = %q, want the slug without its slash", got)
	}
	for _, slug := range []string{"", "/"} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("ForAccount(%q) should panic", slug)
				}
			}()
			c.ForAccount(slug)
		}()
	}
}

func TestBuildURL(t *testing.T) {
	ac := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"}).ForAccount("account123")

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "simple path",
			path:     "/posts.json",
			expected: "https://api.example.com/account123/posts.json",
		},
		{
			name:     "path without leading slash",
			path:     "posts.json",
			expected: "https://api.example.com/account123/posts.json",
		},
		{
			name:     "full URL",
			path:     "https://other.api.com/resource",
			expected: "https://other.api.com/resource",
		},
		{
			name:     "path with account already",
			path:     "/account123/posts.json",
			expected: "https://api.example.com/account123/posts.json",
		},
		{
			name:     "the account alone",
			path:     "/account123",
			expected: "https://api.example.com/account123",
		},
		{
			name:     "the account with a query",
			path:     "/account123?page=2",
			expected: "https://api.example.com/account123?page=2",
		},
		{
			name:     "a path that only starts like the account",
			path:     "/account1234/posts.json",
			expected: "https://api.example.com/account123/account1234/posts.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ac.parent.buildURL(ac.accountPath(tt.path))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestBuildURL_NoAccount(t *testing.T) {
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"})

	result, err := c.buildURL("my/identity.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "https://api.example.com/my/identity.json" {
		t.Errorf("expected path without account, got '%s'", result)
	}
}

func TestBuildURL_PlainHTTP(t *testing.T) {
	c := NewClient(&Config{BaseURL: "https://api.example.com"}, &StaticTokenProvider{Token: "token"})

	if _, err := c.buildURL("http://example.com/file"); err == nil {
		t.Error("a plain HTTP URL off localhost should be refused")
	}
	if got, err := c.buildURL("http://localhost:3000/file"); err != nil || got != "http://localhost:3000/file" {
		t.Errorf("a localhost URL should pass, got %q, %v", got, err)
	}
}

func TestGet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/acme/posts/s1.json" {
			t.Errorf("expected the account's path, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Authorization header, got '%s'", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("expected Accept header, got '%s'", r.Header.Get("Accept"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type header, got '%s'", r.Header.Get("Content-Type"))
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "req-1")
		_, _ = w.Write([]byte(`{"id":"s1","name":"Hello"}`))
	}))
	defer server.Close()

	resp, err := sdkAccount(server.URL).Get(context.Background(), "/posts/s1.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if resp.Headers.Get("X-Request-Id") != "req-1" {
		t.Errorf("expected the response headers, got %v", resp.Headers)
	}

	var post struct{ ID, Name string }
	if err := resp.UnmarshalData(&post); err != nil {
		t.Fatalf("UnmarshalData: %v", err)
	}
	if post.ID != "s1" || post.Name != "Hello" {
		t.Errorf("got %+v", post)
	}
}

func TestPost(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusCreated, `{"id":"s1"}`))

	resp, err := sdkAccount(srv.URL).Post(context.Background(), "/posts.json", map[string]any{"post": map[string]string{"name": "Hello"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected 201, got %d", resp.StatusCode)
	}
	reqs := srv.recorded()
	if len(reqs) != 1 || reqs[0].Method != "POST" || reqs[0].URI != "/acme/posts.json" {
		t.Fatalf("got %+v", reqs)
	}
	sdkAssertJSON(t, reqs[0].Body, `{"post":{"name":"Hello"}}`)
}

func TestPatch(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))

	if _, err := sdkAccount(srv.URL).Patch(context.Background(), "/posts/s1.json", map[string]any{"post": map[string]string{"name": "Hello again"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqs := srv.recorded()
	if len(reqs) != 1 || reqs[0].Method != "PATCH" || reqs[0].URI != "/acme/posts/s1.json" {
		t.Fatalf("got %+v", reqs)
	}
	sdkAssertJSON(t, reqs[0].Body, `{"post":{"name":"Hello again"}}`)
}

func TestPut(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))

	if _, err := sdkAccount(srv.URL).Put(context.Background(), "/posts/s1.json", map[string]string{"name": "Third"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	reqs := srv.recorded()
	if len(reqs) != 1 || reqs[0].Method != "PUT" || reqs[0].URI != "/acme/posts/s1.json" {
		t.Fatalf("got %+v", reqs)
	}
	sdkAssertJSON(t, reqs[0].Body, `{"name":"Third"}`)
}

func TestDelete(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	resp, err := sdkAccount(srv.URL).Delete(context.Background(), "/posts/s1.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent || string(resp.Data) != "null" {
		t.Errorf("a 204 should read as null, got %d %q", resp.StatusCode, resp.Data)
	}
	reqs := srv.recorded()
	if len(reqs) != 1 || reqs[0].Method != "DELETE" || reqs[0].URI != "/acme/posts/s1.json" || len(reqs[0].Body) != 0 {
		t.Fatalf("got %+v", reqs)
	}
}

func TestUserAgentHeader(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))

	_, _ = sdkClient(srv.URL).Get(context.Background(), "/my/identity.json")
	_, _ = sdkClient(srv.URL, WithUserAgent("mailpiece-cli/1.0")).Get(context.Background(), "/my/identity.json")

	reqs := srv.recorded()
	if got := reqs[0].Header.Get("User-Agent"); got != "mailpiece-sdk-go/"+Version {
		t.Errorf("expected the default User-Agent, got '%s'", got)
	}
	if got := reqs[1].Header.Get("User-Agent"); got != "mailpiece-cli/1.0" {
		t.Errorf("expected User-Agent 'mailpiece-cli/1.0', got '%s'", got)
	}
}

type headerAuth struct{}

func (headerAuth) Authenticate(_ context.Context, req *http.Request) error {
	req.Header.Set("X-Api-Key", "k")
	return nil
}

func TestAuthStrategy(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))

	if _, err := sdkClient(srv.URL, WithAuthStrategy(headerAuth{})).Get(context.Background(), "/my/identity.json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h := srv.recorded()[0].Header
	if h.Get("X-Api-Key") != "k" || h.Get("Authorization") != "" {
		t.Errorf("a custom strategy replaces the bearer token, got %v", h)
	}
}

type failingTokenProvider struct{}

func (failingTokenProvider) AccessToken(context.Context) (string, error) {
	return "", errors.New("no token stored")
}

func TestTokenProviderErrorSendsNothing(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))
	c := NewClient(&Config{BaseURL: srv.URL}, failingTokenProvider{})

	if _, err := c.Get(context.Background(), "/my/identity.json"); err == nil || !strings.Contains(err.Error(), "no token stored") {
		t.Fatalf("expected the provider's error, got %v", err)
	}
	if _, err := c.Post(context.Background(), "/my/access_tokens.json", nil); err == nil {
		t.Fatal("expected the provider's error on a POST")
	}
	if n := len(srv.recorded()); n != 0 {
		t.Errorf("sent %d requests without a token", n)
	}
}

func TestErrorResponses(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		statusCode   int
		responseBody string
		expectedCode string
		expectedExit int
		message      string
		hint         string
	}{
		{"401 on a read", "GET", 401, `{"error":"Invalid token"}`, CodeAuth, ExitAuth, "Authentication failed", ""},
		{"401 on a POST names the read token", "POST", 401, ``, CodeAuth, ExitAuth, "Authentication failed", ReadOnlyTokenHint},
		{"401 on a PATCH names the read token", "PATCH", 401, ``, CodeAuth, ExitAuth, "Authentication failed", ReadOnlyTokenHint},
		{"401 on a PUT names the read token", "PUT", 401, ``, CodeAuth, ExitAuth, "Authentication failed", ReadOnlyTokenHint},
		{"401 on a DELETE names the read token", "DELETE", 401, ``, CodeAuth, ExitAuth, "Authentication failed", ReadOnlyTokenHint},
		{"403 on a read", "GET", 403, `{"error":"Access denied"}`, CodeForbidden, ExitForbidden, "Access denied", ""},
		{"403 on a write carries no read-token hint", "POST", 403, ``, CodeForbidden, ExitForbidden, "Access denied", ""},
		{"403 with a reason says it", "POST", 403, `{"error":"This post cannot go to that destination"}`, CodeForbidden, ExitForbidden, "This post cannot go to that destination", ""},
		{"403 with a message says it", "DELETE", 403, `{"message":"Sending can't be stopped for an address that unsubscribed or bounced."}`, CodeForbidden, ExitForbidden, "Sending can't be stopped for an address that unsubscribed or bounced.", ""},
		{"403 with a body that gives no reason", "POST", 403, `{"email":["is taken"]}`, CodeForbidden, ExitForbidden, "Access denied", ""},
		{"404", "GET", 404, `{"error":"Not found"}`, CodeNotFound, ExitNotFound, "Resource not found: ", ""},
		{"422 with field errors", "POST", 422, `{"name":["can't be blank"],"title":["is invalid"]}`, CodeValidation, ExitValidation, "name: can't be blank; title: is invalid", ""},
		{"422 with an error", "PATCH", 422, `{"error":"Validation failed"}`, CodeValidation, ExitValidation, "Validation failed", ""},
		{"429", "GET", 429, ``, CodeRateLimit, ExitRateLimit, "Rate limited", ""},
		{"500", "GET", 500, `{"error":"Internal server error"}`, CodeAPI, ExitAPI, "Server error (500)", ""},
		{"502", "GET", 502, ``, CodeAPI, ExitAPI, "Gateway error (502)", ""},
		{"503", "GET", 503, ``, CodeAPI, ExitAPI, "Gateway error (503)", ""},
		{"504", "GET", 504, ``, CodeAPI, ExitAPI, "Gateway error (504)", ""},
		{"another status with an error", "GET", 418, `{"error":"I'm a teapot"}`, CodeAPI, ExitAPI, "I'm a teapot", "The API answered HTTP 418 to GET "},
		{"another status with a message", "POST", 400, `{"message":"Bad request body"}`, CodeAPI, ExitAPI, "Bad request body", "The API answered HTTP 400 to POST "},
		{"another status with no body", "DELETE", 409, ``, CodeAPI, ExitAPI, "Request failed (HTTP 409)", "The API answered HTTP 409 to DELETE "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			c := sdkClient(server.URL, WithMaxRetries(1))
			var err error
			switch tt.method {
			case "GET":
				_, err = c.Get(context.Background(), "/acme/resource.json")
			case "POST":
				_, err = c.Post(context.Background(), "/acme/resource.json", map[string]string{"name": "x"})
			case "PATCH":
				_, err = c.Patch(context.Background(), "/acme/resource.json", map[string]string{"name": "x"})
			case "PUT":
				_, err = c.Put(context.Background(), "/acme/resource.json", map[string]string{"name": "x"})
			case "DELETE":
				_, err = c.Delete(context.Background(), "/acme/resource.json")
			}

			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var apiErr *Error
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected *Error, got %T", err)
			}

			if apiErr.Code != tt.expectedCode {
				t.Errorf("expected code '%s', got '%s'", tt.expectedCode, apiErr.Code)
			}
			if apiErr.ExitCode() != tt.expectedExit {
				t.Errorf("expected exit code %d, got %d", tt.expectedExit, apiErr.ExitCode())
			}
			if apiErr.HTTPStatus != tt.statusCode {
				t.Errorf("expected HTTP status %d, got %d", tt.statusCode, apiErr.HTTPStatus)
			}
			if !strings.HasPrefix(apiErr.Message, tt.message) {
				t.Errorf("expected message '%s', got '%s'", tt.message, apiErr.Message)
			}
			if apiErr.Hint != tt.hint && (tt.hint == "" || !strings.HasPrefix(apiErr.Hint, tt.hint)) {
				t.Errorf("expected hint '%s', got '%s'", tt.hint, apiErr.Hint)
			}
		})
	}
}

func TestErrorResponse_UnhandledStatusNamesWhatWasAsked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotAcceptable)
		_, _ = w.Write([]byte(`{"status":406,"error":"Not Acceptable"}`))
	}))
	defer server.Close()

	_, err := sdkClient(server.URL).Get(context.Background(), "/834206594/my/identity.json?signature=secret")
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %v", err)
	}
	if apiErr.Message != "Not Acceptable" || apiErr.HTTPStatus != 406 {
		t.Errorf("expected the API's own words and status, got %q (%d)", apiErr.Message, apiErr.HTTPStatus)
	}
	want := "The API answered HTTP 406 to GET " + server.URL + "/834206594/my/identity.json."
	if apiErr.Hint != want {
		t.Errorf("hint = %q, want %q", apiErr.Hint, want)
	}
}

func TestErrorResponse_RequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-42")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	_, err := sdkClient(server.URL).Get(context.Background(), "/acme/posts.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.RequestID != "req-42" {
		t.Fatalf("expected the request id on the error, got %v", err)
	}
}

func TestErrorResponse_LongMessageIsTruncated(t *testing.T) {
	long := strings.Repeat("x", 2*MaxErrorMessageBytes)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = fmt.Fprintf(w, `{"error":%q}`, long)
	}))
	defer server.Close()

	_, err := sdkClient(server.URL).Get(context.Background(), "/acme/posts.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || len(apiErr.Message) != MaxErrorMessageBytes || !strings.HasSuffix(apiErr.Message, "...") {
		t.Fatalf("expected a message cut to %d bytes, got %v", MaxErrorMessageBytes, err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected int
	}{
		{"empty", "", 0},
		{"seconds", "5", 5},
		{"capped", "600", 300},
		{"overflow", "9227000000", 300},
		{"zero", "0", 0},
		{"negative", "-1", 0},
		{"invalid", "not-a-number", 0},
		{"a date in the past", "Wed, 21 Oct 2015 07:28:00 GMT", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseRetryAfter(tt.value); got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}

	soon := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(soon); got < 100 || got > 120 {
		t.Errorf("a date two minutes ahead should be about 120 seconds, got %d", got)
	}

	later := time.Now().Add(time.Hour).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(later); got != 300 {
		t.Errorf("a date an hour ahead should be capped at 300 seconds, got %d", got)
	}
}

// --- retries ---

func TestRetryOn5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
			t.Run(fmt.Sprintf("%s %d", method, status), func(t *testing.T) {
				var attempts atomic.Int32
				srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
					if attempts.Add(1) <= 1 {
						w.WriteHeader(status)
						return
					}
					sdkReply(http.StatusOK, `{"id":"1"}`)(w, r)
				})

				hooks := &retryRecordingHooks{}
				c := sdkClient(srv.URL, WithHooks(hooks))
				resp, err := sdkSend(c, method)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if resp.StatusCode != 200 {
					t.Errorf("expected 200, got %d", resp.StatusCode)
				}
				if attempts.Load() != 2 {
					t.Errorf("expected 2 attempts, got %d", attempts.Load())
				}
				if len(hooks.next) != 1 || hooks.next[0] != 2 {
					t.Errorf("OnRetry should announce attempt 2, got %v", hooks.next)
				}
			})
		}
	}
}

func sdkSend(c *Client, method string) (*Response, error) {
	ctx := context.Background()
	switch method {
	case "GET":
		return c.Get(ctx, "/acme/resource.json")
	case "POST":
		return c.Post(ctx, "/acme/resource.json", map[string]string{"name": "test"})
	case "PUT":
		return c.Put(ctx, "/acme/resource.json", map[string]string{"name": "test"})
	case "PATCH":
		return c.Patch(ctx, "/acme/resource.json", map[string]string{"name": "test"})
	case "DELETE":
		return c.Delete(ctx, "/acme/resource.json")
	}
	panic("sdkSend: " + method)
}

func TestRetryOn429(t *testing.T) {
	var attempts atomic.Int32
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 2 {
			w.WriteHeader(429)
			return
		}
		sdkReply(http.StatusOK, `{"id":"1"}`)(w, r)
	})

	resp, err := sdkClient(srv.URL).Get(context.Background(), "/acme/resource.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if attempts.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts.Load())
	}
}

func TestRetryAfterHeaderReachesTheHooks(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
	})

	hooks := &requestRecordingHooks{}
	_, err := sdkClient(srv.URL, WithMaxRetries(1), WithHooks(hooks)).Get(context.Background(), "/acme/resource.json")
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeRateLimit || apiErr.ExitCode() != ExitRateLimit {
		t.Fatalf("expected a rate-limit *Error, got %v", err)
	}
	if len(hooks.results) != 1 || hooks.results[0].RetryAfter != 120 || hooks.results[0].StatusCode != 429 {
		t.Errorf("the hooks should see the 429 and its Retry-After, got %+v", hooks.results)
	}
}

func TestRetryGivesUpAfterMaxRetries(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusServiceUnavailable, ``))

	_, err := sdkClient(srv.URL).Get(context.Background(), "/acme/resource.json")
	if err == nil {
		t.Fatal("expected error")
	}
	if n := len(srv.recorded()); n != DefaultMaxRetries {
		t.Errorf("expected %d attempts, got %d", DefaultMaxRetries, n)
	}
	if !strings.Contains(err.Error(), "request failed after 3 attempts: Gateway error (503)") {
		t.Errorf("got %q", err)
	}
}

func TestNoRetryPostOn5xx(t *testing.T) {
	for _, status := range []int{500, 502, 503, 504} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			srv := newSDKServer(t, sdkReply(status, `{"error":"server error"}`))

			_, err := sdkClient(srv.URL).Post(context.Background(), "/acme/resource.json", map[string]string{"name": "test"})
			if err == nil {
				t.Fatal("expected error")
			}
			if n := len(srv.recorded()); n != 1 {
				t.Errorf("expected 1 attempt (no retry for POST on 5xx), got %d", n)
			}
		})
	}
}

func TestNoRetryPostOn429(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusTooManyRequests, ``))

	_, err := sdkClient(srv.URL).Post(context.Background(), "/acme/resource.json", map[string]string{"name": "test"})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != CodeRateLimit {
		t.Fatalf("expected a rate-limit *Error, got %v", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("expected 1 attempt (a POST is sent once), got %d", n)
	}
}

func TestRetryIdempotentPost(t *testing.T) {
	var attempts atomic.Int32
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) <= 1 {
			w.WriteHeader(503)
			return
		}
		sdkReply(http.StatusOK, `{"id":"1"}`)(w, r)
	})

	resp, err := sdkClient(srv.URL).Post(WithIdempotent(context.Background()), "/acme/resource.json", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != 200 || attempts.Load() != 2 {
		t.Errorf("an idempotent POST should be retried, got %d after %d attempts", resp.StatusCode, attempts.Load())
	}
}

func TestNoRetryOn4xx(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 422} {
		for _, method := range []string{"GET", "PUT", "PATCH", "DELETE"} {
			t.Run(fmt.Sprintf("%s %d", method, status), func(t *testing.T) {
				srv := newSDKServer(t, sdkReply(status, `{}`))

				if _, err := sdkSend(sdkClient(srv.URL), method); err == nil {
					t.Fatal("expected error")
				}
				if n := len(srv.recorded()); n != 1 {
					t.Errorf("a %d is a refusal and is never retried, got %d attempts", status, n)
				}
			})
		}
	}
}

func TestWithNoRetry(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusServiceUnavailable, ``))

	_, err := sdkClient(srv.URL).Get(WithNoRetry(context.Background()), "/acme/resource.json")
	if err == nil {
		t.Fatal("expected error")
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("WithNoRetry should send once, got %d attempts", n)
	}
}

func TestRetryStopsWhenTheContextIsCancelled(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusServiceUnavailable, ``))
	ctx, cancel := context.WithCancel(context.Background())
	hooks := &cancelOnRetryHooks{cancel: cancel}

	_, err := sdkClient(srv.URL, WithBaseDelay(time.Hour), WithHooks(hooks)).Get(ctx, "/acme/resource.json")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the cancellation, got %v", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("expected 1 attempt before the cancellation, got %d", n)
	}
}

type cancelOnRetryHooks struct {
	NoopHooks
	cancel context.CancelFunc
}

func (h *cancelOnRetryHooks) OnRetry(context.Context, RequestInfo, int, error) { h.cancel() }

func TestNetworkError(t *testing.T) {
	hooks := &retryRecordingHooks{}
	c := sdkClient("http://localhost:1", WithMaxRetries(2), WithHooks(hooks)) // Invalid port
	_, err := c.Get(context.Background(), "/acme/resource.json")

	if err == nil {
		t.Fatal("expected network error")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *Error, got %T", err)
	}

	if apiErr.Code != CodeNetwork {
		t.Errorf("expected code 'network', got '%s'", apiErr.Code)
	}
	if apiErr.ExitCode() != ExitNetwork {
		t.Errorf("expected exit code %d, got %d", ExitNetwork, apiErr.ExitCode())
	}
	if len(hooks.next) != 1 {
		t.Errorf("a network error on a GET should be retried, got retries %v", hooks.next)
	}
}

// --- pagination ---

func TestGetWithPagination(t *testing.T) {
	var serverURL string
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", fmt.Sprintf(`<%s/acme/posts.json?page=2>; rel="next"`, serverURL))
			sdkReply(http.StatusOK, `[{"id":"1"},{"id":"2"}]`)(w, r)
		case "2":
			w.Header().Set("Link", `</acme/posts.json?page=3>; rel="next"`)
			sdkReply(http.StatusOK, `[{"id":"3"}]`)(w, r)
		default:
			sdkReply(http.StatusOK, `[{"id":"4"}]`)(w, r)
		}
	})
	serverURL = srv.URL

	items, err := sdkAccount(srv.URL).GetAll(context.Background(), "/posts.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("expected 4 items over three pages, got %d", len(items))
	}
	recorded := srv.recorded()
	uris := make([]string, 0, len(recorded))
	for _, r := range recorded {
		uris = append(uris, r.URI)
	}
	if strings.Join(uris, " ") != "/acme/posts.json /acme/posts.json?page=2 /acme/posts.json?page=3" {
		t.Errorf("expected the next links followed, absolute then relative, got %v", uris)
	}
}

func TestGetAllStopsOnAnEmptyPage(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</acme/posts.json?page=9>; rel="next"`)
		if r.URL.Query().Get("page") == "" {
			sdkReply(http.StatusOK, `[{"id":"1"}]`)(w, r)
			return
		}
		sdkReply(http.StatusOK, `[]`)(w, r)
	})

	items, err := sdkAccount(srv.URL).GetAll(context.Background(), "/posts.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || len(srv.recorded()) != 2 {
		t.Errorf("the app links a page past the end: expected 1 item in 2 requests, got %d in %d", len(items), len(srv.recorded()))
	}
}

func TestGetAllWithLimit(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</acme/posts.json?page=next>; rel="next"`)
		sdkReply(http.StatusOK, `[{"id":"a"},{"id":"b"}]`)(w, r)
	})

	items, err := sdkAccount(srv.URL).GetAllWithLimit(context.Background(), "/posts.json", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 || len(srv.recorded()) != 2 {
		t.Errorf("expected 3 items in 2 requests, got %d in %d", len(items), len(srv.recorded()))
	}
}

func TestGetAllMaxPages(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</acme/posts.json?page=next>; rel="next"`)
		sdkReply(http.StatusOK, `[{"id":"a"}]`)(w, r)
	})

	items, err := sdkAccount(srv.URL, WithMaxPages(2)).GetAll(context.Background(), "/posts.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 || len(srv.recorded()) != 2 {
		t.Errorf("expected the cap of 2 pages, got %d items in %d requests", len(items), len(srv.recorded()))
	}
}

func TestGetAllRefusesACrossOriginNextLink(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<https://evil.example.com/acme/posts.json?page=2>; rel="next"`)
		sdkReply(http.StatusOK, `[{"id":"a"}]`)(w, r)
	})

	_, err := sdkAccount(srv.URL).GetAll(context.Background(), "/posts.json")
	if err == nil || !strings.Contains(err.Error(), "pagination Link header points to different origin") {
		t.Fatalf("expected the cross-origin link refused, got %v", err)
	}
	if n := len(srv.recorded()); n != 1 {
		t.Errorf("expected 1 request, got %d", n)
	}
}

func TestGetAllRefusesANonList(t *testing.T) {
	srv := newSDKServer(t, sdkReply(http.StatusOK, `{"id":"a"}`))

	_, err := sdkAccount(srv.URL).GetAll(context.Background(), "/posts.json")
	if err == nil || !strings.Contains(err.Error(), "failed to parse response") {
		t.Fatalf("got %v", err)
	}
}

func TestListReturnsTheNextPageLink(t *testing.T) {
	srv := newSDKServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `</acme/subscribers.json?page=3>; rel="next"`)
		sdkReply(http.StatusOK, `[{"id":"c1","name":"Jane"}]`)(w, r)
	})

	subscribers, resp, err := listAt[User](context.Background(), sdkAccount(srv.URL), "/subscribers.json?page=2", "/subscribers.json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subscribers) != 1 || subscribers[0].Name != "Jane" {
		t.Errorf("got %+v", subscribers)
	}
	if resp.NextPageURL() != "/acme/subscribers.json?page=3" {
		t.Errorf("NextPageURL = %q", resp.NextPageURL())
	}
	if uri := srv.recorded()[0].URI; uri != "/acme/subscribers.json?page=2" {
		t.Errorf("uri = %s", uri)
	}
}

// --- hooks ---

type orderHooks struct {
	NoopHooks
	name string
	log  *[]string
}

func (h orderHooks) OnRequestStart(ctx context.Context, _ RequestInfo) context.Context {
	*h.log = append(*h.log, "start "+h.name)
	return ctx
}

func (h orderHooks) OnRequestEnd(context.Context, RequestInfo, RequestResult) {
	*h.log = append(*h.log, "end "+h.name)
}

func TestChainHooks(t *testing.T) {
	if _, ok := NewChainHooks(nil, NoopHooks{}).(NoopHooks); !ok {
		t.Error("nil and no-op hooks alone should chain to NoopHooks")
	}
	var log []string
	a := orderHooks{name: "a", log: &log}
	if NewChainHooks(nil, a) != Hooks(a) {
		t.Error("one hook should chain to itself")
	}

	srv := newSDKServer(t, sdkReply(http.StatusOK, `{}`))
	chain := NewChainHooks(a, orderHooks{name: "b", log: &log})
	if _, err := sdkClient(srv.URL, WithHooks(chain)).Get(context.Background(), "/my/identity.json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(log, ", ") != "start a, start b, end b, end a" {
		t.Errorf("starts run in order and ends in reverse, got %v", log)
	}
}
