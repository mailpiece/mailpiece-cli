// Package mcpserver assembles the MCP server behind `mailpiece mcp`: the
// hand-written Mailpiece tool catalog served through the shared toolkit's
// gateway, dispatching each action to the CLI's authenticated,
// account-scoped SDK client.
package mcpserver

import (
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/basecamp/mcp/gateway"

	"github.com/mailpiece/mailpiece-cli/internal/mcpserver/catalog"
	"github.com/mailpiece/mailpiece-cli/pkg/mailpiece"
)

// Name identifies the server in the MCP initialize handshake. The version
// is the CLI's own: `mailpiece mcp` is the CLI serving MCP, not a separate
// product.
const Name = "mailpiece-cli"

// Config selects the served tool surface.
type Config struct {
	// ReadOnly drops every write action from the catalog and refuses write
	// dispatch outright. The served default: writes are an explicit opt-in,
	// paired with a write token. The token's permission is the server-side
	// enforcement, this filter is the client-side surface.
	ReadOnly bool
	// Domains narrows the served domains by key ("identity", ...).
	// Empty means all. Unknown keys are a startup error: fail closed.
	Domains []string
	// Version is the CLI version reported in the initialize handshake.
	Version string
}

// Server wraps the toolkit gateway serving the catalog, dispatching
// through the CLI's SDK clients.
type Server struct {
	gw      *gateway.Server
	version string
}

// New validates the catalog and hands it to the gateway, which applies
// the config's domain and read-only filters. Tool calls dispatch through
// account (account-scoped operations) and root (the few unscoped ones,
// like identity get).
func New(account *mailpiece.AccountClient, root *mailpiece.Client, cfg Config) (*Server, error) {
	if account == nil || root == nil {
		return nil, fmt.Errorf("mcpserver: account and root API clients are required")
	}

	domains, err := catalog.Load()
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}

	srv := &Server{version: cfg.Version}
	dispatch := dispatcher{account: account, root: root}.handle
	gw, err := gateway.New(catalog.GatewayDomains(domains), gateway.Config{
		ReadOnly: cfg.ReadOnly,
		Domains:  cfg.Domains,
		Handler:  dispatch,
	})
	if err != nil {
		return nil, err
	}
	srv.gw = gw

	return srv, nil
}

// Domains returns the served domains.
func (s *Server) Domains() []gateway.Domain {
	return s.gw.Domains()
}

// BuildMCPServer constructs the SDK MCP server with one gateway tool per
// served domain.
func (s *Server) BuildMCPServer(logger *slog.Logger) *mcp.Server {
	return s.gw.BuildMCPServer(&mcp.Implementation{Name: Name, Version: s.version}, logger)
}
