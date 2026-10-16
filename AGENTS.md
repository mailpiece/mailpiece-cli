# mailpiece CLI Development Context

Read `CONTRACT.md` first: it names the layout, the API, the lanes and the rules for this build.

This repository is a port of `auditans-cli` to Mailpiece, the newsletter app at
`/rails/vendor/other/mailpieces`. On the shell (sign-in, identity, access tokens, the raw API, output
and `mailpiece mcp`) sit three domains, each in its own files: posts (`post`, `search`), subscribers
(`subscriber`) and the newsletter (`snippet`, `journey`, `settings`, `user`). Where auditans-cli made
a choice, keep it.

## Repository Structure

```
mailpiece-cli/
├── cmd/mailpiece/        # Main entrypoint
├── pkg/mailpiece/        # SDK: client core plus one *_service.go per domain
├── internal/
│   ├── auth/            # Credential store (keyring, file fallback)
│   ├── commands/        # Command implementations, one file per command
│   ├── config/          # Configuration loading (YAML, env)
│   ├── errors/          # Error constructors over github.com/basecamp/cli/output
│   ├── harness/         # Coding-agent detection for doctor and skill install
│   ├── mcpserver/       # `mailpiece mcp` MCP server (catalog/ hand-written)
│   └── render/          # Output rendering (styled, markdown)
├── e2e/                 # Go integration tests (harness/, cli_tests/)
├── extension/           # MCPB desktop extension manifest
├── scripts/             # Release script
├── skills/              # Embedded agent skill
└── .claude-plugin/      # Claude Code integration
```

## SDK Architecture

Commands call the SDK in `pkg/mailpiece`:

- **`getSDK()`** returns `*mailpiece.AccountClient` bound to the resolved account. Call
  `requireAuthAndAccount()` first: it checks the token, the SDK, and resolves the account.
- **`getSDKClient()`** returns `*mailpiece.Client` for account-independent operations (identity).
- **`convertSDKError(err)`** converts SDK errors to `*output.Error`.
- **`normalizeAny(v)`** JSON-round-trips typed results to maps for the output layer.

Print with `printDetail`, `printList`, `printListPaginated` or `printMutation`, each with
breadcrumbs built by `breadcrumb(action, cmd, description)`.

## Configuration

Precedence, highest first: flags (`--token`, `--account`, `--profile`, `--api-url`), environment
(`MAILPIECE_TOKEN`, `MAILPIECE_ACCOUNT`, `MAILPIECE_PROFILE`, `MAILPIECE_API_URL`), the named profile
(`~/.config/mailpiece/config.json`), local config (`.mailpiece.yaml`), global config
(`~/.config/mailpiece/config.yaml`). With no account anywhere, the token's only account is used.

## Testing

```bash
make build            # Build binary to ./bin/mailpiece
make test             # Run Go unit tests (no API required; test-unit is an alias)
make test-e2e         # Run e2e tests (requires credentials; alias of make e2e)
make test-run NAME=TestOutputContractListCommands  # Run a specific e2e test
make surface-snapshot # Regenerate SURFACE.txt
go test ./internal/mcpserver/catalog -update   # Regenerate the MCP catalog snapshot
make check            # Local CI gate: fmt-check vet lint lint-actions tidy-check test-race
make check-all        # check, e2e
```

Requirements: Go 1.26+ and the linters pinned in `.mise.toml` (`mise install`), API credentials
for e2e tests.

Agents write tests and compile them with `go test -run '^$' ./...`; the owner runs them.

### Unit Test Patterns

Tests use `SetTestModeWithSDK(mock)` which creates an httptest server backed by `MockClient`:

```go
mock := NewMockClient()
mock.GetResponse = &APIResponse{Data: map[string]any{"id": "1"}}
SetTestModeWithSDK(mock)
SetTestConfig("token", "account", "https://api.example.com")
defer resetTest()
```

- `mock.OnGet(path, resp)`: route-specific GET responses
- `mock.WithGetData(data)`: set default GET response data
- `mock.WithListData(data)`: set GetWithPagination response (used as fallback for GET when GetResponse is default empty map and no `OnGet` route is set)
- JSON round-trip through httptest converts `int` to `float64`: use `float64(n)` in assertions
- `mockTest(t)` is that setup for a test signed in to the account `acme`: it returns the mock and
  the result, and resets when the test ends

