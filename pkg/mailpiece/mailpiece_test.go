package mailpiece

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

// --- version ---

func TestVersion(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
	if DefaultUserAgent != "mailpiece-sdk-go/"+Version {
		t.Fatalf("DefaultUserAgent = %q", DefaultUserAgent)
	}
}

// --- errors ---

func TestErrorMessage(t *testing.T) {
	e := &Error{Message: "boom"}
	if e.Error() != "boom" {
		t.Fatalf("got %q", e.Error())
	}
}

func TestErrorMessageWithHint(t *testing.T) {
	e := &Error{Message: "boom", Hint: "try again"}
	if e.Error() != "boom: try again" {
		t.Fatalf("got %q", e.Error())
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("root")
	e := &Error{Message: "wrapped", Cause: cause}
	if !errors.Is(e, cause) {
		t.Fatal("Unwrap should expose cause")
	}
}

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		code string
		want int
	}{
		{CodeUsage, ExitUsage},
		{CodeNotFound, ExitNotFound},
		{CodeAuth, ExitAuth},
		{CodeForbidden, ExitForbidden},
		{CodeRateLimit, ExitRateLimit},
		{CodeNetwork, ExitNetwork},
		{CodeAPI, ExitAPI},
		{CodeValidation, ExitValidation},
		{CodeAmbiguous, ExitAmbiguous},
		{"unknown", ExitAPI},
	}
	for _, tc := range cases {
		if got := ExitCodeFor(tc.code); got != tc.want {
			t.Errorf("ExitCodeFor(%q) = %d, want %d", tc.code, got, tc.want)
		}
	}
}

func TestExitCodeValues(t *testing.T) {
	cases := map[string]struct{ got, want int }{
		"ok":         {ExitOK, 0},
		"usage":      {ExitUsage, 1},
		"not found":  {ExitNotFound, 2},
		"auth":       {ExitAuth, 3},
		"forbidden":  {ExitForbidden, 4},
		"rate limit": {ExitRateLimit, 5},
		"network":    {ExitNetwork, 6},
		"api":        {ExitAPI, 7},
		"ambiguous":  {ExitAmbiguous, 8},
		"validation": {ExitValidation, 9},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("exit code %s = %d, want %d", name, tc.got, tc.want)
		}
	}
}

func TestErrorExitCode(t *testing.T) {
	e := ErrNotFound("Post", "123")
	if e.ExitCode() != ExitNotFound {
		t.Fatalf("got %d", e.ExitCode())
	}
}

func TestErrorConstructors(t *testing.T) {
	if e := ErrUsage("bad"); e.Code != CodeUsage || e.Message != "bad" {
		t.Fatalf("ErrUsage: %+v", e)
	}
	if e := ErrUsageHint("bad", "fix"); e.Hint != "fix" {
		t.Fatalf("ErrUsageHint: %+v", e)
	}
	if e := ErrNotFound("Post", "42"); e.Code != CodeNotFound || e.Message != "Post not found: 42" {
		t.Fatalf("ErrNotFound: %+v", e)
	}
	if e := ErrNotFoundHint("Post", "42", "check name"); e.Hint != "check name" {
		t.Fatalf("ErrNotFoundHint: %+v", e)
	}
	if e := ErrAuth("no token"); e.Code != CodeAuth {
		t.Fatalf("ErrAuth: %+v", e)
	}
	if e := ErrForbidden("denied"); e.HTTPStatus != 403 || e.Code != CodeForbidden {
		t.Fatalf("ErrForbidden: %+v", e)
	}
	if e := ErrRateLimit(0); e.Hint != "Try again later" {
		t.Fatalf("ErrRateLimit(0): %+v", e)
	}
	if e := ErrRateLimit(30); e.Hint != "Try again in 30 seconds" {
		t.Fatalf("ErrRateLimit(30): %+v", e)
	}
	if e := ErrNetwork(errors.New("timeout")); !e.Retryable || e.Cause == nil || e.Code != CodeNetwork {
		t.Fatalf("ErrNetwork: %+v", e)
	}
	if e := ErrAPI(500, "oops"); e.HTTPStatus != 500 {
		t.Fatalf("ErrAPI: %+v", e)
	}
	if e := ErrAmbiguous("post", nil); e.Hint != "Be more specific" {
		t.Fatalf("ErrAmbiguous(nil): %+v", e)
	}
	if e := ErrAmbiguous("post", []string{"a", "b"}); e.Hint == "Be more specific" {
		t.Fatalf("ErrAmbiguous with matches should list them: %+v", e)
	}
	if e := nothingToChange(); e.Code != CodeUsage || e.Message != "Nothing to change" || e.Hint == "" {
		t.Fatalf("nothingToChange: %+v", e)
	}
}

