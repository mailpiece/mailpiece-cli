# Security Policy

## Reporting a Vulnerability

Please report security vulnerabilities privately through GitHub: on
[mailpiece/mailpiece-cli](https://github.com/mailpiece/mailpiece-cli), open the **Security** tab
and choose **Report a vulnerability**.

Do **NOT** open public GitHub issues for security vulnerabilities.

We will acknowledge receipt within 48 hours and aim to provide a fix within 90 days depending on severity.

## Credential Storage

The Mailpiece CLI stores API tokens using your operating system's native credential storage:

| Platform | Storage |
|----------|---------|
| macOS | Keychain |
| Windows | Credential Manager |
| Linux | Secret Service (GNOME Keyring, KWallet) |

### File-based Fallback

If the system keyring is unavailable (headless servers, containers), set:

```bash
export MAILPIECE_NO_KEYRING=1
```

Credentials will then be stored in plain text in `~/.config/mailpiece/credentials/credentials.json`,
created with `0600` permissions.

## Verifying a Release

Each release's `checksums.txt` is signed with cosign keyless, and the signature is published beside
it as `checksums.txt.bundle`:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.bundle \
  --certificate-identity-regexp '^https://github.com/mailpiece/mailpiece-cli/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum --ignore-missing -c checksums.txt
```

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest  | Yes       |
| < Latest | No       |

We only provide security fixes for the latest release. Users should upgrade promptly.
