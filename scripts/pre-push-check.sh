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

# ── Repo hygiene (never gated: an absolute home path can land in any file) ──

run_check "Absolute home paths" ./scripts/check-abs-paths.sh

# Ungated too: retired framing lands wherever prose is edited, and it degrades
# quietly — a stale phrase reads as intentional to the next reader.
run_check "Retired framing in user-facing prose" ./scripts/check-vocabulary.sh

# Ungated: the interchange chrome came back once as one adapter method and one
# header action, and neither touched a file this check could have been gated on.
run_check "Desktop names no interchange format" ./scripts/check-desktop-interchange.sh

run_check "Reference dataset provenance" ./scripts/check-reference-provenance.sh

# Ungated: a hand-rolled run walk is written wherever someone needs "just the
# text", and it fails by showing less content rather than by failing. ~1s.
run_check "Run projections are declared" ./scripts/check-run-projection.sh

# Ungated: a walk selector dies in the app, not in the recorder that names it,
# so gating this on the recorder's own path would never fire. ~2s.
run_check "Walk selectors still exist" ./scripts/check-walk-selectors.sh

# Ungated because an em dash lands wherever prose is written, in any of the
# three tiers a reader sees it through: a Go string, an extracted catalog, a
# docs page. The catalogs part reports rather than fails on a surface the local
# tree has not extracted; the l10n workflow runs it with --require-extracted.
run_check "Em dashes in shipped prose" ./scripts/check-em-dashes.sh

# Ungated because the gate it proves runs only in the nightly convergence:
# nothing else here would notice it losing its teeth. ~1s, all in a scratch repo.
run_check "Sync backing gate" ./scripts/check-sync-backed.sh --self-test

# Ungated for the same reason: the delivery it proves runs only in scheduled
# jobs, so nothing else here would notice it pushing to the wrong place or
# staging more than the job owns. ~1s, all in scratch repos.
run_check "Scheduled auto-PR delivery" ./scripts/auto-pr.sh --self-test

# Ungated for the same reason, plus one specific to this repo: the bowrain
# checks below are gated on ^bowrain/core/ and ^bowrain/plugin/ only, so the
# main bowrain module has no local format gate at all — which is how nine files
# there drifted. ~1s for the whole tree.
run_check "Go formatting (gofmt -l -s)" ./scripts/check-gofmt.sh

# ── Go checks ──────────────────────────────────────────────────────────────

if matches '^core/' '^go\.(mod|sum)$' '^cli/' '^kapi/' '^go\.work'; then
    run_check "Go lint (framework)" make check-framework
fi

if matches '^bowrain/core/' '^bowrain/plugin/' '^bowrain/go\.(mod|sum)$'; then
    run_check "Go lint (bowrain)" make check-bowrain
fi

# The docs playground compiles the CLI to js/wasm. CI builds it, but only in the
# docs workflows — so a dependency that cannot target js/wasm (a TTY, signals,
# cgo, the clipboard) passed every local check and failed CI minutes later.
# Compile only, ~1s warm.
if matches '^core/' '^host/' '^cli/' '^kapi/' '^go\.work' '^go\.(mod|sum)$' '/go\.(mod|sum)$'; then
    run_check "js/wasm build (docs playground)" make check-wasm
fi

# ── go mod tidy drift check ───────────────────────────────────────────────

if matches '^go\.(mod|sum)$' '/go\.(mod|sum)$'; then
    run_check "go mod tidy" bash -c '
      for dir in . cli kapi apps/kapi-desktop bowrain/core bowrain/plugin bowrain; do
        (cd "$dir" && go mod tidy)
      done
      git diff --exit-code -- "**go.mod" "**go.sum"'
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