func TestAsError(t *testing.T) {
	orig := ErrAuth("test")
	if AsError(orig) != orig {
		t.Fatal("AsError should return the same *Error")
	}

	plain := errors.New("plain")
	wrapped := AsError(plain)
	if wrapped.Code != CodeAPI {
		t.Fatalf("AsError wraps as API: %+v", wrapped)
	}
	if wrapped.Cause != plain { //nolint:errorlint // identity is the point
		t.Fatal("AsError should preserve cause")
	}
}

// --- config ---

func TestNormalizeBaseURL(t *testing.T) {
	if NormalizeBaseURL("https://mailpiece.example.com/") != "https://mailpiece.example.com" {
		t.Fatal("should strip trailing slash")
	}
	if NormalizeBaseURL("https://mailpiece.example.com") != "https://mailpiece.example.com" {
		t.Fatal("should leave clean URL unchanged")
	}
}

func TestNormalizeAccountSlug(t *testing.T) {
	for in, want := range map[string]string{
		"/834206594":  "834206594",
		"834206594":   "834206594",
		"/834206594/": "834206594",
		"":            "",
	} {
		if got := NormalizeAccountSlug(in); got != want {
			t.Errorf("NormalizeAccountSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	cfg := &Config{}
	t.Setenv("MAILPIECE_API_URL", "https://custom.api")
	t.Setenv("MAILPIECE_ACCOUNT", "acct-1")
	cfg.LoadConfigFromEnv()

	if cfg.BaseURL != "https://custom.api" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Account != "acct-1" {
		t.Fatalf("Account = %q", cfg.Account)
	}
}

func TestLoadConfigFromEnvKeepsValuesWhenUnset(t *testing.T) {
	cfg := &Config{BaseURL: "https://kept.example.com", Account: "kept"}
	t.Setenv("MAILPIECE_API_URL", "")
	t.Setenv("MAILPIECE_ACCOUNT", "")
	cfg.LoadConfigFromEnv()

	if cfg.BaseURL != "https://kept.example.com" || cfg.Account != "kept" {
		t.Fatalf("empty env should change nothing, got %+v", cfg)
	}
}

// --- security ---

func TestRequireSecureEndpoint(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://mailpiece.example.com/posts.json", false},
		{"http://localhost:3000/posts.json", false},
		{"http://127.0.0.1:3000/posts.json", false},
		{"http://[::1]:3000/posts.json", false},
		{"http://foo.localhost/posts.json", false},
		{"http://mailpiece.example.com/posts.json", true},
		{"ftp://mailpiece.example.com/posts.json", true},
	}
	for _, tc := range cases {
		err := RequireSecureEndpoint(tc.url)
		if (err != nil) != tc.wantErr {
			t.Errorf("RequireSecureEndpoint(%q) err=%v, wantErr=%v", tc.url, err, tc.wantErr)
		}
	}
}

func TestRequireHTTPSNamesOnlyTheOrigin(t *testing.T) {
	err := requireHTTPS("http://storage.example.com/blob?sig=SECRETVALUE")
	if err == nil {
		t.Fatal("expected the http URL to be refused")
	}
	if strings.Contains(err.Error(), "SECRETVALUE") || !strings.Contains(err.Error(), "http://storage.example.com") {
		t.Fatalf("got %q", err)
	}
}

func TestRedactHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")
	h.Set("Cookie", "session=abc")
	h.Set("Set-Cookie", "session_token=abc")
	h.Set("X-CSRF-Token", "csrf")
	h.Set("Content-Type", "application/json")

	redacted := RedactHeaders(h)
	for _, key := range []string{"Authorization", "Cookie", "Set-Cookie", "X-CSRF-Token"} {
		if redacted.Get(key) != "[REDACTED]" {
			t.Errorf("%s not redacted: %q", key, redacted.Get(key))
		}
	}
	if redacted.Get("Content-Type") != "application/json" {
		t.Fatal("Content-Type should be unchanged")
	}
	// Original should be untouched
	if h.Get("Authorization") != "Bearer secret" {
		t.Fatal("original header mutated")
	}
}

func TestRedactHeadersLeavesAbsentHeadersAbsent(t *testing.T) {
	redacted := RedactHeaders(http.Header{"Accept": {"application/json"}})
	if _, ok := redacted["Authorization"]; ok {
		t.Fatal("an absent Authorization should stay absent")
	}
}

func TestIsSameOrigin(t *testing.T) {
	if !isSameOrigin("https://mailpiece.example.com/a", "https://mailpiece.example.com/b") {
		t.Fatal("same origin should match")
	}
	if isSameOrigin("https://mailpiece.example.com/a", "https://evil.com/b") {
		t.Fatal("different hosts should not match")
	}
	if !isSameOrigin("https://mailpiece.example.com:443/a", "https://mailpiece.example.com/b") {
		t.Fatal("default port should be normalized")
	}
	if isSameOrigin("http://127.0.0.1:3000/a", "http://127.0.0.1:3001/a") {
		t.Fatal("different ports should not match")
	}
	if isSameOrigin("http://mailpiece.example.com/a", "https://mailpiece.example.com/a") {
		t.Fatal("different schemes should not match")
	}
	if isSameOrigin("/a", "/a") {
		t.Fatal("relative URLs have no origin")
	}
}

func TestTruncateString(t *testing.T) {
	if truncateString("short", 100) != "short" {
		t.Fatal("should not truncate short strings")
	}
	result := truncateString("this is a long string", 10)
	if len(result) > 10 {
		t.Fatalf("result too long: %q", result)
	}
	if result != "this is..." {
		t.Fatalf("got %q", result)
	}
	if got := truncateString("abcdef", 2); got != "ab" {
		t.Fatalf("got %q", got)
	}
}

func TestLimitedReadAll(t *testing.T) {
	if data, err := limitedReadAll(strings.NewReader("12345"), 5); err != nil || string(data) != "12345" {
		t.Fatalf("got %q, %v", data, err)
	}
	if _, err := limitedReadAll(strings.NewReader("123456"), 5); err == nil {
		t.Fatal("a body over the limit should be refused")
	}
}

// --- pagination ---

func TestParseNextLink(t *testing.T) {
	cases := []struct {
		header string
		want   string
	}{
		{`<https://mailpiece.example.com/acme/posts.json?page=2>; rel="next"`, "https://mailpiece.example.com/acme/posts.json?page=2"},
		{`<https://mailpiece.example.com/acme/posts.json?page=2>; rel="next", <https://mailpiece.example.com/acme/posts.json?page=5>; rel="last"`, "https://mailpiece.example.com/acme/posts.json?page=2"},
		{`<https://mailpiece.example.com/acme/posts.json?page=1>; rel="prev"`, ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := parseNextLink(tc.header); got != tc.want {
			t.Errorf("parseNextLink(%q) = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestResponseNextPageURL(t *testing.T) {
	var none *Response
	if none.NextPageURL() != "" {
		t.Fatal("a nil response has no next page")
	}
	resp := &Response{Headers: http.Header{"Link": {`</acme/posts.json?page=2>; rel="next"`}}}
	if got := resp.NextPageURL(); got != "/acme/posts.json?page=2" {
		t.Fatalf("got %q", got)
	}
}

// --- paths ---

func TestValidateNumber(t *testing.T) {
	if err := ValidateNumber(1); err != nil {
		t.Fatalf("1 is a number: %v", err)
	}
	for _, n := range []int64{0, -3} {
		err := ValidateNumber(n)
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeUsage {
			t.Errorf("ValidateNumber(%d) = %v, want a usage error", n, err)
		}
	}
}

func TestPath(t *testing.T) {
	got, err := Path("/subscribers/%s/links/%d.json", "c 1/x", int64(7))
	if err != nil {
		t.Fatal(err)
	}
	if got != "/subscribers/c%201%2Fx/links/7.json" {
		t.Fatalf("got %q", got)
	}
	if _, err := Path("/links/%d.json", int64(0)); err == nil {
		t.Fatal("a number from 1 is required")
	}
}

func TestPathPanicsOnAnUnknownArgument(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected a panic for an int argument")
		}
	}()
	_, _ = Path("/links/%d.json", 7)
}

func TestValidationMessage(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"record errors, sorted by field", `{"title":["is invalid"],"name":["can't be blank","is too short"]}`, "name: can't be blank; name: is too short; title: is invalid"},
		{"error key", `{"error":"Not allowed"}`, "Not allowed"},
		{"message key", `{"message":"Nope"}`, "Nope"},
		{"empty body", ``, "Validation failed"},
		{"empty object", `{}`, "Validation failed"},
		{"not JSON", `<html>`, "Validation failed"},
		{"field with no messages", `{"name":[]}`, "Validation failed"},
		{"unexpected shape", `{"base":"broken"}`, `Validation failed: {"base":"broken"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validationMessage([]byte(tc.body)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
