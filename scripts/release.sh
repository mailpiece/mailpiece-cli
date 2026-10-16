#!/usr/bin/env bash
# Usage: scripts/release.sh VERSION   (or VERSION=v0.2.0 make release; DRY_RUN=1 to stop before tagging)
#
# Checks main, runs make release-check, then tags and pushes vX.Y.Z, which starts
# .github/workflows/release.yml.

set -euo pipefail

VERSION="${1:-${VERSION:-}}"
DRY_RUN="${DRY_RUN:-0}"

if [ -z "$VERSION" ]; then
  echo "Usage: scripts/release.sh VERSION"
  echo "       VERSION=v0.2.0 make release"
  exit 1
fi

TAG="v${VERSION#v}"

if ! echo "$TAG" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
  echo "ERROR: Invalid version '$VERSION' (expected vX.Y.Z or vX.Y.Z-suffix)"
  exit 1
fi

CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$CURRENT_BRANCH" != "main" ]; then
  echo "ERROR: Not on main (currently on $CURRENT_BRANCH)"
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "ERROR: Working tree is not clean"
  git status --short
  exit 1
fi

git fetch origin main --tags --quiet
LOCAL=$(git rev-parse HEAD)
REMOTE=$(git rev-parse origin/main)
if [ "$LOCAL" != "$REMOTE" ]; then
  echo "ERROR: Local main is not in sync with origin"
  echo "  Local:  $LOCAL"
  echo "  Remote: $REMOTE"
  exit 1
fi

if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
  echo "ERROR: Tag $TAG already exists"
  exit 1
fi

echo "Release preflight for $TAG"
echo "  Commit: $LOCAL"
echo ""
echo "Running release-check..."
make release-check

if [ "$DRY_RUN" = "1" ] || [ "$DRY_RUN" = "true" ]; then
  echo ""
  echo "DRY RUN complete. No tag created."
  exit 0
fi

echo ""
echo "Creating tag $TAG..."
git tag -a "$TAG" -m "Release $TAG"
git push origin "$TAG"

echo ""
echo "Released $TAG"
echo "GitHub Actions will build and publish it: https://github.com/mailpiece/mailpiece-cli/actions"
