#!/usr/bin/env bash
# test-integration.sh — run the //go:build integration lane for the Go module in
# the current working directory. Extra `go test` flags (-race, -shuffle=on, …)
# are passed through as arguments.
#
# Two things this exists to get right, both of which the previous inline recipe
# got wrong:
#
#   * The tag list must be ONE list. `go test` parses -tags with the standard
#     flag package, so a later -tags overwrites an earlier one rather than
#     merging. Appending `-tags=integration` to a command already carrying
#     `-tags fts5` therefore dropped fts5 silently, and every Postgres/SQLite
#     suite in this lane died on "no such function: fts5".
#
#   * The package set must be discovered, not filtered by test name. The old
#     recipe narrowed the run with `-run Integration`, which matches only the
#     tests that happen to spell "Integration" in their name — 12 of the 34
#     tagged tests. The cross-store parity suites (TestMemoryParity_*,
#     TestTermsParity_*, TestPostgresMemory_*) and the whole governed
#     change-set lifecycle were excluded by their own names.
#
# Scope: the packages that actually carry a tagged file. `-tags` can only ADD
# files to a build, never select them, so a plain `./...` would re-run the
# module's entire suite for the sake of a handful of tagged tests. The set is
# discovered from the tree rather than hand-listed, because a hand-listed set is
# how the lane went stale in the first place.
set -euo pipefail

TAGS="${GOTAGS_INTEGRATION:-fts5,integration}"
TIMEOUT="${GOTEST_TIMEOUT:-20m}"

# Capture the listing in one step rather than piping into the loop: a `go list`
# that fails inside a process substitution would abort nothing, and the lane
# would quietly run a subset. Under `set -e` this aborts instead.
listing="$(go list -f '{{.Dir}}	{{.ImportPath}}' ./...)"

paths=()
while IFS=$'\t' read -r dir importpath; do
	[ -n "$dir" ] || continue
	if grep -q '^//go:build integration' "$dir"/*_test.go 2>/dev/null; then
		paths+=("$importpath")
	fi
done <<<"$listing"

if [ ${#paths[@]} -eq 0 ]; then
	echo "[test-integration] no //go:build integration packages in this module — nothing to run"
	exit 0
fi

echo "[test-integration] tags=${TAGS}, packages:"
printf '  %s\n' "${paths[@]}"

# In CI, setup-go-modules names GO_ACTIONGRAPH_DIR, as for the Makefiles'
# $(GOTEST): the run writes its action graph there, and
# scripts/go-cache-hits.py reports the share of package builds the build
# cache served.
graph=()
if [ -n "${GO_ACTIONGRAPH_DIR:-}" ]; then
	graph=("-debug-actiongraph=$(mktemp "$GO_ACTIONGRAPH_DIR/actiongraph.XXXXXX")")
fi

# ${graph[@]+...}: bash before 4.4 (macOS /bin/bash is 3.2) treats an empty
# array as unbound under set -u.
exec go test -tags "$TAGS" ${graph[@]+"${graph[@]}"} "$@" -timeout "$TIMEOUT" -count=1 "${paths[@]}"
