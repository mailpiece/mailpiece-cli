// Wire tests: a real MCP client drives the server over in-memory
// transports (the toolkit's mcptest harness), and dispatched actions land
// on a fake Mailpiece (httptest) through a real SDK client, so the asserted
// HTTP surface (bearer auth, account scoping, .json paths, query encoding,
// body wrapping) is exactly what `mailpiece mcp` sends, and the played-back
// response shapes (JSON, 204s, 201 Locations, Link pagination) are
// rendered the way the app produces them.
package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/basecamp/mcp/mcptest"

	"github.com/mailpiece/mailpiece-cli/internal/mcpserver/catalog"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

const testAccount = "834206594"

// fakeMailpiece runs an httptest server that checks auth on every request
// and serves the registered handlers.
func fakeMailpiece(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want the CLI's token", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// refusingMailpiece fails the test on any request: what it guards must be
// refused before anything is sent.
func refusingMailpiece(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func testSDK(baseURL string) *mailpiece.Client {
	return mailpiece.NewClient(
		&mailpiece.Config{BaseURL: baseURL},
		&mailpiece.StaticTokenProvider{Token: "test-token"},
		mailpiece.WithBaseDelay(time.Millisecond),
		mailpiece.WithMaxJitter(time.Millisecond),
	)
}

// connect builds a server against the fake Mailpiece, through a real SDK
// client bound to the test account, and connects a real MCP client to it.
// Writes are enabled unless the config says otherwise, so dispatch tests
// reach write actions; read-only is the served default and has its own
// tests.
func connect(t *testing.T, cfg Config, upstream *httptest.Server) (*Server, *mcp.ClientSession) {
	t.Helper()

	baseURL := "http://127.0.0.1:0" // no upstream: any dispatch attempt fails loudly
	if upstream != nil {
		baseURL = upstream.URL
	}
	sdk := testSDK(baseURL)

	srv, err := New(sdk.ForAccount(testAccount), sdk, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return srv, mcptest.Connect(t, srv.BuildMCPServer(slog.New(slog.DiscardHandler)))
}

func callJSON(t *testing.T, session *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	text, isError := mcptest.CallText(t, session, tool, args)
	if isError {
		t.Fatalf("call failed: %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("result %q: %v", text, err)
	}
	return payload
}

func readBody(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		t.Fatalf("body %q: %v", data, err)
	}
	return body
}

func countActions(srv *Server) int {
	total := 0
	for _, d := range srv.Domains() {
		total += len(d.ActionNames())
	}
	return total
}

func TestListToolsServesCatalog(t *testing.T) {
	srv, session := connect(t, Config{}, nil)
	tools := mcptest.ListTools(t, session)

	domains, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	actions := 0
	for _, d := range domains {
		actions += len(d.Operations)
		tool, ok := tools[d.Tool]
		if !ok {
			t.Errorf("missing tool %q", d.Tool)
			continue
		}
		if !strings.Contains(tool.Description, "ACTIONS") {
			t.Errorf("tool %q description lacks its action list", d.Tool)
		}
	}
	if len(tools) != len(domains) {
		t.Errorf("tools/list returned %d tools, want %d", len(tools), len(domains))
	}
	if got := countActions(srv); got != actions {
		t.Errorf("served actions = %d, want %d", got, actions)
	}

	// identity is all reads; fixtures is not.
	if !tools["mailpiece_identity"].Annotations.ReadOnlyHint {
		t.Error("mailpiece_identity must hint read-only")
	}
	if tools["mailpiece_fixtures"].Annotations.ReadOnlyHint {
		t.Error("mailpiece_fixtures must not hint read-only with writes served")
	}
}

// Every tool carries a title and all four hints, explicit, as connector
// directories require. With writes served, a tool is destructive when one of
// its writes is and idempotent when all its writes are; read-only, every
// tool is a read: idempotent and never destructive. The world is closed.
func TestToolsCarryTitleAndTruthfulHints(t *testing.T) {
	type hints struct{ readOnly, idempotent, destructive bool }
	withWrites := map[string]hints{
		"mailpiece_identity":      {readOnly: true, idempotent: true},
		"mailpiece_access_tokens": {destructive: true},
		"mailpiece_snippets":      {destructive: true},
		"mailpiece_journeys":      {destructive: true},
		"mailpiece_settings":      {destructive: true},
		"mailpiece_users":         {idempotent: true},
		"mailpiece_posts":         {destructive: true},
		"mailpiece_search":        {readOnly: true, idempotent: true},
		"mailpiece_subscribers":   {destructive: true},
		"mailpiece_fixtures":      {},
	}

	domains, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]string{}
	for _, d := range domains {
		titles[d.Tool] = d.Title
	}

	for _, cfg := range []Config{{}, {ReadOnly: true}} {
		_, session := connect(t, cfg, nil)
		for name, tool := range mcptest.ListTools(t, session) {
			a := tool.Annotations
			if a == nil {
				t.Errorf("%s: no annotations", name)
				continue
			}
			if a.Title == "" || a.Title != titles[name] {
				t.Errorf("%s: title = %q, want %q", name, a.Title, titles[name])
			}
			if a.DestructiveHint == nil || a.OpenWorldHint == nil {
				t.Errorf("%s: destructive and open-world hints must be explicit: %+v", name, a)
				continue
			}
			if *a.OpenWorldHint {
				t.Errorf("%s: openWorldHint set", name)
			}

			want := hints{readOnly: true, idempotent: true}
			if !cfg.ReadOnly {
				var ok bool
				if want, ok = withWrites[name]; !ok {
					t.Errorf("%s: no expected hints; add it to the table", name)
					continue
				}
			}
			got := hints{readOnly: a.ReadOnlyHint, idempotent: a.IdempotentHint, destructive: *a.DestructiveHint}
			if got != want {
				t.Errorf("%s (read-only %v): hints = %+v, want %+v", name, cfg.ReadOnly, got, want)
			}
		}
	}
}

func TestReadOnlyFiltersWriteActions(t *testing.T) {
	srv, session := connect(t, Config{ReadOnly: true}, refusingMailpiece(t))

	// Every domain with a read survives, and every survivor is all-read.
	domains, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	withARead, reads := 0, 0
	for _, d := range domains {
		domainReads := 0
		for _, op := range d.Operations {
			if op.ReadOnly {
				domainReads++
			}
		}
		if domainReads > 0 {
			withARead++
		}
		reads += domainReads
	}
	tools := mcptest.ListTools(t, session)
	if len(tools) != withARead {
		t.Fatalf("tools/list returned %d tools, want %d", len(tools), withARead)
	}
	for name, tool := range tools {
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q must be read-only", name)
		}
	}
	if _, ok := tools["mailpiece_access_tokens"]; ok {
		t.Error("read-only server serves mailpiece_access_tokens, which has no read")
	}
	if got := countActions(srv); got != reads {
		t.Errorf("served actions = %d, want %d", got, reads)
	}
	for _, d := range srv.Domains() {
		domain, ok := d.(*catalog.Domain)
		if !ok {
			t.Fatalf("served domain %q is not a catalog domain", d.Name())
		}
		for _, op := range domain.Operations {
			if op.Method != http.MethodGet {
				t.Errorf("read-only serves %s/%s, a %s", domain.Key, op.Action, op.Method)
			}
		}
	}

	// A filtered write action is gone from the catalog, so dispatch refuses
	// it in-band, without a request, even when a client ignores the schema.
	for _, refused := range []struct{ tool, action string }{
		{"mailpiece_fixtures", "create_fixture"},
		{"mailpiece_fixtures", "move_fixture"},
	} {
		text, isError := mcptest.CallText(t, session, refused.tool, map[string]any{"action": refused.action})
		if !isError {
			t.Fatalf("%s %s succeeded on read-only server: %s", refused.tool, refused.action, text)
		}
		if !strings.Contains(text, "unknown action") {
			t.Errorf("refusal = %q", text)
		}
	}

	// A write-only tool is not served at all.
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "mailpiece_access_tokens",
		Arguments: map[string]any{"action": "create_access_token", "params": map[string]any{"description": "x", "permission": "write"}},
	})
	if err == nil && !res.IsError {
		t.Error("mailpiece_access_tokens answered on a read-only server")
	}
}

