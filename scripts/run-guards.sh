#!/usr/bin/env bash
# run-guards.sh: list or run the repo guards of one lane.
#
#   scripts/run-guards.sh --list <lane>   one line per guard: target, description, paths (tab-separated)
#   scripts/run-guards.sh <lane>          run every guard of the lane
#
# The guards are the files under scripts/guards/ (see scripts/lib/guards.mk).
# The lanes are lint, pre-push and ci. `make lint` runs its lane as
# prerequisites, scripts/pre-push-check.sh filters its lane by the changed
# paths, and the Repo guards CI job runs the ci lane through this script.
#
# A run goes on past a failing guard, names every guard that failed at the end
# and exits 1 when one did. Under GitHub Actions each guard's output is a
# collapsible group and each failure is an error annotation.

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

usage() {
    echo "usage: $0 [--list] <lint|pre-push|ci>" >&2
    exit 2
}

LIST=false
if [ "${1:-}" = "--list" ]; then
    LIST=true
    shift
fi
[ $# -eq 1 ] || usage
LANE="$1"

# The description is the help text on the target's rule line.
describe() {
    local target="$1" file="scripts/guards/$1.mk" line
    line="$(grep -E "^${target}:.*## " "$file" | head -n 1)" || true
    if [ -z "$line" ]; then
        echo "run-guards: $file has no '$target: ## <description>' rule" >&2
        return 1
    fi
    printf '%s\n' "${line#*## }"
}

GUARD_LIST="$(make -s --no-print-directory guard-list LANE="$LANE")"

if [ "$LIST" = true ]; then
    while IFS=$'\t' read -r target paths; do
        [ -n "$target" ] || continue
        printf '%s\t%s\t%s\n' "$target" "$(describe "$target")" "$paths"
    done <<< "$GUARD_LIST"
    exit 0
fi

in_actions() { [ "${GITHUB_ACTIONS:-}" = "true" ]; }

FAILED=()
SUMMARY=()
COUNT=0
while IFS=$'\t' read -r -u 3 target _paths; do
    [ -n "$target" ] || continue
    COUNT=$((COUNT + 1))
    desc="$(describe "$target")"
    if in_actions; then
        echo "::group::$target: $desc"
    else
        echo "── $target: $desc"
    fi
    start=$SECONDS
    if make --no-print-directory "$target"; then
        status=ok
    else
        status=FAILED
        FAILED+=("$target")
    fi
    in_actions && echo "::endgroup::"
    SUMMARY+=("$(printf '%-7s %4ss  %s' "$status" "$((SECONDS - start))" "$target")")
    if [ "$status" = FAILED ] && in_actions; then
        echo "::error title=Repo guard failed::$target: $desc (rerun with: make $target)"
    fi
done 3<<< "$GUARD_LIST"

if [ "$COUNT" -eq 0 ]; then
    echo "run-guards: no guard names the $LANE lane" >&2
    exit 1
fi
echo ""
printf '%s\n' "${SUMMARY[@]}"
echo ""
if [ "${#FAILED[@]}" -gt 0 ]; then
    echo "${#FAILED[@]} of $COUNT $LANE guard(s) failed:"
    printf '  make %s\n' "${FAILED[@]}"
    exit 1
fi
echo "All $COUNT $LANE guard(s) passed."
