#!/usr/bin/env bash
# scripts/reorg-monorepo.sh — Reorganize the gokapi monorepo
#
# Creates two top-level directories: framework/ and bowrain/.
# Framework code (core, cli, kapi, examples, bench) moves under framework/.
# Bowrain-related code (bowrain-cli, platform, packages/ui, docker, deploy,
# compose, assets) consolidates under bowrain/.
#
# Target layout:
#
#   framework/              framework Go module root (go.mod moves here)
#     core/                 framework packages (moved from ./core/)
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
#     assets/               bowrain logo (moved from ./assets/bowrain-logo.png)
#     compose.yaml          dev compose (moved from root)
#     compose.override.yaml dev compose overlay (moved from root)
#     package.json          npm workspaces (moved + rewritten)
#     package-lock.json     lockfile (moved from root)
#     docs/                 bowrain docs (already in place)
#   assets/                 gokapi framework logos (bowrain logo removed)
#   website/                main docusaurus site (unchanged)
#
# What changes:
#   - Directory locations (git mv)
#   - go.work use directives
#   - replace directives in every go.mod
#   - Makefile path variables
#   - .goreleaser.yaml build dirs and hooks
#   - .dockerignore paths
#   - CI workflow path triggers
#   - Root package.json → bowrain/package.json (rewritten workspaces)
#   - deploy/docker/compose.yaml Dockerfile paths
#
# What does NOT change:
#   - Go module names (github.com/gokapi/gokapi/cli stays the same)
#   - Go import paths in source code (no source rewriting needed)
#   - Website configuration (bowrain docs plugin already configured)
#
# Usage:
#   git checkout -b reorg
#   bash scripts/reorg-monorepo.sh
#   # Review changes, run tests, commit
#
set -euo pipefail

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

# ─── 7. Move compose files → bowrain/ ───────────────────────────────────────

echo "--- Moving compose files → bowrain/"
git mv compose.yaml bowrain/compose.yaml
git mv compose.override.yaml bowrain/compose.override.yaml

# ─── 8. Move bowrain asset → bowrain/assets/ ─────────────────────────────────

echo "--- Moving bowrain-logo.png → bowrain/assets/"
mkdir -p bowrain/assets
git mv assets/bowrain-logo.png bowrain/assets/bowrain-logo.png

# ─── 9. Remove kapi/apps/kapi-web/ ──────────────────────────────────────────

echo "--- Removing framework/kapi/apps/kapi-web/ (decouples kapi from bowrain UI)"
git rm -rf framework/kapi/apps/kapi-web

# ─── 10. Move root package.json + lockfile → bowrain/ ───────────────────────

echo "--- Moving package.json and package-lock.json → bowrain/"
git mv package.json bowrain/package.json
git mv package-lock.json bowrain/package-lock.json

# ─── 11. Rewrite bowrain/package.json workspaces ────────────────────────────

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

# ─── 12. Update go.work ─────────────────────────────────────────────────────

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

# ─── 13. Update replace directives in go.mod files ──────────────────────────

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
# gokapi root: => ../ → => ../../framework
# cli: => ../cli → => ../../framework/cli
# platform: => ../platform → => ../platform (stays — sibling under bowrain/)
sed -i 's|gokapi/gokapi => \.\./|gokapi/gokapi => ../../framework|' bowrain/cli/go.mod
sed -i 's|gokapi/cli => \.\./cli|gokapi/cli => ../../framework/cli|' bowrain/cli/go.mod

# bowrain/platform/go.mod: was platform/go.mod (moved 1 level deeper)
# gokapi root: => ../ → => ../../framework
sed -i 's|gokapi/gokapi => \.\./|gokapi/gokapi => ../../framework|' bowrain/platform/go.mod

# bowrain/go.mod: stays at bowrain/ (not moved)
# gokapi root: => ../ → => ../framework
# platform: => ../platform → => ./platform (now a sibling under bowrain/)
# Important: process platform first (more specific) before the root pattern
sed -i 's|gokapi/platform => \.\./platform|gokapi/platform => ./platform|' bowrain/go.mod
sed -i 's|gokapi/gokapi => \.\./|gokapi/gokapi => ../framework|' bowrain/go.mod