func TestDescribeServesOperationSchema(t *testing.T) {
	_, session := connect(t, Config{}, nil)

	op := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "describe",
		"params": map[string]any{"action": "place_fixture"},
	})
	if op["method"] != "POST" || op["path"] != "/fixtures/{fixture_id}/shelves/{shelf_id}/placement" {
		t.Errorf("describe = %v", op)
	}
}

func TestUnknownDomainFailsClosed(t *testing.T) {
	sdk := testSDK("http://127.0.0.1:0")
	_, err := New(sdk.ForAccount(testAccount), sdk, Config{Domains: []string{"identity", "nope"}})
	if err == nil || !strings.Contains(err.Error(), `unknown domain "nope"`) {
		t.Errorf("err = %v, want unknown domain failure", err)
	}
}

func TestNarrowedDomainsServeOnlyThose(t *testing.T) {
	_, session := connect(t, Config{Domains: []string{"fixtures"}}, nil)
	tools := mcptest.ListTools(t, session)
	if len(tools) != 1 {
		t.Fatalf("tools = %d, want just mailpiece_fixtures", len(tools))
	}
	if _, ok := tools["mailpiece_fixtures"]; !ok {
		t.Fatal("mailpiece_fixtures missing")
	}
}

func TestServerRequiresClients(t *testing.T) {
	if _, err := New(nil, nil, Config{}); err == nil {
		t.Error("nil clients did not error")
	}
}

