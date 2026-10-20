# Mailpiece CLI

Command-line client for Mailpiece, the newsletter app: one account is one newsletter, with its
posts and its subscribers.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mailpiece/mailpiece-cli/main/scripts/install.sh | bash
```

The installer downloads the latest release for your platform, checks it against the release's
checksums and puts `mailpiece` in `~/.local/bin` (or `$MAILPIECE_BIN_DIR`). With Homebrew:

```bash
brew install --cask mailpiece/tap/mailpiece
```

From a checkout, for development:

```bash
make build        # ./bin/mailpiece
```

## Sign in

Mailpiece is self-hosted, so the CLI has no default address: give your install's with `--api-url`,
`MAILPIECE_API_URL` or `mailpiece setup`, and it is saved with the profile. Create a personal access
token in Mailpiece under Profile › Personal access tokens, then:

```bash
pbpaste | mailpiece auth login --with-token --profile 834206594 --api-url https://your-install
mailpiece auth login TOKEN --profile 834206594 --api-url https://your-install   # also works, but lands in shell history
mailpiece auth email                  # or sign in with a code sent by email
mailpiece signup                      # new to Mailpiece: sign up by email code, then save a token
mailpiece auth list                   # saved profiles; mailpiece auth switch PROFILE picks one
mailpiece identity
mailpiece doctor
```

`mailpiece setup` walks through the same steps interactively, and saves them globally or to a local
`.mailpiece.yaml`. An account is named by its number, which is the slug the app gives and what is saved; a handle also answers but changes. For a local server, use
`--api-url http://app.mailpiece.localhost:3020`; plain HTTP is accepted for localhost only.

## Commands

Every command explains itself with `--help`; `mailpiece commands --json` lists them all with
their flags. Every `list` also answers to `ls`, every `show` to `view`, every `delete` to `rm`.
Run bare, piped or with `--agent`, `mailpiece` prints a quickstart JSON; in a terminal, the help.

### Core commands

| Command      | Subcommands                                                                                                                                      | Description                                                        |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------ |
| `post`       | `list`, `show`, `create`, `update`, `delete`, `publish`, `unpublish`, `schedule`, `unschedule`, `deliver`, `test`, `relist`, `unlist`, `link`, `history` | Write a post and release it to the web and to the list             |
| `subscriber` | `list`, `show`, `create`, `deactivate`, `activate`, `bounce`, `delete`                                                                           | The people on the newsletter's list                                |
| `snippet`    | `list`, `show`, `create`, `update`, `delete`                                                                                                     | Named pieces of text pasted into posts                             |
| `journey`    | `list`, `show`, `email show`, `email create`, `email update`, `email delete`, `email activate`, `email deactivate`, `email test`                 | The mail a newsletter sends by itself, by trigger                  |
| `search`     |                                                                                                                                                  | The posts and subscribers a query matches                          |
| `user`       | `show`, `update`                                                                                                                                 | Your own user on the account                                       |
| `show`       |                                                                                                                                                  | Any record by the URL of its page in Mailpiece, as JSON            |

`post deliver` mails the whole list and cannot be undone; `post test` and `journey email test` mail
one address. Text flags (`--body`, `--content`, `--description`) take Markdown or HTML, and each has a
`-file` twin that reads a file, or stdin with `-`. With no text flag, `post`, `snippet` and
`journey email` read the body from piped stdin, or open `$EDITOR` in a terminal; `-t` is the title
or subject and `-c` the body. `--attach FILE` (repeatable) on `post` and `journey email`
`create`/`update` uploads a file and adds it at the end of the body.

```bash
cat issue.md | mailpiece post create -t "Shipping week" --attach chart.png
cat new.md | mailpiece post update POST_ID      # replaces the body when nothing else is passed
```

### Account and admin

| Command    | Subcommands                                                              | Description                                               |
| ---------- | ------------------------------------------------------------------------ | --------------------------------------------------------- |
| `settings` | `newsletter`, `handle`, `sending`, `subscribe-form`, `join-code`, `notifications` | The newsletter's settings, each part with `show` and `update` |
| `auth`     | `login`, `email`, `status`, `list`, `switch`, `logout`                   | Sign in and out, and pick a profile                       |
| `token`    | `list`, `create`, `delete`                                               | Your personal access tokens                               |
| `identity` | `show`                                                                   | Your identity and the accounts you can reach              |

### Setup and tools


