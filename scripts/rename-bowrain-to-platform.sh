#!/usr/bin/env bash
# scripts/rename-bowrain-to-platform.sh — Rename bowrain/ → platform/
#
# Run AFTER reorg-monorepo.sh.  Renames the top-level bowrain/ directory
# to platform/ and the nested bowrain/platform/ subdirectory (the Go
# platform module) to platform/core/.
#
# What changes:
#   - Directory locations (git mv)
#   - go.work use directives
#   - go.mod replace directives (relative paths only)
#   - Makefile directory path references
#   - .goreleaser.yaml directory paths
#   - .dockerignore paths
#   - CI workflow file path references
#   - compose.yaml / compose.override.yaml internal paths
#   - deploy config paths
#
# What does NOT change:
#   - Go module names (github.com/gokapi/gokapi/bowrain stays the same)
#   - Go import paths in .go source files (module names unchanged)
#   - Docker image names (ghcr.io/gokapi/bowrain-*)
#   - GHA cache scope names (scope=bowrain-*)
#   - Environment variable names (BOWRAIN_OIDC_*, BOWRAIN_SMTP_*, etc.)
#   - Makefile target names (build-bowrain, test-bowrain, etc.)
#   - Makefile variable names (BOWRAIN_CLI_DIR, etc.)
#   - CI job names and output variable names
#
# Usage:
#   git checkout -b rename-bowrain
#   bash scripts/rename-bowrain-to-platform.sh
#   # Review changes, run tests, commit
#
set -euo pipefail

# Portable in-place sed: macOS BSD sed requires `-i ''`, GNU sed uses just `-i`
if sed --version 2>/dev/null | grep -q GNU; then
  _SED_I_ARGS=(-i)
else
  _SED_I_ARGS=(-i '')
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# ─── safety checks ──────────────────────────────────────────────────────────

if [[ -n "$(git status --porcelain)" ]]; then
  echo "ERROR: working tree is dirty — commit or stash first" >&2
  exit 1
fi

if [[ ! -d bowrain ]]; then
  echo "ERROR: bowrain/ directory not found — run reorg-monorepo.sh first" >&2
  exit 1
fi

if [[ ! -d bowrain/platform ]]; then
  echo "ERROR: bowrain/platform/ not found — expected post-reorg layout" >&2
  exit 1
fi

echo "==> Renaming bowrain/ → platform/ (and bowrain/platform/ → platform/core/)"

# ─── 1. Git mv: rename directories ──────────────────────────────────────────
#
# Can't do a direct git mv bowrain/platform → platform/core in one step
# because "platform" will collide with the parent rename.  Use a temp name.

echo "--- Step 1: Renaming directories"

echo "    bowrain/platform/ → bowrain/_core-tmp (temp)"
git mv bowrain/platform bowrain/_core-tmp

echo "    bowrain/ → platform/"
git mv bowrain platform

echo "    platform/_core-tmp → platform/core"
git mv platform/_core-tmp platform/core

# ─── 2. Update go.work ──────────────────────────────────────────────────────

echo "--- Step 2: Updating go.work"
cat > go.work <<'GOWORK'
go 1.26.0

use (
	./framework
	./framework/cli
	./framework/kapi
	./platform/core
	./platform/cli
	./platform
)
GOWORK

# ─── 3. Update go.mod replace directives ────────────────────────────────────
#
# Only relative paths change.  Module names stay the same.
#
# platform/go.mod       (was bowrain/go.mod):      ./platform → ./core
# platform/cli/go.mod   (was bowrain/cli/go.mod):  ../platform → ../core
# platform/core/go.mod  (was bowrain/platform/go.mod): ../../framework — unchanged

echo "--- Step 3: Updating go.mod replace directives"

sed "${_SED_I_ARGS[@]}" \
  's|gokapi/platform => \./platform|gokapi/platform => ./core|' \
  platform/go.mod

sed "${_SED_I_ARGS[@]}" \
  's|gokapi/platform => \.\./platform|gokapi/platform => ../core|' \
  platform/cli/go.mod

# ─── 4. Update Makefile ─────────────────────────────────────────────────────
#
# Replace directory paths only.  Target names, variable names, and env vars
# are left unchanged.

echo "--- Step 4: Updating Makefile directory paths"

# Specific first: bowrain/platform → platform/core
sed "${_SED_I_ARGS[@]}" 's|bowrain/platform|platform/core|g' Makefile

# General: bowrain/ → platform/  (covers all remaining path references)
sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' Makefile

# Bare 'cd bowrain &&' (bowrain module root, no trailing slash)
sed "${_SED_I_ARGS[@]}" 's|cd bowrain &&|cd platform &&|g' Makefile

# ─── 5. Update .goreleaser.yaml ─────────────────────────────────────────────

echo "--- Step 5: Updating .goreleaser.yaml"

sed "${_SED_I_ARGS[@]}" 's|bowrain/platform|platform/core|g' .goreleaser.yaml
sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' .goreleaser.yaml

# ─── 6. Update .dockerignore files ──────────────────────────────────────────

echo "--- Step 6: Updating .dockerignore files"

sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' .dockerignore

if [[ -f platform/.dockerignore ]]; then
  sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' platform/.dockerignore
fi

# ─── 7. Update CI workflows ─────────────────────────────────────────────────

echo "--- Step 7: Updating CI workflow files"

for wf in .github/workflows/*.yml; do
  # Specific first: bowrain/platform → platform/core
  sed "${_SED_I_ARGS[@]}" 's|bowrain/platform|platform/core|g' "$wf"

  # General: bowrain/ → platform/
  sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' "$wf"

  # Bare 'cd bowrain' at end of line (multiline run: blocks in YAML)
  sed "${_SED_I_ARGS[@]}" 's|cd bowrain$|cd platform|g' "$wf"

  # Grep regex patterns in CI filter scripts: '^bowrain/' → '^platform/'
  sed "${_SED_I_ARGS[@]}" "s|'\\^bowrain/|'^platform/|g" "$wf"
done

# ─── 8. Update compose files ────────────────────────────────────────────────

echo "--- Step 8: Updating compose files"

if [[ -f platform/compose.yaml ]]; then
  sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' platform/compose.yaml
fi

if [[ -f platform/compose.override.yaml ]]; then
  sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' platform/compose.override.yaml
fi

# ─── 9. Update deploy configs ───────────────────────────────────────────────

echo "--- Step 9: Updating deploy configs"

find platform/deploy/ -name '*.yaml' -o -name '*.yml' 2>/dev/null | while read -r f; do
  sed "${_SED_I_ARGS[@]}" 's|bowrain/|platform/|g' "$f"
done

# ─── Done ────────────────────────────────────────────────────────────────────

echo ""
echo "==> Rename complete. Next steps:"
echo ""
echo "  1. Review changes:    git diff --stat"
echo "  2. Verify Go build:   make build build-server build-bowrain"
echo "  3. Verify tests:      make test"
echo "  4. Commit:            git add -A && git commit -m 'rename: bowrain/ → platform/, bowrain/platform/ → platform/core/'"
echo ""