func TestServerName(t *testing.T) {
	sdk := testSDK("http://127.0.0.1:0")
	srv, err := New(sdk.ForAccount(testAccount), sdk, Config{Version: "1.2.3"})
	if err != nil {
		t.Fatal(err)
	}
	session := mcptest.Connect(t, srv.BuildMCPServer(slog.New(slog.DiscardHandler)))
	info := session.InitializeResult().ServerInfo
	if info.Name != "mailpiece-cli" || info.Version != "1.2.3" {
		t.Errorf("server info = %+v, want mailpiece-cli 1.2.3", info)
	}
}

func TestDispatchGetIsAccountScoped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+testAccount+"/fixtures/s1.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "s1", "name": "First"}`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	fixture := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "get_fixture",
		"params": map[string]any{"fixture_id": "s1"},
	})
	if fixture["name"] != "First" {
		t.Errorf("fixture = %v", fixture)
	}
}

func TestReadOnlyServesReads(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+testAccount+"/fixtures/42.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "42", "name": "First"}`))
	})
	_, session := connect(t, Config{ReadOnly: true}, fakeMailpiece(t, mux))

	fixture := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "get_fixture",
		"params": map[string]any{"fixture_id": 42},
	})
	if fixture["name"] != "First" {
		t.Errorf("fixture = %v", fixture)
	}
}

func TestDispatchEncodesQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("state"); got != "on hold" {
			t.Errorf("state = %q", got)
		}
		_, _ = w.Write([]byte(`[]`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	text, isError := mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "list_fixtures",
		"params": map[string]any{"state": "on hold"},
	})
	if isError || text != `[]` {
		t.Errorf("result = %q (isError=%v)", text, isError)
	}
}

func TestDispatchWrapsBodyAndFollowsCreatedLocation(t *testing.T) {
	location := "/" + testAccount + "/fixtures/s9.json"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		fixture, _ := readBody(t, r)["fixture"].(map[string]any)
		if fixture["name"] != "First" {
			t.Errorf("fixture body = %v", fixture)
		}
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET "+location, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "s9", "name": "First"}`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	fixture := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "create_fixture",
		"params": map[string]any{"name": "First"},
	})
	if fixture["id"] != "s9" {
		t.Errorf("create must return the created resource via its Location, got %v", fixture)
	}
}

// An absolute Location is reduced to its path, so the follow-up read stays
// on the configured instance whatever host the header names.
func TestDispatchFollowsAbsoluteLocationOnTheConfiguredHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "https://elsewhere.example/"+testAccount+"/fixtures/s9.json")
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /"+testAccount+"/fixtures/s9.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id": "s9"}`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	fixture := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "create_fixture",
		"params": map[string]any{"name": "First"},
	})
	if fixture["id"] != "s9" {
		t.Errorf("fixture = %v", fixture)
	}
}

// The write succeeded, so a failed follow-up read reports the creation
// rather than masking it as a failure.
func TestDispatchReportsCreationWhenFollowFails(t *testing.T) {
	location := "/" + testAccount + "/fixtures/s9.json"
	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", location)
		w.WriteHeader(http.StatusCreated)
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	result := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "create_fixture",
		"params": map[string]any{"name": "First"},
	})
	if result["ok"] != true || result["status"] != float64(201) || result["location"] != location {
		t.Errorf("result = %v", result)
	}
	if note, _ := result["note"].(string); !strings.Contains(note, "created, but fetching") {
		t.Errorf("note = %q", note)
	}
}

