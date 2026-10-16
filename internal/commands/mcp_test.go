package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mcpTestAccount = "834206594"

func TestMCPCommandRegistration(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"mcp"})
	if err != nil || cmd.Name() != "mcp" {
		t.Fatalf("mcp command not registered: %v", err)
	}

	writes := cmd.Flags().Lookup("writes")
	if writes == nil || writes.DefValue != "false" {
		t.Errorf("writes flag = %#v, want present and defaulting off (read-only is the served default)", writes)
	}
	if cmd.Flags().Lookup("domains") == nil {
		t.Error("domains flag missing")
	}
}

func TestMCPCommandRequiresAuth(t *testing.T) {
	resetMCPFlags(t)
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("", "", "https://api.example.com")
	defer resetTest()

	rootCmd.SetArgs([]string{"mcp"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "No API token configured") {
		t.Fatalf("err = %v, want auth error", err)
	}
	assertExitCode(t, err, 3)
}

// resetMCPFlags clears the mcp flag variables and their changed marks:
// cobra's Var flags accumulate across Execute calls within one process, and
// runMCP reads the environment only for a flag not given, so each test
// starts from the command's real defaults.
func resetMCPFlags(t *testing.T) {
	t.Helper()
	mcpWrites = false
	mcpDomains = nil
	for _, name := range []string{"writes", "domains"} {
		mcpCmd.Flags().Lookup(name).Changed = false
	}
}

// stubMCPTransport swaps the stdio transport for the server side of an
// in-memory pipe and returns the client side.
func stubMCPTransport(t *testing.T) (client, server mcp.Transport) {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	orig := mcpTransport
	mcpTransport = func() mcp.Transport { return serverTransport }
	t.Cleanup(func() { mcpTransport = orig })
	return clientTransport, serverTransport
}

// runMCPCommand runs `mailpiece mcp` against a stub Mailpiece upstream and
// connects a real MCP client to it over the transport seam. The command
// exits when the client session closes.
func runMCPCommand(t *testing.T, upstream *httptest.Server, args ...string) *mcp.ClientSession {
	t.Helper()

	resetMCPFlags(t)
	SetTestModeWithSDK(NewMockClient())
	SetTestSDK(upstream.URL) // repoint the SDK at the asserting upstream
	SetTestConfig("test-token", mcpTestAccount, upstream.URL)
	t.Cleanup(resetTest)

	clientTransport, serverTransport := stubMCPTransport(t)

	rootCmd.SetArgs(append([]string{"mcp"}, args...))
	// A command that fails before it serves never opens its end of the pipe, and the client's
	// handshake would wait on it forever: closing that end makes the client fail at once.
	done := make(chan error, 1)
	go func() {
		err := rootCmd.Execute()
		if err != nil {
			if conn, connectErr := serverTransport.Connect(context.Background()); connectErr == nil {
				_ = conn.Close()
			}
		}
		done <- err
	}()
	t.Cleanup(func() {
		if err := <-done; err != nil {
			t.Errorf("mailpiece mcp exited with error: %v", err)
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("MCP initialize failed: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func mcpRefusingUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	t.Cleanup(upstream.Close)
	return upstream
}

func mcpListTools(t *testing.T, session *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	tools := make([]*mcp.Tool, 0, 21)
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		tools = append(tools, tool)
	}
	return tools
}

func mcpToolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	slices.Sort(names)
	return names
}

func TestMCPCommandServesMCP(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/"+mcpTestAccount+"/fixtures.json" {
			t.Errorf("unexpected HTTP request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		// Tool calls must ride on the CLI's own credentials.
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q, want the CLI's token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"f1","name":"First"}]`))
	}))
	t.Cleanup(upstream.Close)

	session := runMCPCommand(t, upstream)

	if got := session.InitializeResult().ServerInfo.Name; got != "mailpiece-cli" {
		t.Errorf("server name = %q, want mailpiece-cli", got)
	}

	names := mcpToolNames(mcpListTools(t, session))
	if want := mcpToolCount(t, false); len(names) != want {
		t.Fatalf("tools = %v, want %d read-only mailpiece_* tools", names, want)
	}
	if slices.Contains(names, "mailpiece_access_tokens") {
		t.Error("read-only serves mailpiece_access_tokens, which has no read")
	}

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "mailpiece_fixtures",
		Arguments: map[string]any{"action": "list_fixtures", "params": map[string]any{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("fixtures list failed: %v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T", result.Content[0])
	}
	var fixtures []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(text.Text), &fixtures); err != nil || len(fixtures) == 0 || fixtures[0].Name != "First" {
		t.Fatalf("fixtures list result = %q (%v)", text.Text, err)
	}
}

func TestMCPCommandDefaultsToReadOnlyAndPassesDomainsThrough(t *testing.T) {
	session := runMCPCommand(t, mcpRefusingUpstream(t), "--domains", "fixtures")

	tools := mcpListTools(t, session)
	if len(tools) != 1 || tools[0].Name != "mailpiece_fixtures" {
		t.Fatalf("tools = %v, want just mailpiece_fixtures", mcpToolNames(tools))
	}
	if !strings.Contains(tools[0].Description, "- list_fixtures (RO)") {
		t.Error("read-only mailpiece_fixtures lost its read actions")
	}
	if strings.Contains(tools[0].Description, "- create_fixture:") {
		t.Error("mailpiece_fixtures lists a write action without --writes")
	}

	// A write is refused in-band, and nothing reaches the app.
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "mailpiece_fixtures",
		Arguments: map[string]any{"action": "create_fixture", "params": map[string]any{"name": "First"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("a write succeeded without --writes")
	}
}

func TestMCPCommandWritesOptIn(t *testing.T) {
	session := runMCPCommand(t, mcpRefusingUpstream(t), "--writes", "--domains", "fixtures")

	tools := mcpListTools(t, session)
	if len(tools) != 1 || tools[0].Name != "mailpiece_fixtures" {
		t.Fatalf("tools = %v, want just mailpiece_fixtures", mcpToolNames(tools))
	}
	if !strings.Contains(tools[0].Description, "- create_fixture:") {
		t.Error("--writes did not serve write actions")
	}
}

func TestMCPCommandWritesServesEveryTool(t *testing.T) {
	session := runMCPCommand(t, mcpRefusingUpstream(t), "--writes")

	names := mcpToolNames(mcpListTools(t, session))
	if want := mcpToolCount(t, true); len(names) != want {
		t.Fatalf("tools = %v, want %d with --writes", names, want)
	}
	for _, name := range []string{"mailpiece_access_tokens", "mailpiece_identity", "mailpiece_fixtures"} {
		if !slices.Contains(names, name) {
			t.Errorf("--writes does not serve %s", name)
		}
	}
}

// A desktop extension can set switches only through the environment; an
// unfilled form field arrives as a literal ${...} and counts as unset, and
// a flag given on the command line wins.
func TestMCPCommandEnvironment(t *testing.T) {
	cases := []struct {
		name       string
		writes     string
		domains    string
		args       []string
		wantTools  []string
		wantAll    bool
		wantWrites bool
	}{
		{name: "writes from the environment", writes: "true", domains: "fixtures", wantTools: []string{"mailpiece_fixtures"}, wantWrites: true},
		{name: "writes other than true stay off", writes: "yes", domains: "fixtures", wantTools: []string{"mailpiece_fixtures"}},
		{name: "unfilled writes stays off", writes: "${user_config.writes}", domains: "fixtures", wantTools: []string{"mailpiece_fixtures"}},
		{name: "domains with spaces", domains: "fixtures, identity", wantTools: []string{"mailpiece_fixtures", "mailpiece_identity"}},
		{name: "unfilled domains serves all", domains: "${user_config.domains}", wantAll: true},
		{name: "writes flag beats the environment", writes: "true", domains: "fixtures", args: []string{"--writes=false"}, wantTools: []string{"mailpiece_fixtures"}},
		{name: "domains flag beats the environment", domains: "fixtures", args: []string{"--domains", "identity"}, wantTools: []string{"mailpiece_identity"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("MAILPIECE_MCP_WRITES", tc.writes)
			t.Setenv("MAILPIECE_MCP_DOMAINS", tc.domains)

			session := runMCPCommand(t, mcpRefusingUpstream(t), tc.args...)
			tools := mcpListTools(t, session)
			names := mcpToolNames(tools)
			if tc.wantTools != nil && !slices.Equal(names, tc.wantTools) {
				t.Fatalf("tools = %v, want %v", names, tc.wantTools)
			}
			if want := mcpToolCount(t, false); tc.wantAll && len(names) != want {
				t.Fatalf("tools = %d, want %d", len(names), want)
			}
			for _, tool := range tools {
				if tool.Name != "mailpiece_fixtures" {
					continue
				}
				if got := strings.Contains(tool.Description, "- create_fixture:"); got != tc.wantWrites {
					t.Errorf("mailpiece_fixtures serves writes = %v, want %v", got, tc.wantWrites)
				}
			}
		})
	}
}

func TestMCPCommandUnknownDomainFailsClosed(t *testing.T) {
	resetMCPFlags(t)
	mock := NewMockClient()
	SetTestModeWithSDK(mock)
	SetTestConfig("test-token", mcpTestAccount, "https://api.example.com")
	defer resetTest()

	rootCmd.SetArgs([]string{"mcp", "--domains", "bogus"})
	err := rootCmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown domain "bogus"`) {
		t.Fatalf("err = %v, want unknown domain failure", err)
	}
}