| Command      | Description                                                                                                                   |
| ------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `setup`      | Interactive setup wizard, global or local; `setup claude` installs the Claude Code plugin and links the skill               |
| `signup`     | Sign up, or generate a token, by email code: `start`, `verify`, `complete`                                                    |
| `doctor`     | Check CLI health and diagnose issues; `--verbose`, `--all-profiles`                                                           |
| `config`     | `show` and `explain` the resolved configuration                                                                               |
| `mcp`        | Serve Mailpiece to MCP clients over stdio                                                                                      |
| `api`        | Raw API access: `get`, `post`, `put`, `delete`; a read token can only GET, and the token goes to the configured API host only |
| `skill`      | Pick where to install the agent skill, or print it when piped; `skill install` installs it                                    |
| `commands`   | List every command                                                                                                            |
| `completion` | Shell completion scripts                                                                                                      |
| `version`    | Print version information                                                                                                     |


See `skills/mailpiece/SKILL.md` for worked examples.

## Configuration


| Flag        | Environment         |
| ----------- | ------------------- |
| `--token`   | `MAILPIECE_TOKEN`   |
| `--api-url` | `MAILPIECE_API_URL` |
| `--profile` | `MAILPIECE_PROFILE` |
|             | `MAILPIECE_ACCOUNT` |


Flags win over the environment, the environment over the saved profile, the profile over a
local `.mailpiece.yaml` (in the current directory or a parent), and that over the global config
file. The account comes from the profile. A login saves under the selected or default profile, or
else one named after the configured account; a sign-in saves under one named after the account.
`MAILPIECE_ACCOUNT` is a deprecated stand-in for `MAILPIECE_PROFILE` that names the account while
no profile is saved. `mailpiece config explain` shows why each value won.

## Output

Every command takes `--json`, `--quiet`, `--agent`, `--ids-only`, `--count`, `--limit`, `--styled`,
`--markdown`, `--jq` and `--verbose`; `--jq` is refused on `version`, `skill`, `setup` and `completion`.
A create shows the location the app sends back. Piped output is JSON by default. `mailpiece commands --json` lists every command.

### JSON envelope

Every command returns structured JSON:

```json
{
  "ok": true,
  "data": [...],
  "summary": "5 posts",
  "breadcrumbs": [{"action": "show", "cmd": "mailpiece post show <id>"}]
}
```

An error is `{"ok": false, "error": "...", "code": "...", "hint": "..."}`. Breadcrumbs suggest the
next commands, for people and agents alike.

## Agents

**Claude Code:** `mailpiece setup claude` adds this repository as a Claude Code plugin
marketplace, installs the `mailpiece` plugin from it, and links the embedded skill into Claude's
skills directory.

`mailpiece skill` asks where to install the embedded agent skill (Agents, Claude Code, OpenCode,
Codex or a custom path), and prints it when piped; `mailpiece skill install` installs it for coding
agents. The skill covers setting up a self-hosted Mailpiece as well as using it. An installed skill
is refreshed after an upgrade.

`mailpiece mcp` serves Mailpiece as MCP tools over stdio, on your signed-in account:

```bash
claude mcp add mailpiece -- mailpiece mcp       # Claude Code
mailpiece mcp --writes                         # serve changes too (pair with a write token)
mailpiece mcp --domains posts,subscribers       # narrow the served tools
```

It is read-only unless `--writes` is passed; then the write actions are served too. The domains are
`posts`, `search`, `subscribers`, `snippets`, `journeys`, `settings`, `users`, `identity` and
`access_tokens`.

## Troubleshooting

```bash
mailpiece doctor                 # Install, config, auth, API and agent health check
mailpiece doctor --profile acme  # Check one saved profile
mailpiece doctor --all-profiles  # Sweep every saved profile
mailpiece doctor --verbose       # Include effective config details
mailpiece doctor --json          # Structured output for scripts and support
```

Common follow-up commands:

```bash
mailpiece auth status
mailpiece config show
mailpiece config explain
mailpiece identity show
mailpiece setup
mailpiece setup claude
mailpiece skill install
```

## Development

```bash
make build              # Build binary to ./bin/mailpiece
make surface-snapshot   # Regenerate SURFACE.txt after changing a command or flag
make test-unit          # Run unit tests, CLI and SDK (no API required)
make e2e                # Run owner-only CLI contract e2e suite
make e2e-run NAME=TestOutputContractListCommands
make check              # Local CI gate: fmt-check, vet, lint, lint-actions, tidy-check, race-test
make release-check      # check, replace-check, vuln, secrets
```

E2E requirements, read from `.env.test` (copy `.env.test.example`):

- `MAILPIECE_TEST_TOKEN`
- `MAILPIECE_TEST_ACCOUNT`
- optional: `MAILPIECE_TEST_API_URL`
- optional: `MAILPIECE_TEST_BINARY`
- optional: `MAILPIECE_TEST_WRITE_ACCOUNT`, a scratch account for the tests that write; without
it they skip. Never the demo account.



## License

MIT.