# ─── 14. Update Makefile ────────────────────────────────────────────────────

echo "--- Updating Makefile path references"

# Module directory variables
sed -i 's|^CLI_DIR  *:=.*|CLI_DIR         := framework/cli|' Makefile
sed -i 's|^PLATFORM_DIR  *:=.*|PLATFORM_DIR    := bowrain/platform|' Makefile
sed -i 's|^BOWRAIN_CLI_DIR  *:=.*|BOWRAIN_CLI_DIR := bowrain/cli|' Makefile
sed -i 's|^KAPI_DIR  *:=.*|KAPI_DIR        := framework/kapi|' Makefile

# Path variables
sed -i 's|CERT_DIR  *:= docker/traefik/certs|CERT_DIR     := bowrain/docker/traefik/certs|' Makefile
sed -i 's|KAPI_WEB_DIR  *:= kapi/apps/kapi-web|# KAPI_WEB_DIR removed (kapi-web decoupled)|' Makefile

# cd commands for module operations
sed -i 's|cd cli \&\&|cd framework/cli \&\&|g' Makefile
sed -i 's|cd platform \&\&|cd bowrain/platform \&\&|g' Makefile
sed -i 's|cd bowrain-cli \&\&|cd bowrain/cli \&\&|g' Makefile
sed -i 's|cd kapi \&\&|cd framework/kapi \&\&|g' Makefile

# Docker compose references
sed -i 's|-f compose\.yaml|-f bowrain/compose.yaml|g' Makefile
sed -i 's|-f compose\.override\.yaml|-f bowrain/compose.override.yaml|g' Makefile

# Docker build context paths
sed -i 's|docker/bowrain-server|bowrain/docker/bowrain-server|g' Makefile
sed -i 's|docker/bowrain-web|bowrain/docker/bowrain-web|g' Makefile
sed -i 's|docker/bowrain-worker|bowrain/docker/bowrain-worker|g' Makefile
sed -i 's|docker/keycloak|bowrain/docker/keycloak|g' Makefile
sed -i 's|docker/traefik|bowrain/docker/traefik|g' Makefile

# Remove kapi-web references from Makefile (kapi-web was removed)
sed -i '/KAPI_WEB_DIR/d' Makefile
sed -i '/kapi-web-deps/d' Makefile
sed -i '/kapi-web-build/d' Makefile
sed -i '/kapi\/apps\/kapi-web/d' Makefile

# ─── 15. Update .goreleaser.yaml ────────────────────────────────────────────

echo "--- Updating .goreleaser.yaml"

# Before hooks: update cd paths for module tidy
sed -i 's|cd cli \&\&|cd framework/cli \&\&|g' .goreleaser.yaml
sed -i 's|cd platform \&\&|cd bowrain/platform \&\&|g' .goreleaser.yaml
sed -i 's|cd bowrain-cli \&\&|cd bowrain/cli \&\&|g' .goreleaser.yaml
sed -i 's|cd kapi \&\&|cd framework/kapi \&\&|g' .goreleaser.yaml

# Build dirs
sed -i 's|dir: bowrain-cli|dir: bowrain/cli|g' .goreleaser.yaml
sed -i 's|dir: kapi$|dir: framework/kapi|g' .goreleaser.yaml

# ─── 16. Update .dockerignore ───────────────────────────────────────────────

echo "--- Updating .dockerignore"

# The Dockerfiles now live under bowrain/docker/ and the build context
# is likely bowrain/. Adjust relative paths accordingly.
sed -i 's|apps/web/dist/|bowrain/apps/web/dist/|g' .dockerignore
sed -i 's|apps/bowrain/frontend/dist/|bowrain/apps/bowrain/frontend/dist/|g' .dockerignore
sed -i 's|apps/bowrain/|bowrain/apps/bowrain/|g' .dockerignore

