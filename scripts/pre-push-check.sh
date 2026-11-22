#!/bin/bash
# pre-push-check.sh: run only the checks relevant to your changes.
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
    CHANGED=$(git diff --name-only "$BASE"...HEAD 2>/dev/null; git diff --name-only 2>/dev/null; git diff --cached --name-only 2>/dev/null)
    CHANGED=$(echo "$CHANGED" | sort -u)

    if [ -z "$CHANGED" ]; then
        pass "No changes detected, nothing to check."
        exit 0
    fi
fi

matches() {
    [ "$RUN_ALL" = true ] && return 0
    for pattern in "$@"; do
        grep -qE "$pattern" <<< "$CHANGED" && return 0
    done
    return 1
}

echo ""

# ── Repo guards (scripts/guards/) ─────────────────────────────────────────

# Every guard of the pre-push lane. One with no GUARD_PATHS runs on every push;
# one with GUARD_PATHS runs when a changed path matches one of them. Each guard
# file says why it is gated or not.
GUARD_LIST="$(./scripts/run-guards.sh --list pre-push)"
while IFS=$'\t' read -r -u 3 target desc paths; do
    [ -n "$target" ] || continue
    if [ -n "$paths" ]; then
        read -r -a patterns <<< "$paths"
        matches "${patterns[@]}" || continue
    fi
    run_check "$desc" make --no-print-directory "$target"
done 3<<< "$GUARD_LIST"

# ── Go checks ──────────────────────────────────────────────────────────────

if matches '\.go$' '(^|/)go\.(mod|sum|work)$' '\.([cm]?[tj]s|[tj]sx)$' '(^|/)(LICENSE|package\.json|tsconfig[^/]*\.json)$' '^scripts/check-(go-module-boundaries|ts-license-boundary|workspace-versions)[.-]' '^pnpm-workspace\.yaml$' '^Makefile$'; then
    run_check "Module boundaries" make check-module-boundaries
fi

# The gate has to name every directory `make check-framework` reaches, and that
# is `golangci-lint run ./...` from the ROOT module plus host, cli, kapi and
# gen-refs. The root module is more than core/: kpz/, memory/, terms/,
# providers/ and scripts/ are all in it. Listing core/, cli/ and kapi/ alone let
# a testifylint failure in scripts/authoringeval pass here and fail CI, which is
# the same shape ci.yml's own framework filter was widened for. Only .go files
# under scripts/ count, so editing a shell script there stays fast.
if matches '^core/' '^host/' '^kpz/' '^memory/' '^terms/' '^providers/' '^scripts/.*\.go$' '^go\.(mod|sum)$' '^cli/' '^kapi/' '^go\.work'; then
    run_check "Go lint (framework)" make check-framework
fi

if matches '^bowrain/core/' '^bowrain/plugin/' '^bowrain/.*\.go$' '^bowrain/go\.(mod|sum)$'; then
    run_check "Go lint (bowrain)" make check-bowrain
fi

if matches '^bowrain/.*\.(go|sql)$' '^(core|host|kpz|memory|providers|terms|voice)/.*\.go$' \
    '^(bowrain/(core/|plugin/)?|host/)?go\.(mod|sum)$' '^go\.work(\.sum)?$' \
    '^(Makefile|bowrain/Makefile|scripts/pre-push-check\.sh)$'; then
    run_check "Bowrain tests (pull request)" make GITHUB_EVENT_NAME=pull_request ci-test-bowrain
fi

# The /evals index copies each eval dataset's date and headline number, so a
# republished dataset leaves the committed index stale until `make eval-index`
# regenerates it. The datasets sit under web/static/data/ and beside their pages
# as web/src/pages/<page>/_<name>.json, and the index tests also resolve the
# make targets the cards name. The package links no cgo and runs in seconds.
if matches '^web/static/data/' '^web/src/pages/[^/]+/_[^/]+\.json$' '^scripts/evalindex/' '^Makefile$'; then
    run_check "Eval index is current" go test ./scripts/evalindex/
fi

# The desktop module is its own go.mod, so the framework lint above stops at it.
# Same gate as the CI `kapi-desktop` job, and the same target that job runs, so
# a finding surfaces here rather than on the pull request.
if matches '^apps/kapi-desktop/'; then
    run_check "Go lint (Kapi Desktop)" make kapi-desktop-lint
fi

