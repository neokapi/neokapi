#!/usr/bin/env bash
# scripts/reorg-monorepo.sh — Reorganize the neokapi monorepo
#
# Creates two top-level directories: framework/ and bowrain/.
# Framework code (core, cli, kapi, examples, bench) moves under framework/.
# Bowrain-related code (bowrain-cli, platform, packages/ui, docker, deploy,
# compose, assets, e2e) consolidates under bowrain/.
#
# Target layout:
#
#   framework/              framework Go module root (go.mod moves here)
#     core/                 framework packages (moved from ./core/)
#     sievepen/             translation memory (promoted from core/sievepen/)
#     termbase/             terminology (promoted from core/termbase/)
#     providers/ai/         LLM providers (promoted from core/ai/provider/)
#     providers/mt/         MT providers (promoted from core/mt/provider/)
#     cli/                  shared CLI base (moved from ./cli/)
#     kapi/                 kapi CLI tool (moved from ./kapi/, kapi-web removed)
#     examples/             plugin examples (moved from ./examples/)
#     bench/                benchmarks (moved from ./bench/)
#   bowrain/                bowrain server, apps, desktop (unchanged)
#     cli/                  bowrain CLI (moved from ./bowrain-cli/)
#     platform/             platform types & interfaces (moved from ./platform/)
#     packages/ui/          shared React component library (moved from ./packages/ui/)
#     docker/               Dockerfiles (moved from ./docker/)
#     deploy/               deployment configs (moved from ./deploy/)
#     e2e/                  e2e test infra (files merged from ./e2e/ into existing bowrain/e2e/)
#     assets/               bowrain logo (moved from ./assets/bowrain-logo.png)
#     compose.yaml          dev compose (moved from root)
#     compose.override.yaml dev compose overlay (moved from root)
#     package.json          npm workspaces (moved + rewritten)
#     package-lock.json     lockfile (moved from root)
#     docs/                 bowrain docs (already in place)
#   website/                main docusaurus site (assets/ moved here)
#     assets/               logos and images (moved from ./assets/)
#
# What changes:
#   - Directory locations (git mv)
#   - go.work use directives
#   - replace directives in every go.mod
#   - Go import paths for promoted packages (sievepen, termbase, ai/provider, mt/provider)
#   - Makefile path variables
#   - .goreleaser.yaml build dirs and hooks
#   - .dockerignore paths
#   - CI workflow path triggers
#   - Root package.json → bowrain/package.json (rewritten workspaces)
#   - deploy/docker/compose.yaml Dockerfile paths
#
# What does NOT change:
#   - Go module names (github.com/neokapi/neokapi/cli stays the same)
#   - Most Go import paths (only promoted packages change)
#   - Website configuration (bowrain docs plugin already configured)
#
# Usage:
#   git checkout -b reorg
#   bash scripts/reorg-monorepo.sh
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

if ! command -v git &>/dev/null; then
  echo "ERROR: git is required" >&2
  exit 1
fi

echo "==> Starting monorepo reorganization in $REPO_ROOT"

# ─── 1. Create framework/ and move core, cli, kapi, examples, bench ─────────

echo "--- Creating framework/ directory"
mkdir -p framework

echo "--- Moving go.mod + go.sum → framework/"
git mv go.mod framework/go.mod
git mv go.sum framework/go.sum

echo "--- Moving core/ → framework/core/"
git mv core framework/core

# ─── 1b. Promote packages out of core/ to framework/ top level ───────────────

echo "--- Promoting sievepen/ out of core/"
git mv framework/core/sievepen framework/sievepen

echo "--- Promoting termbase/ out of core/"
git mv framework/core/termbase framework/termbase

echo "--- Promoting mt/provider/ → providers/mt/"
mkdir -p framework/providers
git mv framework/core/mt/provider framework/providers/mt

echo "--- Promoting ai/provider/ → providers/ai/"
git mv framework/core/ai/provider framework/providers/ai