func TestDispatchReportsBodilessSuccess(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /"+testAccount+"/fixtures/f3/position.json", func(w http.ResponseWriter, r *http.Request) {
		if body := readBody(t, r); body["direction"] != "up" {
			t.Errorf("body = %v", body)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	result := callJSON(t, session, "mailpiece_fixtures", map[string]any{
		"action": "move_fixture",
		"params": map[string]any{"fixture_id": "f3", "direction": "up"},
	})
	if result["ok"] != true || result["status"] != float64(204) {
		t.Errorf("result = %v", result)
	}
}

func TestDispatchSurfacesPagination(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<http://example.test/`+testAccount+`/fixtures.json?page=2>; rel="next"`)
		_, _ = w.Write([]byte(`[{"id": "s1"}]`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	page := callJSON(t, session, "mailpiece_fixtures", map[string]any{"action": "list_fixtures"})
	if page["next_page"] != float64(2) {
		t.Errorf("next_page = %v", page["next_page"])
	}
	if data, ok := page["data"].([]any); !ok || len(data) != 1 {
		t.Errorf("data = %v", page["data"])
	}
}

// The app links a next page past the end of a listing; an empty page
// carries no next_page.
func TestDispatchDropsNextPageOnAnEmptyPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /"+testAccount+"/fixtures.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<http://example.test/`+testAccount+`/fixtures.json?page=3>; rel="next"`)
		_, _ = w.Write([]byte(`[]`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	text, isError := mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "list_fixtures",
		"params": map[string]any{"page": 2},
	})
	if isError || text != `[]` {
		t.Errorf("result = %q (isError=%v)", text, isError)
	}
}

func TestDispatchSurfacesAPIErrorsInBand(t *testing.T) {
	mux := http.NewServeMux() // no routes: everything 404s
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	text, isError := mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "get_fixture",
		"params": map[string]any{"fixture_id": "missing"},
	})
	if !isError {
		t.Fatalf("missing fixture did not error: %s", text)
	}
	if !strings.Contains(text, "404") {
		t.Errorf("error = %q, want the HTTP status surfaced", text)
	}
}

func TestDispatchSurfacesARefusalsReason(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /"+testAccount+"/fixtures/s1/activation.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"This post cannot go to that destination"}`))
	})
	mux.HandleFunc("POST /"+testAccount+"/fixtures/s2/activation.json", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	text, isError := mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "activate_fixture",
		"params": map[string]any{"fixture_id": "s1"},
	})
	if !isError || !strings.Contains(text, "403") || !strings.Contains(text, "This post cannot go to that destination") {
		t.Errorf("error = %q (isError=%v), want the 403 with the app's reason", text, isError)
	}

	text, isError = mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "activate_fixture",
		"params": map[string]any{"fixture_id": "s2"},
	})
	if !isError || !strings.Contains(text, "403") || !strings.Contains(text, "Access denied") {
		t.Errorf("error = %q (isError=%v), want a bare 403 as before", text, isError)
	}
}

func TestDispatchRefusesBadParamsWithoutARequest(t *testing.T) {
	_, session := connect(t, Config{}, refusingMailpiece(t))

	text, isError := mcptest.CallText(t, session, "mailpiece_fixtures", map[string]any{
		"action": "create_fixture",
		"params": map[string]any{},
	})
	if !isError || !strings.Contains(text, `missing required param "name"`) {
		t.Errorf("result = %q (isError=%v)", text, isError)
	}
}

func TestIdentityIsUnscoped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /my/identity.json", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"accounts": [{"name": "Acme", "slug": "/` + testAccount + `"}]}`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	payload := callJSON(t, session, "mailpiece_identity", map[string]any{"action": "get_identity"})
	if _, ok := payload["accounts"]; !ok {
		t.Errorf("payload = %v", payload)
	}
}

// Tokens are minted over MCP with --writes, outside the
// account prefix.
func TestAccessTokensAreUnscopedWrites(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /my/access_tokens.json", func(w http.ResponseWriter, r *http.Request) {
		token, _ := readBody(t, r)["access_token"].(map[string]any)
		if token["description"] != "CI" || token["permission"] != "read" {
			t.Errorf("access_token body = %v", token)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id": "t1", "token": "secret"}`))
	})
	_, session := connect(t, Config{}, fakeMailpiece(t, mux))

	payload := callJSON(t, session, "mailpiece_access_tokens", map[string]any{
		"action": "create_access_token",
		"params": map[string]any{"description": "CI", "permission": "read"},
	})
	if payload["id"] != "t1" {
		t.Errorf("payload = %v", payload)
	}
}
