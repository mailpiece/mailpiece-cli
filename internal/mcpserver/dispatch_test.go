package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mailpiece/mailpiece-cli/internal/mcpserver/catalog"
)

func operation(t *testing.T, domainKey, action string) *catalog.Operation {
	t.Helper()
	domains, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range domains {
		if d.Key != domainKey {
			continue
		}
		op, ok := d.Operation(action)
		if !ok {
			t.Fatalf("action %q not in domain %q", action, domainKey)
		}
		return op
	}
	t.Fatalf("domain %q not in catalog", domainKey)
	return nil
}

// params round-trips arguments through JSON so values arrive exactly as
// the MCP layer delivers them (numbers as float64, arrays as []any).
func params(t *testing.T, jsonText string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonText), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestBuildRequest(t *testing.T) {
	cases := []struct {
		name     string
		domain   string
		action   string
		params   string
		wantPath string
		wantBody string // JSON, "" for no body
	}{
		{
			name:   "no params",
			domain: "fixtures", action: "list_fixtures",
			params:   `{}`,
			wantPath: "/fixtures.json",
		},
		{
			name:   "path param substituted and escaped",
			domain: "fixtures", action: "get_fixture",
			params:   `{"fixture_id": "4/2"}`,
			wantPath: "/fixtures/4%2F2.json",
		},
		{
			name:   "numeric path param renders without exponent",
			domain: "fixtures", action: "get_fixture",
			params:   `{"fixture_id": 42}`,
			wantPath: "/fixtures/42.json",
		},
		{
			name:   "two path params",
			domain: "fixtures", action: "place_fixture",
			params:   `{"fixture_id": "f1", "shelf_id": "s1"}`,
			wantPath: "/fixtures/f1/shelves/s1/placement.json",
		},
		{
			name:   "query param encoded",
			domain: "fixtures", action: "list_fixtures",
			params:   `{"state": "on hold"}`,
			wantPath: "/fixtures.json?state=on+hold",
		},
		{
			name:   "page param",
			domain: "fixtures", action: "list_fixtures",
			params:   `{"page": 2}`,
			wantPath: "/fixtures.json?page=2",
		},
		{
			name:   "array query param encoded rails style",
			domain: "fixtures", action: "list_fixtures",
			params:   `{"page": [1, 2]}`,
			wantPath: "/fixtures.json?page%5B%5D=1&page%5B%5D=2",
		},
		{
			name:   "body wrapped under body key",
			domain: "fixtures", action: "create_fixture",
			params:   `{"name": "First"}`,
			wantPath: "/fixtures.json",
			wantBody: `{"fixture": {"name": "First"}}`,
		},
		{
			name:   "flat body on a nested path",
			domain: "fixtures", action: "create_note",
			params:   `{"fixture_id": "f1", "text": "Checked"}`,
			wantPath: "/fixtures/f1/notes.json",
			wantBody: `{"text": "Checked"}`,
		},
		{
			name:   "bodiless write",
			domain: "fixtures", action: "activate_fixture",
			params:   `{"fixture_id": "f1"}`,
			wantPath: "/fixtures/f1/activation.json",
		},
		{
			name:   "wrapped body outside the account",
			domain: "access_tokens", action: "create_access_token",
			params:   `{"description": "CI", "permission": "read"}`,
			wantPath: "/my/access_tokens.json",
			wantBody: `{"access_token": {"description": "CI", "permission": "read"}}`,
		},
		{
			name:   "unscoped path",
			domain: "access_tokens", action: "delete_access_token",
			params:   `{"access_token_id": "t1"}`,
			wantPath: "/my/access_tokens/t1.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, body, err := buildRequest(operation(t, tc.domain, tc.action), params(t, tc.params))
			if err != nil {
				t.Fatal(err)
			}
			if path != tc.wantPath {
				t.Errorf("path = %q, want %q", path, tc.wantPath)
			}
			if tc.wantBody == "" {
				if body != nil {
					t.Errorf("body = %v, want none", body)
				}
			} else {
				got, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				assertJSONEq(t, tc.wantBody, string(got))
			}
		})
	}
}

