---
name: mailpiece
description: |
  Interact with Mailpiece, the newsletter app, via the mailpiece CLI. Write, test, publish,
  schedule and mail a newsletter's posts; read and manage its subscribers; keep its snippets,
  its journeys (the mail it sends by itself) and its settings. Sign in with a personal access
  token or an email code, see the newsletters (accounts) you can reach and your role in each,
  manage your access tokens, and read any record by its URL or through the raw API. Also sets up
  a self-hosted Mailpiece with Docker. Use for ANY Mailpiece question or action.
triggers:
  # Direct invocations
  - mailpiece
  - /mailpiece
  # Common actions
  - write a post
  - draft a newsletter issue
  - publish a post
  - schedule a post
  - send the newsletter
  - send a test email
  - list subscribers
  - add a subscriber
  - who unsubscribed
  - which addresses bounced
  - search the newsletter
  - newsletter snippet
  - welcome email
  - journey email
  - newsletter settings
  - sender address
  - join code
  - sign in to mailpiece
  - which newsletters can I reach
  - mailpiece access token
  # Setup
  - install mailpiece
  - set up mailpiece
  - self-host mailpiece
  - host my newsletter
invocable: true
argument-hint: "[action] [args...]"
---

# /mailpiece - Mailpiece Workflow Command

`mailpiece` is a command-line client of the Mailpiece JSON API. It can do what the signed-in user may
do in Mailpiece, no more: the app enforces every permission. One account is one newsletter, with its
posts, its subscribers, its snippets, its journeys and its settings; nearly all of that is for admins,
and a member is refused with 403. `mailpiece mcp` serves the same over MCP.

## Agent Invariants

**MUST follow these rules:**

1. **Read first.** Anything that writes needs a `write` token and changes a real newsletter. Ask
   before any write the person did not ask for.
2. **Refusals are not retried.** A 403 is a refusal and a 404 is out of reach: report it, with the
   reason when the 403 gives one. A 401 on a write can mean a read token.
3. **An account is named by its slug**, and the slug the app gives is always its number (e.g. `834206594`):
   `mailpiece identity` shows it. A handle (e.g. `acme-news`) also answers in a path but changes, so
   configuration keeps the number.
4. **Use built-in `--jq`** to reduce output: `mailpiece identity --jq '[.data.accounts[] | {slug, name}]'`.
   Never pipe to external jq. `--jq` implies `--json`.
5. **Check breadcrumbs** in responses for next actions with values filled in.
6. **Use `mailpiece doctor` for setup, config and auth issues** before guessing.
7. **Never put a session token on the command line.** `auth email complete` and `signup complete` read it from stdin.
8. **Mail that has left cannot be taken back.** Never run `post deliver`, `post schedule` with
   `--deliver` or a mailing `--destination`, or `journey email activate`, unless the person asked
   for exactly that. `post test` and `journey email test` send real mail to one address.
9. **Only the person undoes an unsubscribe or a bounce**, by subscribing again from the public form.
   No command does it for them: never suggest one.

## Setting up Mailpiece

When the person wants to install or self-host Mailpiece, follow these steps in order. The docs are
the source: fetch https://mailpiece.dev/docs/install.md and https://mailpiece.dev/docs/configuration.md
when anything here is unclear. **Never invent a credential, a domain or an address.** Ask, and wait.

1. **Ask for what only the person has**: a server with Docker and ports 80 and 443 open (or a laptop
   to try it); a domain pointed at it, such as `app.example.com` (not on a laptop); the sender
   address for the app's own mail, such as `My Newsletter <hello@example.com>`; their mail provider
   and its credentials.
2. **Install with Docker.** Make the keys once, on the server, and **tell the person to keep all four
   safe**: without the same encryption keys the data cannot be read again.

   ```bash
   openssl rand -hex 64      # SECRET_KEY_BASE
   openssl rand -base64 32   # each ACTIVE_RECORD_ENCRYPTION_* key
   ```

   Run it with the volume on `/rails/storage` (without it the databases, uploads and certificate are
   lost with the container), and the mail settings from step 3 as more `--env` lines:

   ```bash
   docker run -d --name mailpiece --restart unless-stopped \
     --publish 80:80 --publish 443:443 \
     --volume mailpiece:/rails/storage \
     --env SECRET_KEY_BASE=... \
     --env ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY=... \
     --env ACTIVE_RECORD_ENCRYPTION_DETERMINISTIC_KEY=... \
     --env ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT=... \
     --env TLS_DOMAIN=app.example.com \
     --env BASE_URL=https://app.example.com \
     --env "MAILER_FROM_ADDRESS=My Newsletter <hello@example.com>" \
     ghcr.io/mailpiece/mailpiece:main
   ```

   Behind a proxy that handles HTTPS, leave `TLS_DOMAIN` out and publish port 80 only. On a laptop:
   `--publish 80:80 --env DISABLE_SSL=true --env BASE_URL=http://localhost`, no `TLS_DOMAIN`.
