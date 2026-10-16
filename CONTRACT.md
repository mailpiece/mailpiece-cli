# Mailpiece CLI: build contract

One contract for every agent working here. Read it once, then read only the files your lane names.

## What is being built

`mailpiece`, a Go command-line client of the Mailpiece JSON API, with an MCP server on stdio as one of
its subcommands (`mailpiece mcp`), so a person's own assistant can work with their newsletter. It is
never a standalone MCP server. It is a port of `/rails/vendor/other/auditans-cli`, itself modelled on
37signals' Fizzy stack: where auditans-cli made a choice, keep it.

- Repository root: `/rails/vendor/other/mailpiece-cli` (its own git repository)
- Go module: `github.com/mailpiece/mailpiece-cli`
- Binary: `mailpiece`, entry point `cmd/mailpiece/main.go`
- Go: 1.26.9, pinned in `.mise.toml`
- Auth: bearer token (personal access token). No OAuth, no PKCE, no callback server

## Where it stands

The shell is built: root command and flags, account resolution, `auth` (token and email code),
`signup`, `identity`, `token`, `config`, `api`, `show`, `doctor`, `setup`, `skill`, `completion`,
`version`, the update notice, the output layer, and `mailpiece mcp` with its dispatch and catalog
machinery. One read runs end to end as a command and as an MCP action: `mailpiece identity` and
`mailpiece_identity` / `get_identity`, both `GET /my/identity.json`.

The domain is built on it in three lanes, each a command, an SDK service and an MCP domain:

| Lane | Commands | SDK | MCP tools |
|---|---|---|---|
| posts | `post` (with `link`, `history`), `search` | `posts_service.go`, `search_service.go` | `mailpiece_posts`, `mailpiece_search` |
| subscribers | `subscriber` | `subscribers_service.go` | `mailpiece_subscribers` |
| newsletter | `snippet`, `journey` (with `email`), `settings`, `user` | `snippets_service.go`, `journeys_service.go`, `settings_service.go`, `users_service.go` | `mailpiece_snippets`, `mailpiece_journeys`, `mailpiece_settings`, `mailpiece_users` |

The lanes have met: the shared files below are open to edit again, one agent at a time.

## Layout

```
cmd/mailpiece/main.go
pkg/mailpiece/            SDK: client core plus one *_service.go and types_*.go per domain
internal/config/
internal/commands/       one file per command, each registering itself in init()
internal/mcpserver/      server.go, dispatch.go, catalog/ (one domains_<lane>.go per lane)
internal/harness/
skills/mailpiece/SKILL.md
```

The SDK is written by hand. There is no OpenAPI file and no generator.

## The API

The Rails app is at `/rails/vendor/other/mailpieces`. Read its routes, controllers and jbuilder
views to learn the shapes; never change it. In development it answers at
`http://app.mailpiece.localhost:3020`.

- Bearer tokens are accepted only on JSON requests: always request `.json` with `Accept: application/json`
- A token is `read` or `write`; `read` allows GET and HEAD only
- Account-scoped paths are prefixed with the account's slug, which is its number or its
  handle: `/834206594/posts.json`, `/acme-news/posts.json`. The number never changes; the handle is
  changed and cleared freely. `GET /my/identity.json` is unscoped and lists the accounts a token can
  reach, each with its `slug`, which is always the number: configuration keeps the number
- Ids are 25-character base36 strings
- Roles are `owner admin member system`; configuration is admins only, and a user reaches only
  their own profile
- Out of reach is 404, refused is 403, a read-only token writing is 401, an invalid record is 422
  with the errors as JSON
- Model only what the Rails app serves, and permit only the params its controller permits, named
  as it names them. Rails wraps JSON params under the resource's key where the controller says so

| Shell endpoint | Where its JSON is defined |
|---|---|
| `GET /my/identity.json` | `app/views/my/identities/show.json.jbuilder` |
| `GET/POST/DELETE /my/access_tokens` | `app/controllers/my/access_tokens_controller.rb` |
| `POST /session.json`, `POST /session/magic_link.json` | `app/controllers/sessions_controller.rb`, `sessions/magic_links_controller.rb` |
| `POST /signup/completion.json` | `app/controllers/signups/completions_controller.rb` |
| `GET /users/:id.json` (doctor's account check: your own user) | `app/controllers/users_controller.rb` |

## Configuration

Precedence: flags, then environment, then profile, then config file.

| Flag | Environment |
|---|---|
| `--token` | `MAILPIECE_TOKEN` |
| `--api-url` | `MAILPIECE_API_URL` |
| `--account` | `MAILPIECE_ACCOUNT` |
| `--profile` | `MAILPIECE_PROFILE` |

There is no default API URL: Mailpiece is self-hosted, so the address comes from `--api-url`,
`MAILPIECE_API_URL`, the profile or a config file. Plain HTTP is allowed for localhost only.

## File lanes, so agents can work at the same time

- SDK: one `pkg/mailpiece/<domain>_service.go` and one `pkg/mailpiece/types_<domain>.go` per domain,
  built with `New<Domain>Service(c *AccountClient)`. Never edit another lane's files, `types.go` or
  `client.go`
- Commands: one `internal/commands/<resource>.go` per resource, registering itself in `init()` with
  `rootCmd.AddCommand`, its group set by its own `"group"` annotation and its examples by its own
  `Example:`
- MCP: one `internal/mcpserver/catalog/domains_<lane>.go` per lane, registering its domains from
  `init()` with `Register`. Never edit `dispatch.go`
- Shared files nobody edits in a parallel lane: `root.go`, `output.go`, `commands.go`, `help.go`,
  `README.md`, `skills/mailpiece/SKILL.md`, `go.mod`, `go.sum`. Put what they would need in your report
- `SURFACE.txt` and `internal/mcpserver/catalog/testdata/catalog_snapshot.txt` are generated
  (`make surface-snapshot`, `go test ./internal/mcpserver/catalog -update`). Regenerate them for
  your own lane; when lanes meet they are regenerated again, never merged by hand

## Rules

- Work only inside `/rails/vendor/other/mailpiece-cli`. Change nothing anywhere else, the Rails app included
- Run no ruby at all: no `bin/rails`, no rake, no server
- Never commit, never push, never create a remote, unless the owner asks
- No speculative refactors, no features nobody asked for

## Done means

- `go build ./...`, `go vet ./...` and `go test ./...` pass, and `gofmt -l .` prints nothing
- Your report says what was written, what was run and what is unverified