func TestBuildRequestErrors(t *testing.T) {
	cases := []struct {
		name   string
		domain string
		action string
		params string
		want   string
	}{
		{
			name:   "missing path param",
			domain: "fixtures", action: "get_fixture",
			params: `{}`,
			want:   `missing required param "fixture_id"`,
		},
		{
			name:   "blank path param",
			domain: "fixtures", action: "get_fixture",
			params: `{"fixture_id": ""}`,
			want:   `missing required param "fixture_id"`,
		},
		{
			name:   "missing required body field",
			domain: "fixtures", action: "create_fixture",
			params: `{}`,
			want:   `missing required param "name"`,
		},
		{
			name:   "unknown param names what the action accepts",
			domain: "fixtures", action: "get_fixture",
			params: `{"fixture_id": "f1", "bogus": true}`,
			want:   `unknown param "bogus" for action "get_fixture" (accepts: fixture_id)`,
		},
		{
			name:   "non-scalar path param",
			domain: "fixtures", action: "get_fixture",
			params: `{"fixture_id": {"nested": 1}}`,
			want:   "must be a string, number, or boolean",
		},
		{
			name:   "non-scalar query param",
			domain: "fixtures", action: "list_fixtures",
			params: `{"page": {"nested": 1}}`,
			want:   "must be a scalar",
		},
		{
			name:   "array of objects in query param",
			domain: "fixtures", action: "list_fixtures",
			params: `{"page": [{"nested": 1}]}`,
			want:   "must be a scalar",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := buildRequest(operation(t, tc.domain, tc.action), params(t, tc.params))
			if err == nil {
				t.Fatal("buildRequest accepted the params")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// nextPage reads the target the SDK already took from the Link header.
func TestNextPage(t *testing.T) {
	cases := []struct {
		name string
		next string
		want int
	}{
		{"absent", "", 0},
		{"next with page", "https://app.mailpiece.dev/834206594/posts.json?page=2", 2},
		{"relative next", "/834206594/posts.json?page=3", 3},
		{"next without page", "https://app.mailpiece.dev/834206594/posts.json", 0},
		{"page not a number", "https://app.mailpiece.dev/834206594/posts.json?page=last", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextPage(tc.next); got != tc.want {
				t.Errorf("nextPage(%q) = %d, want %d", tc.next, got, tc.want)
			}
		})
	}
}

func TestBodiless(t *testing.T) {
	cases := []struct {
		data string
		want bool
	}{
		{"", true},
		{"  ", true},
		{"null", true},
		{"{}", false},
		{"[]", false},
	}
	for _, tc := range cases {
		if got := bodiless(json.RawMessage(tc.data)); got != tc.want {
			t.Errorf("bodiless(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestPageEmpty(t *testing.T) {
	cases := []struct {
		data string
		want bool
	}{
		{"[]", true},
		{` [ ] `, true},
		{`[{"id": "s1"}]`, false},
		{`{"id": "s1", "deliveries": []}`, true},
		{`{"id": "s1", "deliveries": [{"status": "sent"}]}`, false},
		{`{"id": "s1", "bounced_at": null}`, false},
		{"null", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := pageEmpty(json.RawMessage(tc.data)); got != tc.want {
			t.Errorf("pageEmpty(%q) = %v, want %v", tc.data, got, tc.want)
		}
	}
}

func TestCallRefusesWhatTheSDKCannotSend(t *testing.T) {
	if _, err := call(context.Background(), nil, "GET", "/posts.json", map[string]any{"x": 1}); err == nil || !strings.Contains(err.Error(), "declares a request body") {
		t.Errorf("GET with a body: err = %v", err)
	}
	if _, err := call(context.Background(), nil, "DELETE", "/posts/p1.json", map[string]any{"x": 1}); err == nil || !strings.Contains(err.Error(), "declares a request body") {
		t.Errorf("DELETE with a body: err = %v", err)
	}
	if _, err := call(context.Background(), nil, "HEAD", "/posts.json", nil); err == nil || !strings.Contains(err.Error(), "unsupported method HEAD") {
		t.Errorf("HEAD: err = %v", err)
	}
}

func assertJSONEq(t *testing.T, want, got string) {
	t.Helper()
	var w, g any
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatal(err)
	}
	wc, _ := json.Marshal(w)
	gc, _ := json.Marshal(g)
	// Canonical re-marshal sorts map keys, so equal JSON compares equal.
	if string(wc) != string(gc) {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}
