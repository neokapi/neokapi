#!/bin/bash
# Pre-commit/push verification hook for Claude Code.
# Blocks git commit and git push if builds/tests fail.
#
# Change-aware: only checks modules affected by staged changes.
# Mirrors the path→module mapping from CI (.github/workflows/ci.yml)
# and pre-push-check.sh for consistency.
set -euo pipefail

INPUT=$(cat)
COMMAND=$(echo "$INPUT" | jq -r '.tool_input.command // empty')

# Only intercept git commit and git push
case "$COMMAND" in
  git\ commit*|git\ push*|*"&&"*git\ commit*|*"&&"*git\ push*)
    ;;
  *)
    exit 0
    ;;
esac

cd "$CLAUDE_PROJECT_DIR" || exit 1

# ── Detect affected modules from staged files ──────────────────────────────

STAGED=$(git diff --cached --name-only 2>/dev/null || true)
if [ -z "$STAGED" ]; then
  # For git push, check what's ahead of remote
  STAGED=$(git diff --name-only "origin/main"...HEAD 2>/dev/null || true)
fi
if [ -z "$STAGED" ]; then
  exit 0
fi

# Module flags
MOD_FRAMEWORK=false
MOD_CLI=false
MOD_KAPI=false
MOD_KAPI_DESKTOP=false
MOD_PLATFORM=false      # bowrain/core
MOD_BOWRAIN_CLI=false
MOD_BOWRAIN=false
MOD_FRONTEND=false

# Map paths to modules (matches CI change detection)
while IFS= read -r f; do
  case "$f" in
    core/*|go.mod|go.sum|go.work|sievepen/*|termbase/*|providers/*)
      MOD_FRAMEWORK=true ;;
    cli/*)
      MOD_CLI=true ;;
    kapi/*)
      MOD_KAPI=true ;;
    apps/kapi-desktop/*)
      MOD_KAPI_DESKTOP=true ;;
    bowrain/core/*)
      MOD_PLATFORM=true ;;
    bowrain/cli/*)
      MOD_BOWRAIN_CLI=true ;;
    bowrain/apps/web/*|bowrain/apps/bowrain/frontend/*|bowrain/packages/*)
      MOD_FRONTEND=true ;;
    bowrain/*)
      MOD_BOWRAIN=true ;;
    packages/*)
      MOD_FRONTEND=true ;;
    .golangci.yml)
      # Lint config change affects all Go modules
      MOD_FRAMEWORK=true; MOD_CLI=true; MOD_KAPI=true
      MOD_PLATFORM=true; MOD_BOWRAIN_CLI=true; MOD_BOWRAIN=true ;;
  esac
done <<< "$STAGED"

# ── Propagate dependencies ──────────────────────────────────────────────────
# framework → cli, kapi, kapi-desktop, platform, bowrain-cli, bowrain
# cli → kapi, bowrain-cli
# platform (bowrain/core) → bowrain-cli, bowrain

if [ "$MOD_FRAMEWORK" = true ]; then
  MOD_CLI=true; MOD_KAPI=true; MOD_KAPI_DESKTOP=true
  MOD_PLATFORM=true; MOD_BOWRAIN_CLI=true; MOD_BOWRAIN=true
fi
if [ "$MOD_CLI" = true ]; then
  MOD_KAPI=true; MOD_BOWRAIN_CLI=true
fi
if [ "$MOD_PLATFORM" = true ]; then
  MOD_BOWRAIN_CLI=true; MOD_BOWRAIN=true
fi

# ── Run checks ──────────────────────────────────────────────────────────────

echo "=== Pre-git verification ===" >&2

ERRORS=0

run_check() {
  local label="$1"; shift
  echo "  Checking: $label..." >&2
  if ! "$@" 2>&1; then
    echo "  FAILED: $label" >&2
    ERRORS=$((ERRORS + 1))
  fi
}

# Go builds and tests for affected modules
if [ "$MOD_FRAMEWORK" = true ]; then
  run_check "Framework build" go build ./...
  run_check "Framework tests" go test -shuffle=on ./... -count=1 -short
fi

if [ "$MOD_CLI" = true ]; then
  run_check "CLI build" bash -c "cd cli && go build ./..."
  run_check "CLI tests" bash -c "cd cli && go test -shuffle=on ./... -count=1 -short"
fi

if [ "$MOD_KAPI" = true ]; then
  run_check "Kapi build" bash -c "cd kapi && go build ./..."
  run_check "Kapi tests" bash -c "cd kapi && go test -shuffle=on ./... -count=1 -short"
fi

if [ "$MOD_KAPI_DESKTOP" = true ]; then
  # Ensure frontend embed placeholder exists for build
  mkdir -p apps/kapi-desktop/frontend/dist
  [ -f apps/kapi-desktop/frontend/dist/index.html ] || echo placeholder > apps/kapi-desktop/frontend/dist/index.html
  run_check "Kapi Desktop build" bash -c "cd apps/kapi-desktop && go build ./..."
  run_check "Kapi Desktop tests" bash -c "cd apps/kapi-desktop && go test -shuffle=on ./backend/... -count=1 -short"
fi

if [ "$MOD_PLATFORM" = true ]; then
  run_check "Platform build" bash -c "cd bowrain/core && go build ./..."
  run_check "Platform tests" bash -c "cd bowrain/core && go test -shuffle=on ./... -count=1 -short"
fi

if [ "$MOD_BOWRAIN_CLI" = true ]; then
  run_check "Bowrain CLI build" bash -c "cd bowrain/cli && go build ./..."
  run_check "Bowrain CLI tests" bash -c "cd bowrain/cli && go test -shuffle=on ./... -count=1 -short"
fi

if [ "$MOD_BOWRAIN" = true ]; then
  # Ensure web embed placeholder exists for build
  mkdir -p bowrain/apps/web/dist
  [ -f bowrain/apps/web/dist/index.html ] || echo placeholder > bowrain/apps/web/dist/index.html
  run_check "Bowrain build" bash -c "cd bowrain && go build ./cmd/bowrain-server/"
  run_check "Bowrain tests" bash -c "cd bowrain && go test -shuffle=on \$(go list ./... | grep -v '/apps/bowrain') -count=1 -short"
fi

# Check for accidental binaries in staged files
if echo "$STAGED" | grep -qE '\.class$|\.jar$|\.exe$|\.o$|\.so$|\.dylib$'; then
  echo "BLOCKED: Binary files detected in staged changes" >&2
  echo "$STAGED" | grep -E '\.class$|\.jar$|\.exe$|\.o$|\.so$|\.dylib$' >&2
  ERRORS=$((ERRORS + 1))
fi

# Frontend checks if frontend files changed
if [ "$MOD_FRONTEND" = true ]; then
  echo "  Checking: Frontend workspace..." >&2
  if ! (cd packages/ui && vp install --frozen-lockfile && vpx tsc) 2>&1; then
    echo "  FAILED: packages/ui typecheck" >&2
    ERRORS=$((ERRORS + 1))
  fi
fi

if [ "$ERRORS" -gt 0 ]; then
  echo "=== BLOCKED: $ERRORS check(s) failed ===" >&2
  exit 2
fi

echo "=== All checks passed ===" >&2
exit 0