echo "--- Moving cli/ → framework/cli/"
git mv cli framework/cli

echo "--- Moving kapi/ → framework/kapi/"
git mv kapi framework/kapi

if [[ -d examples ]]; then
  echo "--- Moving examples/ → framework/examples/"
  git mv examples framework/examples
fi

if [[ -d bench ]]; then
  echo "--- Moving bench/ → framework/bench/"
  git mv bench framework/bench
fi

# ─── 2. Move bowrain-cli/ → bowrain/cli/ ────────────────────────────────────

echo "--- Moving bowrain-cli/ → bowrain/cli/"
git mv bowrain-cli bowrain/cli

# ─── 3. Move platform/ → bowrain/platform/ ──────────────────────────────────

echo "--- Moving platform/ → bowrain/platform/"
git mv platform bowrain/platform

# ─── 4. Move packages/ui/ → bowrain/packages/ui/ ────────────────────────────

echo "--- Moving packages/ui/ → bowrain/packages/ui/"
mkdir -p bowrain/packages
git mv packages/ui bowrain/packages/ui
# Remove packages/ if now empty
if [[ -d packages ]] && [[ -z "$(ls -A packages)" ]]; then
  rmdir packages
fi

# ─── 5. Move docker/ → bowrain/docker/ ──────────────────────────────────────

echo "--- Moving docker/ → bowrain/docker/"
git mv docker bowrain/docker

# ─── 6. Move deploy/ → bowrain/deploy/ ──────────────────────────────────────

echo "--- Moving deploy/ → bowrain/deploy/"
git mv deploy bowrain/deploy

# ─── 7. Move e2e/ files → bowrain/e2e/ ──────────────────────────────────────

