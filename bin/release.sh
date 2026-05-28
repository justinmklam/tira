#!/bin/bash
set -e

BUMP="${1:-}"

usage() {
  echo "Usage: $0 <major|minor|patch>"
  echo ""
  echo "  Computes the next version, previews the changelog, creates a git tag,"
  echo "  and publishes a GitHub release. GoReleaser attaches binaries automatically"
  echo "  via GitHub Actions on tag push."
  exit 1
}

case "$BUMP" in
  major|minor|patch) ;;
  *) usage ;;
esac

# Ensure we're on main
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
  echo "Error: must be on main branch (currently on '$BRANCH')"
  exit 1
fi

REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner 2>/dev/null)
if [ -z "$REPO" ]; then
  echo "Error: could not determine GitHub repo. Is 'gh' authenticated?"
  exit 1
fi

git fetch --tags --quiet

# Get latest semver tag (default to v0.0.0 if none)
LATEST=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)
LATEST="${LATEST:-v0.0.0}"

# Parse and bump semver
VERSION="${LATEST#v}"
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
esac

NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"

echo ""
echo "Current : $LATEST"
echo "New     : $NEW_TAG"
echo ""

# Preview changelog from git log
echo "Changelog:"
git log "${LATEST}..HEAD" --oneline --no-merges | while IFS= read -r line; do
  echo "- $line"
done
echo ""

read -r -p "Create and publish release $NEW_TAG? [y/N] " CONFIRM
case "$CONFIRM" in
  y|Y) ;;
  *)
    echo "Aborted."
    exit 0
    ;;
esac

git tag "$NEW_TAG"
git push origin "$NEW_TAG"

# Build release notes from git log between tags
NOTES_BODY="## What's Changed"$'\n\n'
while IFS= read -r line; do
  NOTES_BODY+="- ${line}"$'\n'
done < <(git log "${LATEST}..HEAD" --oneline --no-merges)
NOTES_BODY+=$'\n'"**Full Changelog**: https://github.com/${REPO}/compare/${LATEST}...${NEW_TAG}"

gh release create "$NEW_TAG" --title "$NEW_TAG" --notes "$NOTES_BODY"

echo ""
echo "✓ Released $NEW_TAG."
echo "  GoReleaser is building binaries — watch progress at:"
echo "  https://github.com/${REPO}/actions"
