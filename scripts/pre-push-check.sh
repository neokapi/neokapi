#!/bin/bash
# pre-push-check.sh — Run only the checks relevant to your changes.
#
# Mirrors the CI change-detection logic from .github/workflows/ci.yml
# so you catch issues before pushing.
#
# Usage:
#   ./scripts/pre-push-check.sh          # check uncommitted + unpushed vs origin/main
#   ./scripts/pre-push-check.sh --all    # run all checks regardless of changes

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
RED='\033[0;31m'
BOLD='\033[1m'
NC='\033[0m'

info()  { echo -e "${BOLD}${GREEN}▸${NC} $1"; }
warn()  { echo -e "${BOLD}${YELLOW}▸${NC} $1"; }
fail()  { echo -e "${BOLD}${RED}✗${NC} $1"; }
pass()  { echo -e "${BOLD}${GREEN}✓${NC} $1"; }

ERRORS=0

run_check() {
    local label="$1"; shift
    info "Running: $label"
    if "$@"; then
        pass "$label"
    else
        fail "$label"
        ERRORS=$((ERRORS + 1))
    fi
}

# Determine changed files
if [ "${1:-}" = "--all" ]; then
    RUN_ALL=true
    info "Running all checks (--all)"
else
    RUN_ALL=false
    # Compare against what's on remote main
    BASE=$(git merge-base HEAD origin/main 2>/dev/null || echo "HEAD~1")
    CHANGED=$(git diff --name-only "$BASE"...HEAD 2>/dev/null; git diff --name-only 2>/dev/null)
    CHANGED=$(echo "$CHANGED" | sort -u)

    if [ -z "$CHANGED" ]; then
        pass "No changes detected, nothing to check."
        exit 0
    fi
fi

matches() {
    [ "$RUN_ALL" = true ] && return 0
    for pattern in "$@"; do
        echo "$CHANGED" | grep -qE "$pattern" && return 0
    done
    return 1
}

echo ""

# ── Go checks ──────────────────────────────────────────────────────────────

if matches '^core/' '^go\.(mod|sum)$' '^cli/' '^kapi/' '^go\.work'; then
    run_check "Go lint (framework)" make check
fi

if matches '^bowrain/core/' '^bowrain/cli/' '^bowrain/go\.(mod|sum)$'; then
    run_check "Go lint (bowrain)" make -C bowrain check
fi

# ── Frontend checks ────────────────────────────────────────────────────────

if matches '^bowrain/packages/ui/' '^bowrain/apps/web/' '^bowrain/apps/bowrain/frontend/'; then
    run_check "Frontend (bowrain)" make frontend-check-all
fi

# ── Kapi Desktop frontend ─────────────────────────────────────────────────

if matches '^apps/kapi-desktop/' '^packages/(ui|flow-editor)/'; then
    run_check "Kapi Desktop frontend" make kapi-desktop-frontend-check

    if matches '^packages/flow-editor/'; then
        run_check "Flow editor" make flow-editor-check
    fi
fi

# ── Summary ────────────────────────────────────────────────────────────────

echo ""
if [ "$ERRORS" -gt 0 ]; then
    fail "$ERRORS check(s) failed. Fix before pushing."
    exit 1
else
    pass "All relevant checks passed."
fi