# The docs playground compiles the CLI to js/wasm. CI builds it, but only in the
# docs workflows, so a dependency that cannot target js/wasm (a TTY, signals,
# cgo, the clipboard) passed every local check and failed CI minutes later.
# Compile only, ~1s warm.
if matches '^core/' '^host/' '^cli/' '^kapi/' '^go\.work' '^go\.(mod|sum)$' '/go\.(mod|sum)$'; then
    run_check "js/wasm build (docs playground)" make check-wasm
fi

# The store suites under GOOS=js in Node, over the browser engine's own SQLite
# driver (core/storage/driver_js.go with packages/engine/src/sqlite.ts), then
# natively with every pool held to one connection. The js run also puts the
# change service through its conformance suite (core/change, and the host's
# service for a project, whose recorder writes the workspace log), which
# compiles the whole host test package for js, and runs the browser engine's
# own tests. The same gate as the CI `test-wasm-stores` job; the suites run in
# seconds once compiled. Needs `vp install` for @sqlite.org/sqlite-wasm.
if matches '^core/(storage|workspace|projector|projectdb|state|history|blockstore|change)/' '^(memory|terms|voice)/' '^host/' \
    '^kapi/cmd/kapi-wasm-cli/' '^packages/engine/src/sqlite\.ts$' '^scripts/wasm-stores/' '^pnpm-(lock|workspace)\.yaml$'; then
    run_check "Store suites (js/wasm)" make test-wasm-stores
    run_check "Store suites (one connection per pool)" make test-stores-oneconn
fi

# ── Generated TypeScript contract types ───────────────────────────────────

# @neokapi/contract-types is generated from Go: the change contract from
# core/change, the review model from core/review, the content model from
# core/model and its proto. CI gates them in reference-data-drift.yml; this runs
# the same drift gate, the generator's tests and the package's type-level tests
# when a source they read changes.
if matches '^core/' '^scripts/gen-contract-types/' '^packages/contract-types/'; then
    run_check "Contract types are fresh" make check-contract-types
    run_check "Contract types generator and type tests" make test-contract-types
fi

# ── go mod tidy drift check ───────────────────────────────────────────────

if matches '^go\.(mod|sum)$' '/go\.(mod|sum)$'; then
    run_check "go mod tidy" make ci-tidy
fi

# ── Build-derived multilingual artifacts ──────────────────────────────────

# On a same-repo pull request the l10n workflow commits drift in this tier
# back as a bot commit, which GitHub runs no checks on, so the pull request
# stalls until someone closes and reopens it. Regenerating here finds the drift
# before the push. The patterns are the sources the committed tier derives
# from. App sources are not among them: their qps dictionaries are build
# output. About a minute and a half warm; it rebuilds bin/kapi.
if matches '^(core|providers|memory|terms|cli|host)/.*\.go$' '^host/i18n/' '^core/i18n/catalogs/' \
    '^scripts/gen-refs/' '^packages/reference-data/data/' '^bowrain/emails/src/' \
    '^bowrain/mailer/subjects/en\.json$' '^bowrain/web/landing/(src/|index\.html$)' \
    '^packages/(i18n-react|kapi-format)/'; then
    run_check "Build-derived multilingual artifacts are fresh" make l10n-verify
fi

# ── Frontend checks ────────────────────────────────────────────────────────

if matches '^bowrain/packages/ui/' '^bowrain/apps/web/' '^bowrain/apps/bowrain/frontend/'; then
    run_check "Frontend (bowrain)" make frontend-check-all
fi

# The unit suites of the shared packages, the docs site, the landing page and
# the mobile app. Same filter as the CI `frontend-packages` job, and the same
# target that job runs. About a minute.
if matches '^packages/' '^web/' '^bowrain/web/landing/' '^bowrain/mobile/' '^core/place/testdata/' \
    '^bowrain/web/docs/src/css/palette\.generated\.css$' '^Makefile$' '^\.github/workflows/ci\.yml$' \
    '^pnpm-lock\.yaml$' '^package\.json$' '^pnpm-workspace\.yaml$'; then
    run_check "Frontend package suites" make ci-frontend-packages
fi

# ── Walkthrough harness ────────────────────────────────────────────────────

# harness/ is a standalone pnpm tree, so the frontend checks above stop at it.
# Same gate as the CI `harness` job, and the same target that job runs. The
# target installs the harness dependencies on first use.
if matches '^harness/'; then
    run_check "Harness typecheck + tests" make harness-check
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
