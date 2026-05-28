#!/bin/bash
set -e

BUMP="${1:-}"

usage() {
  echo "Usage: $0 <major|minor|patch>"
  echo ""
  echo "  Bumps the version, creates a git tag, and pushes it."
  echo "  GoReleaser runs automatically via GitHub Actions on tag push."
  exit 1
}

# Validate argument
case "$BUMP" in
  major|minor|patch) ;;
  *) usage ;;
esac

# Ensure we're on main and up to date
BRANCH=$(git rev-parse --abbrev-ref HEAD)
if [ "$BRANCH" != "main" ]; then
  echo "Error: must be on main branch (currently on '$BRANCH')"
  exit 1
fi

if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: uncommitted changes. Please commit or stash first."
  exit 1
fi

echo "Fetching latest tags..."
git fetch --tags --quiet

# Get latest tag (default to v0.0.0 if none)
LATEST=$(git tag --sort=-v:refname | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1)
LATEST="${LATEST:-v0.0.0}"

# Parse semver components
VERSION="${LATEST#v}"
MAJOR=$(echo "$VERSION" | cut -d. -f1)
MINOR=$(echo "$VERSION" | cut -d. -f2)
PATCH=$(echo "$VERSION" | cut -d. -f3)

# Bump the appropriate component
case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
esac

NEW_TAG="v${MAJOR}.${MINOR}.${PATCH}"

# Show commits that will be included (matches goreleaser's changelog filters)
echo ""
echo "Current tag : $LATEST"
echo "New tag     : $NEW_TAG"
echo ""
echo "Commits since $LATEST:"
echo ""
git log "${LATEST}..HEAD" \
  --oneline \
  --no-merges \
  --invert-grep \
  --grep="^docs:" \
  --grep="^test:" \
  --grep="^chore:" \
  2>/dev/null || git log "${LATEST}..HEAD" --oneline --no-merges
echo ""

# Confirm
read -r -p "Create and push tag $NEW_TAG? [y/N] " CONFIRM
case "$CONFIRM" in
  y|Y) ;;
  *)
    echo "Aborted."
    exit 0
    ;;
esac

git tag "$NEW_TAG"
git push origin "$NEW_TAG"

echo ""
echo "✓ Tagged $NEW_TAG and pushed."
echo "  GitHub Actions will run GoReleaser — watch progress at:"
echo "  https://github.com/justinmklam/tira/actions"
echo "  Release will appear at:"
echo "  https://github.com/justinmklam/tira/releases/tag/$NEW_TAG"