3. **Mail.** System mail (sign-in codes) uses `SMTP_ADDRESS`, `SMTP_USERNAME`, `SMTP_PASSWORD`, from
   `MAILER_FROM_ADDRESS`; newsletter mail uses `NEWSLETTER_MAIL_*` only, never `SMTP_*`. Fetch
   `https://mailpiece.dev/integrations/<slug>.md` and set exactly the variables it lists, where the
   slug is one of `resend`, `amazon-ses`, `postmark`, `mailgun`, `sendgrid`, `brevo`, `mailjet`,
   `mailersend`, `smtp2go`. Any other provider works over SMTP (https://mailpiece.dev/docs/mail.md):

   ```bash
   NEWSLETTER_MAIL_TRANSPORT=smtp
   NEWSLETTER_MAIL_SMTP_ADDRESS=smtp.example.com
   NEWSLETTER_MAIL_SMTP_PORT=587
   NEWSLETTER_MAIL_SMTP_USERNAME=...
   NEWSLETTER_MAIL_SMTP_PASSWORD=...
   ```

   It must be encrypted: STARTTLS on 587, or `NEWSLETTER_MAIL_SMTP_TLS=true` on a port such as 465.
   Tell the person **bounces aren't recorded in Mailpiece with these providers yet**: they check the
   provider's suppression list and deactivate those subscribers. A changed setting needs the
   container run again (`docker rm -f mailpiece`, then the same `docker run` with the same keys and
   volume).
4. **Open it** at `https://app.example.com/signup` (`http://localhost/signup` on a laptop). While no
   account exists the sign-in code is shown on screen, so the person signs in and names the
   newsletter without any mail. The number in the address after signing in is the account number.
5. **Connect the CLI.** The person makes a token under **Profile › Personal access tokens**, choosing
   **Read + Write**, and copies it. Then they run `pbpaste | mailpiece auth login --with-token
   --account NUMBER --api-url https://app.example.com` (`http://localhost` on a laptop), which reads
   the token from stdin and keeps it out of your transcript and their shell history. Then
   `mailpiece doctor`; fix what it reports before going on. Connect over MCP with `mailpiece mcp --writes`.
6. **Send a test issue** to the person alone, from an address the provider lets them send from:

   ```bash
   mailpiece settings sending update --from-name "My Newsletter" --from-address hello@example.com
   mailpiece post create --title "Hello" --body "First test."
   mailpiece post test POST_ID --to PERSON_EMAIL
   ```

   **Never run `post deliver`** here. If the test does not arrive, look in the provider's email log.

**If it fails:**

- **No image**: if `ghcr.io/mailpiece/mailpiece:main` won't pull, say so; don't pull another image.
  To build their own, the person installs with Kamal (https://mailpiece.dev/docs/kamal.md).
- **Data can't be read after a restart**: the keys changed. Run it again with the original four;
  never make new ones for an install that has data.
- **Redirect loop or HTTPS errors with no certificate**: `DISABLE_SSL=true` only when there is no
  HTTPS at all (a laptop); behind a proxy that does HTTPS, leave it unset.
- **Sign-in code doesn't arrive** (after the first account): `SMTP_*` is unset or wrong.
- **A newsletter message fails**: `NEWSLETTER_MAIL_TRANSPORT` is unset or a setting is wrong; nothing
  is checked at start.
- Anything else: `mailpiece doctor`, and `docker logs mailpiece`.

## Quick Reference

| Resource | List | Show | Create | Other |
|----------|------|------|--------|-------|
| post | `post list`, `post list --published`, `post list --sent` | `post show POST_ID` | `post create` | `post update POST_ID`, `post delete POST_ID` |
| post release | - | - | - | `post publish`, `post unpublish`, `post schedule`, `post unschedule`, `post deliver`, `post test`, `post relist`, `post unlist` (each takes `POST_ID`) |
| post link | `post link list POST_ID` | - | - | `post link update POST_ID LINK_ID --url URL` |
| post history | `post history list POST_ID` | `post history show POST_ID REVISION_ID\|latest` | - | - |
| search | `search QUERY...` | - | - | - |
| subscriber | `subscriber list [--status STATUS]` | `subscriber show SUBSCRIBER_ID [--page N]` | `subscriber create --email EMAIL` | `subscriber deactivate SUBSCRIBER_ID`, `subscriber activate SUBSCRIBER_ID`, `subscriber delete SUBSCRIBER_ID` |
| snippet | `snippet list` | `snippet show SNIPPET_ID` | `snippet create --name NAME --content TEXT` | `snippet update SNIPPET_ID`, `snippet delete SNIPPET_ID` |
| journey | `journey list` | `journey show TRIGGER` | - | - |
| journey email | `journey show TRIGGER` | `journey email show TRIGGER EMAIL_ID` | `journey email create TRIGGER --subject S --body TEXT` | `journey email update\|delete\|activate\|deactivate\|test TRIGGER EMAIL_ID` |
| settings | - | `settings newsletter\|handle\|sending\|subscribe-form\|bounces\|join-code\|notifications show` | - | `settings newsletter\|handle\|sending\|subscribe-form\|join-code\|notifications update`, `settings join-code reset` |
| user | - | `user show USER_ID` | - | `user update USER_ID --name NAME` |
| identity | - | `identity` | - | - |
| token | `token list` | - | `token create` | `token delete TOKEN_ID` |
| show | - | `show URL` | - | - |
| api | `api get PATH` | `api get PATH` | `api post PATH -d JSON` | `api put PATH -d JSON`, `api delete PATH` |

Every `list` also answers to `ls`, every `show` to `view`, and every `delete` to `rm`. A paged
list takes `--page N` or `--all`.

Rich text (`--body`, `--content`, `--description`) is given as Markdown or HTML and stored as HTML.
Each has a `-file` twin (`--body-file`, `--content-file`, `--description-file`) that reads a file, or
stdin with `-`; the two are not passed together. A record answers both forms: `body` / `content` /
`description` is the text alone, and `body_html` / `content_html` / `description_html` is what was
written.

With no text flag, `post`, `snippet` and `journey email` create and update read the body from piped
stdin (on a terminal they open `$EDITOR`); an update does so only when no other field is passed.
`-t` is the title and `-c` the body or content. `--attach FILE` (repeatable) on `post create` and
`post update` uploads the file and adds it at the end of the body; `post update ID --attach FILE`
alone appends to the current body. For exact placement, write the `<action-text-attachment sgid=...>`
tag yourself.

```bash
cat issue.md | mailpiece post create -t "Shipping week"
mailpiece post create -t "Charts" -c "This week in numbers." --attach chart.png
cat new.md | mailpiece post update POST_ID
```

---

## Global Flags

All commands support these global flags unless noted otherwise:

| Flag | Description |
|------|-------------|
| `--token TOKEN` | API access token |
| `--account SLUG` | Account slug, its number (e.g. 834206594); a handle answers too, until it changes: one identity can hold users on several accounts |
| `--profile NAME` | Named profile to use |
| `--api-url URL` | Address of the Mailpiece install; there is no default (plain http for localhost only) |
| `--jq EXPR` | Built-in jq filter (implies `--json`, or filters raw data with `--quiet`/`--agent`). Refused on `version`, `skill`, `setup` and `completion`; not with `--styled`, `--markdown`, `--ids-only` or `--count` |
| `--json` | JSON envelope output |
| `--quiet` | Raw JSON data without envelope |
| `--styled` | Styled terminal output |
| `--markdown` | Markdown output |
| `--agent` | Agent mode: quiet format, no interactive prompts |
| `--ids-only` | Print one ID per line |
| `--count` | Print count of results |
| `--limit N` | Show at most N results |
| `--verbose` | Show request and response details |

Output defaults to styled in a terminal and JSON when piped.

Bare `mailpiece`, piped or with `--agent`, prints a quickstart JSON (version, auth state, quick-start
and common commands); in a terminal it prints the help.

---

## Configuration

```
~/.config/mailpiece/
├── config.yaml               # Global config: token, account, api_url
└── config.json               # Saved profiles (tokens in the system keyring, or a file fallback)

.mailpiece.yaml                # Local config, in the working directory or a parent: token, account, api_url
```

**Local config:** `.mailpiece.yaml`
```yaml
account: "834206594"
api_url: https://mailpiece.example.com
```

It may also hold `token`. `mailpiece setup` writes it with mode 0600 when you choose Local; add it to
`.gitignore`.

**Priority (highest to lowest):**
1. Flags (`--token`, `--account`, `--profile`, `--api-url`)
2. Environment (`MAILPIECE_TOKEN`, `MAILPIECE_ACCOUNT`, `MAILPIECE_PROFILE`, `MAILPIECE_API_URL`)
3. The named profile (`config.json`)
4. Local config (`.mailpiece.yaml`)
5. Global config (`config.yaml`)

Mailpiece is self-hosted, so there is no default address: a command that needs one and has none
fails with "No Mailpiece address configured". With no account anywhere, the token's only account is
used. A default profile outranks a local
`.mailpiece.yaml` for the account and token; `setup` warns when that happens.

**Check context:**
```bash
mailpiece config show          # effective values; token as {configured, source}
mailpiece config explain       # every candidate, and why each value won
mailpiece auth status          # where the token and the account came from (no API call)
mailpiece doctor               # full health check with hints
```

**Setup and sign-in:**
```bash
mailpiece setup                                  # interactive: API URL, token, account, Global or Local, the skill
op read "op://<vault>/<item>/credential" | mailpiece auth login --with-token --account 834206594 --api-url https://mailpiece.example.com
                                                 # save a token, read from stdin, under a profile named after the account
... | mailpiece auth login --with-token --profile agent --account 834206594
mailpiece auth login TOKEN --account 834206594   # the token as an argument works too, but lands in shell history
mailpiece auth status
mailpiece auth list                              # saved profiles, with the active one
mailpiece auth switch PROFILE                    # make a profile the default
mailpiece auth logout                            # remove the current profile's credentials
mailpiece auth logout --all
mailpiece identity                               # your identity, each account's slug, and your role there
```

Create a token in Mailpiece under Profile › Personal access tokens. A `read` token allows GET only.
`setup` asks "Reconfigure?" when a config or profiles exist. Logging out of the default profile
also clears the global account.

### Signing in by Email Code

Interactive:
```bash
mailpiece auth email [--write]
```

Step by step (for agents):
```bash
# Step 1: send a one-time code
mailpiece auth email start --email person@example.com
# Returns: {"pending_authentication_token": "..."}

# Step 2: the person reads the code from their email
mailpiece auth email verify --code CODE --pending-token TOKEN
# Returns: {"session_token": "...", "requires_signup_completion": false, "accounts": [{"id": "...", "name": "...", "slug": "..."}]}

# Step 3: keep the session token out of shell history and the conversation
echo "SESSION_TOKEN" > /tmp/mailpiece-session && chmod 600 /tmp/mailpiece-session

# Step 4: generate and save an access token (session token on stdin)
mailpiece auth email complete [--account SLUG] [--write] < /tmp/mailpiece-session

# Step 5: clean up
rm /tmp/mailpiece-session
```

`--account` is needed when the identity reaches more than one account. In a terminal, `complete`
asks for the session token with a masked prompt. An identity that has not finished signing up comes
back with `requires_signup_completion: true` and a breadcrumb to `mailpiece signup complete --name`.

### Signing Up (New User or Token Generation)

Interactive:
```bash
mailpiece signup [--write]                       # full interactive wizard
```

Step by step (for agents):
```bash
# Step 1: send a one-time code
mailpiece signup start --email person@example.com
# Returns: {"pending_authentication_token": "..."}

# Step 2: the person reads the code from their email
mailpiece signup verify --code CODE --pending-token TOKEN
# Returns: {"session_token": "...", "requires_signup_completion": true/false}
# For existing users (requires_signup_completion=false), also returns: "accounts": [{"id": "...", "name": "...", "slug": "..."}]

# Step 3: keep the session token out of shell history and the conversation
echo "SESSION_TOKEN" > /tmp/mailpiece-session && chmod 600 /tmp/mailpiece-session

# Step 4a: new user: complete signup, which creates their account (session token on stdin)
mailpiece signup complete --name "Full Name" [--write] < /tmp/mailpiece-session
# Returns: {"authenticated": true, "account": "SLUG", "permission": "read", "is_new_user": true}

# Step 4b: existing user: generate a token for an account
mailpiece signup complete --account SLUG [--write] < /tmp/mailpiece-session

# Step 5: clean up
rm /tmp/mailpiece-session
```

The app signs a known email in and a new one up, so `signup` and `auth email` share the start and
verify steps. `complete` takes `--name` or `--account`, not both. The token is saved as `auth email`
saves it, and is never printed. Signing up creates an account: ask first.

### Personal Access Tokens

```bash
mailpiece token list                                               # id, description, permission, created; never the token
mailpiece token create --description "CI" --permission read|write   # shown once: save it
mailpiece token delete TOKEN_ID
```

---

## Response Structure

```json
{
  "ok": true,
  "data": { ... },           // single object or array
  "summary": "...",          // human-readable description
  "notice": "...",           // e.g. more pages exist (omitted when empty)
  "breadcrumbs": [ ... ],    // next actions (omitted when empty)
  "context": { ... }         // location, pagination (omitted when empty)
}
```

**Lists with more pages:**
```json
{
  "ok": true,
  "data": [ ... ],
  "context": {
    "pagination": { "has_next": true, "next_url": "https://..." }
  }
}
```

**Breadcrumbs:** each has `action`, `cmd` (ready to run, values filled in) and `description`.
Placeholders like `<id>` need replacing.

```bash
mailpiece identity --jq '.breadcrumbs'
```

**Creates include the record's location:**
```json
{
  "ok": true,
  "data": { ... },
  "context": { "location": "URL of the new record, as the app sends it" }
}
```

Styled output shows `Location: URL`; markdown shows `**Location:**`.

**Errors:**
```json
{ "ok": false, "error": "Not Found", "code": "not_found", "hint": "optional context" }
```

A validation error names each field as `field: message`.

---

## Posts

A post is a draft until it is released. It is **published once and sent once**: publishing gives it a
page, delivering mails it to the list, and neither happens twice. A scheduled post is still a draft.
A published post can still be edited: its page shows the correction, the email already sent does not
change. All of this is for admins.

**Never run `post deliver`, or `post schedule` with `--deliver` or a mailing `--destination`, unless
the person asked for exactly that.** The mail leaves for the whole list and cannot be taken back.
`post delete` is for good as well, in any state.

### Reading

- `post list` is what is ahead: every draft, newest first, then every scheduled post, soonest first,
  on one page. `--published` (published, not mailed) and `--sent` (mailed) are paged: `--page N`, `--all`.
- A post's state is read from its fields: `published` and `published_at`, `sent_at`,
  `scheduled_posting_at` with `scheduled_delivery`, and `visibility` (`listed`, `unlisted`: reached by
  its link only, `email_only`: no page). `public_url` is there only while the post has a page.
- `deliveries_count` is how many subscribers the post was mailed to. A single post (`post show`) also
  carries `brought_subscribers_count`, how many signed up from it, and `liked_count` and
  `disliked_count`, how readers rated it; the posts list does not.
- `body` is the text as plain text. `body_html` is the text as it was written: edit from that one.
- `search` answers `{posts, subscribers}`, both whole, newest first: the app pages the subscribers
  and the command fetches every page. It is also how a subscriber is found by email:
  `mailpiece search ana@example.com`.

### Writing

```bash
mailpiece post create --title "Hello" --summary "What this one is about" --body-file hello.md
mailpiece post update POST_ID --body-file hello.md     # only what is passed changes
cat hello.html | mailpiece post update POST_ID --body-file -
```

The title is also the email's subject; the summary, when there is one, stands for the post in place
of how it begins.

**Audience boxes pass through untouched.** Text for one audience only is kept in the body as
`<aside data-audience="web">…</aside>` or `<aside data-audience="email">…</aside>`. When editing a
post, start from `body_html` and send every box back exactly as it was. Write the opening and closing
tags at the start of a line; Markdown inside a box is converted only when blank lines separate it
from the tags:

```markdown
<aside data-audience="web">

Read by people on the **web** only.

</aside>
```

`post update --published-at` corrects the date of a published post that is on the web, to now or
earlier. For any other post the app drops it without a word.

### Releasing

| Command | What it does |
|---------|--------------|
| `post test POST_ID [--to EMAIL]` | Mails the post as it stands to you, or to one address. Changes nothing. At most 10 in 3 minutes |
| `post publish POST_ID` | Gives the post its page and **mails nobody**. `--destination web` (listed) or `link` (unlisted). `--published-at`, or `--day` with `--hour`, releases a draft under a past date. Gives a page to an issue mailed by email only |
| `post unpublish POST_ID` | Takes the page down. A post never mailed is a draft again; a mailed issue stays sent, by email only, and its links stop answering until it is published again |
| `post schedule POST_ID --at TIME` | Releases the draft by itself at a time still to come (`--at`, or `--day` with `--hour`). Publishes only, unless `--deliver` or a mailing `--destination` (`web_and_email`, `email`) is passed. Scheduling again moves the time |
| `post unschedule POST_ID` | Cancels the schedule; the post stays a draft |
| `post deliver POST_ID` | Mails the post to everyone active on the list, now, publishing a draft first. **Cannot be undone.** A post already sent is mailed to nobody again, and the summary says which happened |
| `post relist POST_ID` / `post unlist POST_ID` | Shows a published post in the archive and the feed, or keeps its page out of both |

- Times are ISO 8601 with an offset (`2030-01-01T09:00:00-03:00`). `--day` and `--hour` are read in
  the app's time zone.
- After `post deliver`, read the `summary`: the mailing started with this command ("Mailing started",
  with `deliveries_count` how many are mailed), the post was already sent and nobody was mailed again,
  or nobody is active on the list, so the post was published and is still unsent (`sent_at` is null).
- A 403 on a release is a refusal by the post's state, not by your role: an email-only draft cannot be
  published without being sent, only a draft is scheduled, only a published post on the web is listed
  or unlisted, and a mailing destination is offered only while the newsletter has a sender and someone
  active on its list. The error carries the reason, as a 422 does (no sender, a time not ahead, a date
  not past).
- A good order for a new issue: `post create`, `post test`, then ask before `post deliver`.

### Links and history

- `post link list POST_ID`: the links a mailed issue went out with. `original_url` is what the email
  has, `url` where it sends a reader now, and `stale` marks one the post's text no longer has.
  `post link update POST_ID LINK_ID --url URL` corrects where it goes from now on.
- `post history list POST_ID`: the post's earlier versions, oldest first.
  `post history show POST_ID latest` reads the newest with its title and text. Revisions are read only.

---

## Subscribers

The people on the newsletter's list. Admins only.

A subscriber's `status` is one of, read in this order: `bounced` (the mail provider stopped
delivering to the address), `deactivated` (an admin stopped it), `unsubscribed` (the person left),
`confirmed`, `unconfirmed` (it has not answered its confirmation yet). Only a `confirmed` subscriber
is mailed.

```bash
mailpiece subscriber list                          # The confirmed ones: who an issue is mailed to
mailpiece subscriber list --status bounced         # Also: unconfirmed, unsubscribed, deactivated
mailpiece subscriber list --all                    # Every page
mailpiece subscriber show SUBSCRIBER_ID            # The subscriber and the issues mailed to it
mailpiece subscriber show SUBSCRIBER_ID --page 2   # Older deliveries
mailpiece search ana@example.com                   # Find a subscriber by email, in any state
mailpiece subscriber create --email ana@example.com
mailpiece subscriber deactivate SUBSCRIBER_ID      # Stop mailing it, keep it and its history
mailpiece subscriber activate SUBSCRIBER_ID        # Resume one an admin stopped
mailpiece subscriber delete SUBSCRIBER_ID          # For good
```

`subscriber show` carries `deliveries`, newest first: each has its own `id` and `created_at`, its
`post` (id, title, url) and a `status` of `sent`, `refused`, `unverified` (handed over and never
answered, so never sent again) or `pending`.

Every subscriber carries `source_post` (the post it signed up from: id, code, title, url, or null)
and `referrer` (the page it came from, or null).

To find a subscriber by email, use `search`: its `subscribers` are the ones the query matches, in any
state, each with its `id`. No list is filtered by address.

**MUST:**

1. **Only the person undoes an unsubscribe or a bounce**, by subscribing again from the public form
   and confirming from the link mailed to them. No command does it for them: never suggest one.
2. **`create` adds an address already confirmed.** No confirmation is mailed and the confirmation's
   journey does not start. An address already on the list is answered as it is. One that is not an
   address, or that unsubscribed, bounced or was stopped by an admin, is refused (422), and the error
   says which of the four it was: report it as it is and do not retry it.
3. **`deactivate` and `activate` are the admin's own switch** and reverse neither an unsubscribe nor
   a bounce. `deactivate` is refused (403, with the reason) for a subscriber that unsubscribed or
   bounced. `activate` on one that was not stopped changes nothing, and its `summary` says so.
4. **`delete` is for good and cannot be undone**: the subscriber goes with its deliveries and its
   journey history, and nothing then remembers that the address unsubscribed or bounced, so it
   could be added again as new. Confirm with the person before running it, and never use it to get
   an unsubscribed or bounced address back onto the list unless they ask for exactly that. To stop
   mailing someone, use `deactivate`.

Importing a file of addresses, adding several at once and the public subscription form have no
command: they are done in the app.

---

## Snippets, journeys, settings and your user

Everything here but `user` is for admins: a member is refused with 403.

### Snippets

A snippet is a name and a piece of rich text the newsletter keeps. The editor pastes it into a post as
ordinary text, so nothing in a post points back at it: updating or deleting a snippet never changes a post
already written. `snippet list` is not paged and carries every snippet's content.

```bash
mailpiece snippet create --name Signature --content "Thanks for reading, **Ana**"
mailpiece snippet update SNIPPET_ID --content-file signature.md
mailpiece snippet delete SNIPPET_ID     # for good
```

### Journeys

A journey is the mail a newsletter sends by itself. There are exactly two, they always exist, and each is
named by its trigger:

- `subscriber_confirmed`: mailed after a person confirms from their own link (never to someone an admin
  added). It holds any number of emails, each with `--delay-days` (days after confirming) and `--send-hour`
  (0 to 23).
- `subscriber_unsubscribed`: one email, mailed at once to a person who unsubscribes. It takes no delay, and
  a second email is refused (403).

`journey show TRIGGER` carries every email in full; `.data.emails[].id` is the `EMAIL_ID` the other
commands take. `journey email show TRIGGER EMAIL_ID` reads one.

```bash
mailpiece journey show subscriber_confirmed --jq '.data.emails[] | {id, subject, delay_days, active}'
mailpiece journey email show subscriber_confirmed EMAIL_ID
mailpiece journey email create subscriber_confirmed --subject Welcome --body "Glad you are here." --delay-days 0
mailpiece journey email test subscriber_confirmed EMAIL_ID            # to your own address
mailpiece journey email test subscriber_confirmed EMAIL_ID --to ana@example.com
mailpiece journey email activate subscriber_confirmed EMAIL_ID
```

**MUST:**

1. **An email is a draft until activated.** `create` and `update` mail nobody.
2. **Ask before `journey email activate`.** From then on the email goes by itself to real people as they
   reach it, and mail that has gone out cannot be taken back. `deactivate` only stops what has not gone yet.
3. **`journey email test` sends real mail**, from the newsletter's sender. It is refused (422) while the
   newsletter has no from name and from address, or the email has no subject or body, and after 10 tests in
   3 minutes (429).
4. **An active email cannot lose its subject or body** (422). Updating an active email changes it for
   everyone it has yet to reach.
5. **`journey email delete` is for good**, active or not.

### Settings

Each part of the settings is a resource of its own, with `show` and, where it can change, `update`. An
update changes only the flags passed.

| Part | Holds | Update flags |
|------|-------|--------------|
| `settings newsletter` | name, description, locale (`en` or `es`) | `--name`, `--description`, `--locale` |
| `settings handle` | handle: the word the newsletter answers at beside its number | `--handle` (empty clears it) |
| `settings sending` | from name, from address, reply-to | `--from-name`, `--from-address`, `--reply-to-address` |
| `settings subscribe-form` | where the embedded form sends a person back to | `--return-url` (empty clears it) |
| `settings bounces` | the webhook address to give the mail server | (read only) |
| `settings join-code` | the code and link people join the team with, its use count and limit | `--usage-limit`; `reset` |
| `settings notifications` | whether you are emailed when someone confirms a subscription | `--new-subscriber=true\|false` |

- **Changing the handle breaks links already mailed.** `settings handle update --handle NAME` changes it and
  `--handle ""` clears it; the old name is free for another newsletter at once. Ask first. The number keeps
  working and is what the CLI saves, so `--account` need not change unless a handle was typed by hand.
- **The locale is the newsletter's language**: every public page and every email is written in it.
- **A newsletter mails nothing without a sender.** `settings sending show` with a null `from_name` or
  `from_address` explains a refused test or send.
- **`settings notifications` is your own**: it is the token's user's setting, on until switched off, and
  changes nobody else's.
- **`settings join-code reset` retires the old code**: every link carrying it stops working. `active` is
  false once `usage_count` has reached `usage_limit`.

### Your user

A user reaches only their own profile: any other id is a 404. Your user's id on each account is in
`mailpiece identity` (`.data.accounts[].user.id`). The only thing to change is your name. The avatar,
passkeys and a change of email address are done in the app.

```bash
mailpiece identity --jq '.data.accounts[] | {slug, user: .user.id}'
mailpiece user update USER_ID --name "Ana Ruiz"
```

---

## Any Record by URL, and the Raw API

```bash
mailpiece show URL                                 # any record by its page's URL in Mailpiece, as JSON
mailpiece api get PATH                             # e.g. posts/ID.json
mailpiece api post PATH --data JSON                # --data - reads the body from stdin
mailpiece api put PATH --data JSON
mailpiece api delete PATH
```

`show` takes the account from the URL, number or handle, and reads the page's `.json` path. `api`
paths are relative to the account (`--account`), or a URL on the configured API host with that
account. Both send the token to the configured API host only: `show` keeps only the URL's path, and
`api` refuses a URL on any other host. A read token can only GET: a write comes back 401 with the
read-token hint. `api` writes are writes like any other: ask first.

---

## Over MCP

`mailpiece mcp` serves the reads as tools, one per domain, on the signed-in account. `--writes` adds
the writes, paired with a write token. `--domains posts,subscribers` narrows the tools. The action
`describe` gives any action's parameters.

| Tool | Actions |
|------|---------|
| `mailpiece_posts` | `list_posts`, `list_published_posts`, `list_sent_posts`, `get_post`, `create_post`, `update_post`, `delete_post`, `publish_post`, `unpublish_post`, `schedule_post`, `unschedule_post`, `deliver_post`, `test_post`, `relist_post`, `unlist_post`, `list_post_links`, `update_post_link`, `list_post_revisions`, `get_post_revision` |
| `mailpiece_search` | `search` |
| `mailpiece_subscribers` | `list_subscribers`, `list_unconfirmed_subscribers`, `list_unsubscribed_subscribers`, `list_bounced_subscribers`, `list_deactivated_subscribers`, `get_subscriber`, `create_subscriber`, `deactivate_subscriber`, `activate_subscriber`, `delete_subscriber` |
| `mailpiece_snippets` | `list_snippets`, `get_snippet`, `create_snippet`, `update_snippet`, `delete_snippet` |
| `mailpiece_journeys` | `list_journeys`, `get_journey`, `get_journey_email`, `create_journey_email`, `update_journey_email`, `delete_journey_email`, `activate_journey_email`, `deactivate_journey_email`, `test_journey_email` |
| `mailpiece_settings` | `get_newsletter`, `get_handle`, `get_sending`, `get_subscribe_form`, `get_bounces`, `get_join_code`, `get_notifications`, and `update_` each but bounces, `reset_join_code` |
| `mailpiece_users` | `get_user`, `update_user` |
| `mailpiece_identity` | `get_identity` |
| `mailpiece_access_tokens` | `create_access_token`, `delete_access_token` (with `--writes` only) |

Over MCP, rich text is passed as HTML: Markdown is not converted. A paged action answers
`next_page` while there is more; pass it back as `page`.

```bash
claude mcp add mailpiece -- mailpiece mcp
```

## The Skill

```bash
mailpiece skill            # in a terminal: pick where to install; piped or --agent: prints this file
mailpiece skill install    # ~/.agents/skills/mailpiece, linked into ~/.claude/skills/mailpiece
mailpiece setup claude     # installs the skill for Claude Code
```

The picker offers Agents (shared), Claude Code (global or project), OpenCode (global or project),
Codex, or a custom path. After an upgrade, the installed skill is refreshed on the next run.

---

## Error Handling

**Exit codes:**

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Usage / invalid arguments |
| 2 | Not found |
| 3 | Authentication failure |
| 4 | Permission denied |
| 5 | Rate limited |
| 6 | Network error |
| 7 | API / server error, validation included |
| 8 | Ambiguous match |

**Authentication errors (exit 3):**
```bash
mailpiece doctor                       # full health check with hints
mailpiece auth status                  # which token and account are in use, and why
mailpiece auth list                    # which profiles are saved
mailpiece auth switch PROFILE          # switch to the right profile
... | mailpiece auth login --with-token --account SLUG
```

A 401 on a write can mean a read token: make a write token, or ask the person to.

**Not found (exit 2):** check the id. Out of reach is a 404 too: do not retry.

**Permission denied (exit 4):** the signed-in user's role does not allow it. Report it.

**Network errors (exit 6):**
```bash
mailpiece doctor                       # API URL and reachability
mailpiece config explain               # why this API URL won
```

**Doctor:**
```bash
mailpiece doctor [--verbose] [--markdown]
mailpiece doctor --profile NAME
mailpiece doctor --all-profiles        # every saved profile; not with --profile, --token, --api-url or --account
```

It checks the version, runtime (verbose), global and local config, saved profiles, filesystem, shell
completion, the installed skill, credentials and their storage, the API URL and reachability,
authentication and account access.

## Learn More

```bash
mailpiece commands --json      # every command and flag
mailpiece <command> --help
```
