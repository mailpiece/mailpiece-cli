# Releasing mailpiece-cli

## Quick Release

```bash
# Run preflight checks, then tag and push
make release VERSION=v0.2.0

# Or run the preflight checks without tagging
make release VERSION=v0.2.0 DRY_RUN=1
```

`scripts/release.sh` checks that you are on `main`, clean and in sync with `origin`, runs
`make release-check`, then tags and pushes. Pushing the tag triggers
`.github/workflows/release.yml`, which:

1. Runs the test and security workflows
2. Checks the tag is on `main`
3. Builds binaries for linux/darwin x amd64/arm64 and windows/amd64, with shell completions
4. Signs `checksums.txt` with a cosign keyless bundle (OIDC), published as `checksums.txt.bundle`
5. Attests build provenance for the archives in `checksums.txt`
6. For stable tags only, publishes the Homebrew cask to `mailpiece/homebrew-tap`

## Versioning

Follow [semver](https://semver.org/). Use a `v` prefix for tags: `v0.2.0`, `v0.2.0-beta1`, `v0.3.0-rc.1`.

| Surface | Stable tag `v0.2.0` | Prerelease tag `v0.2.0-beta1` |
|---------|---------------------|-------------------------------|
| GitHub Releases | A normal release, eligible to be the latest. | A GitHub prerelease, never marked latest. |
| Release assets | Archives, checksums and the cosign bundle. | The same, for testers to download by hand. |
| `install.sh` | Installs `v0.2.0` once GitHub marks it latest. | Never installs it: it reads `releases/latest`. |
| Homebrew | Updates `mailpiece/tap/mailpiece`. | Leaves the cask alone (`skip_upload: auto`). |

## CI Secrets

All release credentials live in the `release` environment (`Settings > Environments > release`), so
they are exposed only to the release job, behind the environment's required reviewers. There are no
repository-level release secrets. `HOMEBREW_TAP_TOKEN` is not a stored secret: it is minted per run
from a GitHub App's credentials, scoped to `mailpiece/homebrew-tap` with contents write. The app must
be installed on `mailpiece/homebrew-tap` with contents read and write.

| Name | Type | Purpose |
|------|------|---------|
| `RELEASE_CLIENT_ID` | variable | The release GitHub App's client ID |
| `RELEASE_APP_PRIVATE_KEY` | secret | The release GitHub App's private key, for the tap push |

```bash
gh variable set RELEASE_CLIENT_ID --env release -R mailpiece/mailpiece-cli
gh secret set RELEASE_APP_PRIVATE_KEY --env release -R mailpiece/mailpiece-cli < app-private-key.pem
```

Pipe the key file in, so its newlines are kept. The cosign signature needs no secret: it uses the
workflow's OIDC token (`id-token: write`).

## Distribution Channels

| Channel | Location | Updated by |
|---------|----------|------------|
| GitHub Releases | `mailpiece/mailpiece-cli/releases` | GoReleaser |
| Homebrew | `mailpiece/homebrew-tap` `Casks/mailpiece.rb` | GoReleaser (stable tags only) |
| Go install | `go install github.com/mailpiece/mailpiece-cli/cmd/mailpiece@latest` | Go module proxy |
| curl installer | `scripts/install.sh` | Reads the latest GitHub release |

The binaries are not signed for macOS or Windows. The cask removes Homebrew's quarantine flag after
install so macOS runs the binary; `install.sh` downloads with curl, which sets none.

## Dry Run

```bash
# Full preflight without tagging
make release VERSION=v0.2.0 DRY_RUN=1

# Validate the GoReleaser config
goreleaser check

# Local build of every archive, unsigned and unpublished (completions come from the before hooks)
goreleaser release --snapshot --clean --skip=sign
```