# ─── 17. Update CI workflows ────────────────────────────────────────────────

echo "--- Updating CI workflow path triggers"

update_workflow_paths() {
  local file="$1"
  # Update path triggers in workflow files
  sed -i "s|'core/\*\*'|'framework/core/**'|g" "$file"
  sed -i "s|'cli/\*\*'|'framework/cli/**'|g" "$file"
  sed -i "s|'kapi/\*\*'|'framework/kapi/**'|g" "$file"
  sed -i "s|'platform/\*\*'|'bowrain/platform/**'|g" "$file"
  sed -i "s|'bowrain-cli/\*\*'|'bowrain/cli/**'|g" "$file"
  sed -i "s|'packages/ui/\*\*'|'bowrain/packages/ui/**'|g" "$file"
  sed -i "s|'docker/bowrain-server/\*\*'|'bowrain/docker/bowrain-server/**'|g" "$file"
  sed -i "s|'docker/bowrain-web/\*\*'|'bowrain/docker/bowrain-web/**'|g" "$file"
  sed -i "s|'docker/bowrain-worker/\*\*'|'bowrain/docker/bowrain-worker/**'|g" "$file"
  sed -i "s|'docker/keycloak/\*\*'|'bowrain/docker/keycloak/**'|g" "$file"
  sed -i "s|'kapi/apps/kapi-web/\*\*'||g" "$file"
  # Also handle double-quoted variants
  sed -i 's|"core/\*\*"|"framework/core/**"|g' "$file"
  sed -i 's|"cli/\*\*"|"framework/cli/**"|g' "$file"
  sed -i 's|"kapi/\*\*"|"framework/kapi/**"|g' "$file"
  sed -i 's|"platform/\*\*"|"bowrain/platform/**"|g' "$file"
  sed -i 's|"bowrain-cli/\*\*"|"bowrain/cli/**"|g' "$file"
  sed -i 's|"packages/ui/\*\*"|"bowrain/packages/ui/**"|g' "$file"
  sed -i 's|"docker/bowrain-server/\*\*"|"bowrain/docker/bowrain-server/**"|g' "$file"
  sed -i 's|"docker/bowrain-web/\*\*"|"bowrain/docker/bowrain-web/**"|g' "$file"
  sed -i 's|"docker/bowrain-worker/\*\*"|"bowrain/docker/bowrain-worker/**"|g' "$file"
  sed -i 's|"docker/keycloak/\*\*"|"bowrain/docker/keycloak/**"|g' "$file"
  # Remove kapi-web path entries entirely
  sed -i "/kapi\/apps\/kapi-web/d" "$file"
  # cd commands in workflow steps
  sed -i 's|cd cli|cd framework/cli|g' "$file"
  sed -i 's|cd platform|cd bowrain/platform|g' "$file"
  sed -i 's|cd bowrain-cli|cd bowrain/cli|g' "$file"
  sed -i 's|cd kapi|cd framework/kapi|g' "$file"
}

for wf in .github/workflows/*.yml; do
  update_workflow_paths "$wf"
done

# ─── 18. Update deploy/docker/compose.yaml Dockerfile paths ─────────────────

echo "--- Updating bowrain/deploy/docker/compose.yaml"
if [[ -f bowrain/deploy/docker/compose.yaml ]]; then
  sed -i 's|docker/bowrain-server|bowrain/docker/bowrain-server|g' bowrain/deploy/docker/compose.yaml
  sed -i 's|docker/bowrain-web|bowrain/docker/bowrain-web|g' bowrain/deploy/docker/compose.yaml
  sed -i 's|docker/bowrain-worker|bowrain/docker/bowrain-worker|g' bowrain/deploy/docker/compose.yaml
  sed -i 's|docker/keycloak|bowrain/docker/keycloak|g' bowrain/deploy/docker/compose.yaml
fi

# ─── 19. Move .dockerignore into bowrain/ (build context is bowrain/) ────────

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