The tests of paging, formats and `mailpiece mcp` run on fixtures that exist only in the test
binaries: a hidden `fixture` command
(`internal/commands/fixture_test.go`) and a `fixtures` MCP domain there and in
`internal/mcpserver/fixture_test.go`. They are left out of help, the catalogue, `SURFACE.txt` and
the catalog snapshot.

E2E environment variables (read from `.env.test`, see `.env.test.example`):
- `MAILPIECE_TEST_TOKEN` - API token (required)
- `MAILPIECE_TEST_ACCOUNT` - Account slug (required)
- `MAILPIECE_TEST_WRITE_ACCOUNT` - Scratch account for the tests that write (optional; they skip without it; never the demo account)
- `MAILPIECE_TEST_API_URL` - API base URL (optional)
- `MAILPIECE_TEST_BINARY` - Prebuilt binary path (optional)

## Checks

`make check` is the local CI gate: `fmt-check vet lint lint-actions tidy-check test-race`,
which is what the test workflow runs, minus govulncheck. Run it before pushing. There is no
`surface-check` in that list, but the surface gate still runs: `test-race` is
`go test -race ./internal/... ./pkg/...`, which includes
`internal/commands.TestSurfaceSnapshot`. `make test` runs the unit tests alone.
`make check-all` adds `e2e`; `make release-check` adds `replace-check`, `vuln` and
`secrets` instead. `lint-actions` needs actionlint, zizmor and shellcheck, pinned in
`.mise.toml`: `mise install` gets the versions CI uses.

`SURFACE.txt` at the repository root is a golden snapshot of the CLI's *structure*:
`CMD`, `SUB`, `FLAG` and `ARG` records, and that test compares against it. It does not
capture `Short`/`Long` help text or flag descriptions, so rewording help leaves the
snapshot green. So a change to the CLI surface fails
`make check` until the snapshot is refreshed. Regenerate with
`make surface-snapshot` (or run the gate alone with `make surface-check`) and commit
the result with the surface change.

## Adding a domain

Each of these registers itself; no shared list is edited.

- **Command**: `internal/commands/<resource>.go`, with `rootCmd.AddCommand(<resource>Cmd)` in its
  own `init()` and `Annotations: map[string]string{"group": "core"}` (or `"admin"`) on the top-level
  command. Set `Example:` on the command itself, not in `help.go`. Tests in `<resource>_test.go`.
- **SDK service**: `pkg/mailpiece/<domain>_service.go` with `New<Domain>Service(c *AccountClient)`,
  and its types in `pkg/mailpiece/types_<domain>.go`. Use `fetch`, `send`, `sendBare` and `listAt`;
  never edit `client.go` or `types.go`.
- **MCP domain**: `internal/mcpserver/catalog/domains_<lane>.go`, calling `Register(&Domain{...})`
  from its own `init()`, with the shorthands in `schema.go`. Files run in name order, which is the
  tool order.
- **Shared helpers**: an update sends only the flags passed, through `changedString` and
  `changedInt` (`root.go`). Rich text is a pair of flags, `--body` with `--body-file` (`-` is stdin),
  registered with `registerRichTextFlags` and read with `richTextFromFlags` (`markdown.go`). A paged
  list uses `pageFlags` and `pagedList` (`pages.go`). A write that cannot be taken back says "This
  cannot be undone." in its help and in its MCP summary (`cannotBeUndone`).
- **Never sent twice**: the client retries a POST only when its service marks it
  `WithIdempotent(ctx)`. A delivery and a test delivery are never marked.
- **Snapshots**: `make surface-snapshot` rewrites `SURFACE.txt` and
  `go test ./internal/mcpserver/catalog -update` rewrites `testdata/catalog_snapshot.txt`. Both are
  generated: when two lanes meet, regenerate, never merge by hand.

## Coding Style

### Go Import Ordering

1. Standard library
2. Third-party packages
3. Project-internal packages

Separate each group with a blank line.

### Naming Conventions

- Commands register themselves in `init()` with `rootCmd.AddCommand`
- Error constructors: `Err` prefix (e.g., `ErrNotFound`)
- Options: functional options pattern (`WithXxx`)

## Output Format

All commands produce structured output via the `github.com/basecamp/cli/output` package:
- Success: `{ok: true, data: ..., summary: "...", breadcrumbs: [...]}`
- Error: `{ok: false, error: "...", code: "...", hint: "..."}`
- Exit codes: 0=OK, 1=Usage, 2=NotFound, 3=Auth, 4=Forbidden, 5=RateLimit, 6=Network, 7=API, 8=Ambiguous