echo "--- Moving e2e/ files → bowrain/e2e/ (merging with existing bowrain/e2e/)"
# bowrain/e2e/ already exists (has server/ and tapes/ subdirs), so move files individually
for f in e2e/*; do
  git mv "$f" "bowrain/e2e/$(basename "$f")"
done
# Remove e2e/ if now empty
if [[ -d e2e ]] && [[ -z "$(ls -A e2e)" ]]; then
  rmdir e2e
fi

# ─── 8. Move compose files → bowrain/ ───────────────────────────────────────

echo "--- Moving compose files → bowrain/"
git mv compose.yaml bowrain/compose.yaml
git mv compose.override.yaml bowrain/compose.override.yaml

# ─── 9. Move assets/ → website/assets/ ────────────────────────────────────────

echo "--- Moving bowrain-logo.png → bowrain/assets/"
mkdir -p bowrain/assets
git mv assets/bowrain-logo.png bowrain/assets/bowrain-logo.png

echo "--- Moving remaining assets/ → website/assets/"
git mv assets website/assets

# ─── 10. Remove kapi/apps/kapi-web/ ─────────────────────────────────────────

echo "--- Removing framework/kapi/apps/kapi-web/ (decouples kapi from bowrain UI)"
git rm -rf framework/kapi/apps/kapi-web

# ─── 11. Move root package.json + lockfile → bowrain/ ───────────────────────

echo "--- Moving package.json and package-lock.json → bowrain/"
git mv package.json bowrain/package.json
git mv package-lock.json bowrain/package-lock.json

# ─── 12. Rewrite bowrain/package.json workspaces ────────────────────────────

echo "--- Rewriting bowrain/package.json workspaces"
cat > bowrain/package.json <<'PKGJSON'
{
  "private": true,
  "workspaces": [
    "packages/ui",
    "apps/web",
    "apps/bowrain/frontend"
  ],
  "devDependencies": {
    "storybook": "^10.2.13"
  }
}
PKGJSON

# ─── 13. Update go.work ─────────────────────────────────────────────────────

echo "--- Updating go.work"
cat > go.work <<'GOWORK'
go 1.26.0

use (
	./framework
	./framework/cli
	./framework/kapi
	./bowrain/platform
	./bowrain/cli
	./bowrain
)
GOWORK

# ─── 14. Rewrite Go import paths for promoted packages ──────────────────────

echo "--- Rewriting Go import paths for promoted packages"

# sievepen:    core/sievepen    → sievepen       (drops core/ prefix)
# termbase:    core/termbase    → termbase       (drops core/ prefix)
# ai/provider: core/ai/provider → providers/ai   (reorganized)
# mt/provider: core/mt/provider → providers/mt   (reorganized)
#
# These find/replace operate on all .go files across every module.
# The module name (github.com/neokapi/neokapi) stays the same — only the
# package path suffix changes.  We match the partial path (without leading
# quote) to also catch aliased imports like:  fw "github.com/…/core/sievepen"

find framework/ bowrain/ -name '*.go' -exec sed "${_SED_I_ARGS[@]}" \
  -e 's|neokapi/neokapi/core/sievepen"|neokapi/neokapi/sievepen"|g' \
  -e 's|neokapi/neokapi/core/termbase"|neokapi/neokapi/termbase"|g' \
  -e 's|neokapi/neokapi/core/ai/provider"|neokapi/neokapi/providers/ai"|g' \
  -e 's|neokapi/neokapi/core/mt/provider"|neokapi/neokapi/providers/mt"|g' \
  {} +

# ─── 15. Update replace directives in go.mod files ──────────────────────────

echo "--- Updating go.mod replace directives"

# framework/cli/go.mod: was cli/go.mod
# cli/ was at repo-root/cli/, go.mod root was repo-root/ (=> ../)
# Now: framework/cli/, go.mod root is framework/ (=> ../)
# No change needed — relative path is identical.

# framework/kapi/go.mod: was kapi/go.mod
# root was => ../ → framework/ is still => ../
# cli was => ../cli → framework/cli is still => ../cli
# No change needed — relative paths are identical.

# bowrain/cli/go.mod: was bowrain-cli/go.mod (moved 1 level deeper)
# neokapi root: => ../ → => ../../framework
# cli: => ../cli → => ../../framework/cli
# platform: => ../platform → => ../platform (stays — sibling under bowrain/)
sed "${_SED_I_ARGS[@]}" 's|neokapi/neokapi => \.\./|neokapi/neokapi => ../../framework|' bowrain/cli/go.mod
sed "${_SED_I_ARGS[@]}" 's|neokapi/cli => \.\./cli|neokapi/cli => ../../framework/cli|' bowrain/cli/go.mod

# bowrain/platform/go.mod: was platform/go.mod (moved 1 level deeper)
# neokapi root: => ../ → => ../../framework
sed "${_SED_I_ARGS[@]}" 's|neokapi/neokapi => \.\./|neokapi/neokapi => ../../framework|' bowrain/platform/go.mod

# bowrain/go.mod: stays at bowrain/ (not moved)
# neokapi root: => ../ → => ../framework
# platform: => ../platform → => ./platform (now a sibling under bowrain/)
# Important: process platform first (more specific) before the root pattern
sed "${_SED_I_ARGS[@]}" 's|neokapi/platform => \.\./platform|neokapi/platform => ./platform|' bowrain/go.mod
sed "${_SED_I_ARGS[@]}" 's|neokapi/neokapi => \.\./|neokapi/neokapi => ../framework|' bowrain/go.mod

# ─── 16. Update Makefile ────────────────────────────────────────────────────

echo "--- Updating Makefile path references"

# Module directory variables
sed "${_SED_I_ARGS[@]}" 's|^CLI_DIR  *:=.*|CLI_DIR         := framework/cli|' Makefile
sed "${_SED_I_ARGS[@]}" 's|^PLATFORM_DIR  *:=.*|PLATFORM_DIR    := bowrain/platform|' Makefile
sed "${_SED_I_ARGS[@]}" 's|^BOWRAIN_CLI_DIR  *:=.*|BOWRAIN_CLI_DIR := bowrain/cli|' Makefile
sed "${_SED_I_ARGS[@]}" 's|^KAPI_DIR  *:=.*|KAPI_DIR        := framework/kapi|' Makefile

# Path variables
sed "${_SED_I_ARGS[@]}" 's|CERT_DIR  *:= docker/traefik/certs|CERT_DIR     := bowrain/docker/traefik/certs|' Makefile
sed "${_SED_I_ARGS[@]}" 's|KAPI_WEB_DIR  *:= kapi/apps/kapi-web|# KAPI_WEB_DIR removed (kapi-web decoupled)|' Makefile

# cd commands for module operations
sed "${_SED_I_ARGS[@]}" 's|cd cli \&\&|cd framework/cli \&\&|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|cd platform \&\&|cd bowrain/platform \&\&|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|cd bowrain-cli \&\&|cd bowrain/cli \&\&|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|cd kapi \&\&|cd framework/kapi \&\&|g' Makefile

# Docker compose references
sed "${_SED_I_ARGS[@]}" 's|-f compose\.yaml|-f bowrain/compose.yaml|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|-f compose\.override\.yaml|-f bowrain/compose.override.yaml|g' Makefile

# Docker build context paths
sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-server|bowrain/docker/bowrain-server|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-web|bowrain/docker/bowrain-web|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-worker|bowrain/docker/bowrain-worker|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|docker/keycloak|bowrain/docker/keycloak|g' Makefile
sed "${_SED_I_ARGS[@]}" 's|docker/traefik|bowrain/docker/traefik|g' Makefile

# Remove kapi-web references from Makefile (kapi-web was removed)
sed "${_SED_I_ARGS[@]}" '/KAPI_WEB_DIR/d' Makefile
sed "${_SED_I_ARGS[@]}" '/kapi-web-deps/d' Makefile
sed "${_SED_I_ARGS[@]}" '/kapi-web-build/d' Makefile
sed "${_SED_I_ARGS[@]}" '/kapi\/apps\/kapi-web/d' Makefile

# ─── 17. Update .goreleaser.yaml ────────────────────────────────────────────

echo "--- Updating .goreleaser.yaml"

# Before hooks: update cd paths for module tidy
sed "${_SED_I_ARGS[@]}" 's|cd cli \&\&|cd framework/cli \&\&|g' .goreleaser.yaml
sed "${_SED_I_ARGS[@]}" 's|cd platform \&\&|cd bowrain/platform \&\&|g' .goreleaser.yaml
sed "${_SED_I_ARGS[@]}" 's|cd bowrain-cli \&\&|cd bowrain/cli \&\&|g' .goreleaser.yaml
sed "${_SED_I_ARGS[@]}" 's|cd kapi \&\&|cd framework/kapi \&\&|g' .goreleaser.yaml

# Build dirs
sed "${_SED_I_ARGS[@]}" 's|dir: bowrain-cli|dir: bowrain/cli|g' .goreleaser.yaml
sed "${_SED_I_ARGS[@]}" 's|dir: kapi$|dir: framework/kapi|g' .goreleaser.yaml

# ─── 18. Update .dockerignore ───────────────────────────────────────────────

echo "--- Updating .dockerignore"

# The Dockerfiles now live under bowrain/docker/ and the build context
# is likely bowrain/. Adjust relative paths accordingly.
sed "${_SED_I_ARGS[@]}" 's|apps/web/dist/|bowrain/apps/web/dist/|g' .dockerignore
sed "${_SED_I_ARGS[@]}" 's|apps/bowrain/frontend/dist/|bowrain/apps/bowrain/frontend/dist/|g' .dockerignore
sed "${_SED_I_ARGS[@]}" 's|apps/bowrain/|bowrain/apps/bowrain/|g' .dockerignore

# ─── 19. Update CI workflows ────────────────────────────────────────────────

echo "--- Updating CI workflow path triggers"

update_workflow_paths() {
  local file="$1"
  # Update path triggers in workflow files
  sed "${_SED_I_ARGS[@]}" "s|'core/\*\*'|'framework/core/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'cli/\*\*'|'framework/cli/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'kapi/\*\*'|'framework/kapi/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'platform/\*\*'|'bowrain/platform/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'bowrain-cli/\*\*'|'bowrain/cli/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'packages/ui/\*\*'|'bowrain/packages/ui/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'docker/bowrain-server/\*\*'|'bowrain/docker/bowrain-server/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'docker/bowrain-web/\*\*'|'bowrain/docker/bowrain-web/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'docker/bowrain-worker/\*\*'|'bowrain/docker/bowrain-worker/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'docker/keycloak/\*\*'|'bowrain/docker/keycloak/**'|g" "$file"
  sed "${_SED_I_ARGS[@]}" "s|'kapi/apps/kapi-web/\*\*'||g" "$file"
  # Also handle double-quoted variants
  sed "${_SED_I_ARGS[@]}" 's|"core/\*\*"|"framework/core/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"cli/\*\*"|"framework/cli/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"kapi/\*\*"|"framework/kapi/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"platform/\*\*"|"bowrain/platform/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"bowrain-cli/\*\*"|"bowrain/cli/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"packages/ui/\*\*"|"bowrain/packages/ui/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"docker/bowrain-server/\*\*"|"bowrain/docker/bowrain-server/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"docker/bowrain-web/\*\*"|"bowrain/docker/bowrain-web/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"docker/bowrain-worker/\*\*"|"bowrain/docker/bowrain-worker/**"|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|"docker/keycloak/\*\*"|"bowrain/docker/keycloak/**"|g' "$file"
  # Remove kapi-web path entries entirely
  sed "${_SED_I_ARGS[@]}" "/kapi\/apps\/kapi-web/d" "$file"
  # cd commands in workflow steps
  sed "${_SED_I_ARGS[@]}" 's|cd cli|cd framework/cli|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|cd platform|cd bowrain/platform|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|cd bowrain-cli|cd bowrain/cli|g' "$file"
  sed "${_SED_I_ARGS[@]}" 's|cd kapi|cd framework/kapi|g' "$file"
  # e2e compose path (root e2e/ moved into bowrain/e2e/)
  sed "${_SED_I_ARGS[@]}" 's|-f e2e/compose\.yaml|-f bowrain/e2e/compose.yaml|g' "$file"
}

for wf in .github/workflows/*.yml; do
  update_workflow_paths "$wf"
done

# ─── 20. Update deploy/docker/compose.yaml Dockerfile paths ─────────────────

echo "--- Updating bowrain/deploy/docker/compose.yaml"
if [[ -f bowrain/deploy/docker/compose.yaml ]]; then
  sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-server|bowrain/docker/bowrain-server|g' bowrain/deploy/docker/compose.yaml
  sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-web|bowrain/docker/bowrain-web|g' bowrain/deploy/docker/compose.yaml
  sed "${_SED_I_ARGS[@]}" 's|docker/bowrain-worker|bowrain/docker/bowrain-worker|g' bowrain/deploy/docker/compose.yaml
  sed "${_SED_I_ARGS[@]}" 's|docker/keycloak|bowrain/docker/keycloak|g' bowrain/deploy/docker/compose.yaml
fi

# ─── 21. Move .dockerignore into bowrain/ (build context is bowrain/) ────────

echo "--- Copying .dockerignore into bowrain/"
cp .dockerignore bowrain/.dockerignore

# ─── Done ────────────────────────────────────────────────────────────────────

echo ""
echo "==> Reorganization complete. Next steps:"
echo ""
echo "  1. Review changes:  git diff --stat"
echo "  2. Verify Go build: cd framework && go build ./..."
echo "  3. Verify tests:    make test"
echo "  4. Verify frontend: cd bowrain && npm install && npm run build"
echo "  5. Commit:          git add -A && git commit -m 'reorg: framework/ and bowrain/ top-level layout'"
echo ""
