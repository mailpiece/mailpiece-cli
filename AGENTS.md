# Mailpiece CLI Development Context

`mailpiece` is a command-line client of the Mailpiece JSON API, with an MCP server on stdio as one of
its subcommands (`mailpiece mcp`). On the shell (sign-in, identity, access tokens, the raw API, output
and `mailpiece mcp`) sit the domains: posts (`post`, `search`), subscribers (`subscriber`) and the
newsletter (`snippet`, `journey`, `settings`, `user`).

## Repository Structure

```
mailpiece-cli/
├── cmd/mailpiece/        # Main entrypoint
├── pkg/mailpiece/        # SDK: client core plus one *_service.go per domain
├── internal/
│   ├── commands/        # Command implementations, one file per command
│   ├── config/          # Configuration loading (YAML, env)
│   ├── errors/          # Error constructors over github.com/basecamp/cli/output
│   ├── harness/         # Coding-agent detection for doctor and skill install
│   ├── mcpserver/       # `mailpiece mcp` MCP server (catalog/ hand-written)
│   └── render/          # Output rendering (styled, markdown)
├── e2e/                 # Go integration tests (harness/, cli_tests/)
├── scripts/             # Release script
├── skills/              # Embedded agent skill
└── .claude-plugin/      # Claude Code integration
```

## SDK Architecture

Commands call the SDK in `pkg/mailpiece`:

- **`getSDK()`** returns `*mailpiece.AccountClient` bound to the configured account, with a
  service per domain (`getSDK().Posts()`). Call `requireAuthAndAccount()` first, before any
  check of the flags: it checks the token, the account, and the SDK.
- **`getSDKClient()`** returns `*mailpiece.Client` for account-independent operations (identity).
- **`convertSDKError(err)`** converts SDK errors to `*output.Error`.
- **`normalizeAny(v)`** JSON-round-trips typed results to maps for the output layer.

Print with `printDetail`, `printList`, `printListPaginated` or `printMutation`, each with
breadcrumbs built by `breadcrumb(action, cmd, description)`.

## Mailpiece API Reference

The app's routes, controllers and jbuilder views (https://github.com/mailpiece/mailpiece) define
the shapes. The SDK is written by hand: there is no OpenAPI file and no generator.

- There is no default API URL: Mailpiece is self-hosted, so the address comes from `--api-url`,
  `MAILPIECE_API_URL`, the profile or a config file. Plain HTTP is allowed for localhost only
- Bearer tokens are accepted only on JSON requests: always request `.json` with
  `Accept: application/json`
- A token is `read` or `write`; `read` allows GET and HEAD only
- Account-scoped paths are prefixed with the account's slug, which is its number or its handle:
  `/834206594/posts.json`, `/acme-news/posts.json`. The number never changes; the handle is changed
  and cleared freely. `GET /my/identity.json` is unscoped and lists the accounts a token can reach,
  each with its `slug`, which is always the number: configuration keeps the number
- Ids are 25-character base36 strings
- Roles are `owner admin member system`; configuration is admins only, and a user reaches only
  their own profile
- Out of reach is 404, refused is 403, a read-only token writing is 401, an invalid record is 422
  with the errors as JSON
- Model only what the app serves, and permit only the params its controller permits, named as it
  names them. Rails wraps JSON params under the resource's key where the controller says so

## Configuration

Precedence, highest first: flags (`--token`, `--profile`, `--api-url`), environment
(`MAILPIECE_TOKEN`, `MAILPIECE_PROFILE`, `MAILPIECE_API_URL`), the named profile
(`~/.config/mailpiece/config.json`), local config (`.mailpiece.yaml`), global config
(`~/.config/mailpiece/config.yaml`). A saved profile's account always wins. `MAILPIECE_ACCOUNT` is
accepted as a deprecated alias for `MAILPIECE_PROFILE`.

## Authentication

Bearer personal access tokens; there is no OAuth. Run `mailpiece setup` for interactive
configuration, `mailpiece auth login` to save a token directly, or `mailpiece auth email` to get one
by email code. Tokens are kept in the system keyring, or in a file under the config directory when
`MAILPIECE_NO_KEYRING` is set or no keyring is available.

## Testing

```bash
make build            # Build binary to ./bin/mailpiece
make test-unit        # Run Go unit tests (no API required)
make test-e2e         # Run e2e tests (requires credentials; make test and make e2e are aliases)
make test-run NAME=TestOutputContractListCommands  # Run a specific e2e test
make surface-snapshot # Regenerate SURFACE.txt
go test ./internal/mcpserver/catalog -update   # Regenerate the MCP catalog snapshot
make check            # Local CI gate: fmt-check vet lint lint-actions tidy-check race-test
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

`make check` is the local CI gate: `fmt-check vet lint lint-actions tidy-check race-test`,
which is what the test workflow runs, minus govulncheck. Run it before pushing. There is no
`surface-check` in that list, but the surface gate still runs: `race-test` is
`go test -race ./internal/... ./pkg/...`, which includes
`internal/commands.TestSurfaceSnapshot`. `make test-unit` runs the unit tests alone, and
`make test` is the e2e suite. `make release-check` adds `replace-check`, `vuln` and `secrets`.
`lint-actions` needs actionlint, zizmor and shellcheck, pinned in
`.mise.toml`: `mise install` gets the versions CI uses.

`SURFACE.txt` at the repository root is a golden snapshot of the CLI's *structure*:
`CMD`, `SUB`, `FLAG` and `ARG` records, and that test compares against it. It does not
capture `Short`/`Long` help text or flag descriptions, so rewording help leaves the
snapshot green. So a change to the CLI surface fails
`make check` until the snapshot is refreshed. Regenerate with
`make surface-snapshot` (or run the gate alone with `make surface-check`) and commit
the result with the surface change.

## Adding a domain

- **Command**: `internal/commands/<resource>.go`, with `rootCmd.AddCommand(<resource>Cmd)` in its
  own `init()`, and its name in `commandCatalogGroups` (`commands.go`), or it drops out of help
  and the catalogue. Set `Example:` on the command itself, not in `help.go`. Tests in
  `<resource>_test.go`.
- **SDK service**: `pkg/mailpiece/<domain>_service.go` with `New<Domain>Service(c *AccountClient)`
  and a lazy accessor on `AccountClient` (`client.go`), and its types in
  `pkg/mailpiece/types_<domain>.go`. Use `fetch`, `send`, `sendBare` and `listAt`. A service sends
  what it is given: the command checks its flags, and the MCP server leaves that to the API's 422.
- **MCP domain**: `internal/mcpserver/catalog/domains_<domain>.go`, calling `Register(&Domain{...})`
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
  generated: regenerate them, never merge by hand.

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
