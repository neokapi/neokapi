#!/usr/bin/env bash
# Compile what a pull request's Go jobs compile, so the build cache this run
# leaves behind serves them.
#
#   scripts/ci-warm-build-cache.sh              compile every workspace module
#   scripts/ci-warm-build-cache.sh --race       the same, as the jobs on main compile
#   scripts/ci-warm-build-cache.sh --self-test
#
# A pull request can read a cache saved on main and one saved on its own
# branch, and nothing else: a cache saved by another pull request is out of
# its reach. Pull request test jobs compile without the race detector, and the
# jobs on main compile with it, so nothing on main ever saved objects a pull
# request could use and the first run of every pull request compiled from
# nothing. The `Warm the pull request build cache` job in ci.yml runs this on
# main and saves the result under the cache flavor pull requests restore.
#
# With --race the script compiles what the test jobs on main and the nightly
# compile instead: race-instrumented, with atomic coverage. The `Warm the race
# build cache` job runs it and is the one writer of the "race" flavor.
#
# Each module's packages and test binaries are compiled with the tags the test
# targets use and no test is run. A module that does not compile is reported
# and the others still run: the test jobs are where a broken build fails, and
# a partial cache is worth more than none. The script fails only when nothing
# compiled, which means the cache it would save is empty.
set -uo pipefail

tags="${WARM_TAGS:-fts5}"
extra=()

# warm_modules compiles the tests of every module on stdin, one directory per
# line, and prints a summary. It returns 1 when no module compiled.
warm_modules() {
  local compile="$1" dir ok=0 failed=()
  while IFS= read -r dir; do
    [ -n "$dir" ] || continue
    if "$compile" "$dir"; then
      ok=$((ok + 1))
    else
      failed+=("$dir")
      echo "::warning::warm-build-cache: ${dir} did not compile; its objects are missing from the cache"
    fi
  done
  echo "warm-build-cache: compiled ${ok} module(s), ${#failed[@]} failed"
  [ "$ok" -gt 0 ]
}

# compile_tests builds the packages and test binaries of one module and runs
# no test: `-run '^$'` matches none.
compile_tests() {
  (cd "$1" && go test -tags "$tags" "${extra[@]}" -run '^$' ./... > /dev/null)
}

self_test() {
  local out status
  # shellcheck disable=SC2329  # called by name, through warm_modules
  fake() { [ "$1" != "bad" ]; }

  out="$(printf 'a\nbad\nb\n' | warm_modules fake)"
  status=$?
  [ "$status" -eq 0 ] || { echo "self-test: one failing module must not fail the run"; return 1; }
  grep -q 'compiled 2 module(s), 1 failed' <<< "$out" || { echo "self-test: wrong summary: $out"; return 1; }
  grep -q '::warning::.*bad did not compile' <<< "$out" || { echo "self-test: the failing module is not named"; return 1; }

  out="$(printf 'bad\n' | warm_modules fake)"
  status=$?
  [ "$status" -eq 1 ] || { echo "self-test: a run that compiled nothing must fail"; return 1; }

  out="$(printf '' | warm_modules fake)"
  status=$?
  [ "$status" -eq 1 ] || { echo "self-test: an empty module list must fail"; return 1; }

  echo "ci-warm-build-cache: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

if [ "${1:-}" = "--race" ]; then
  # The coverage flags change how the package under test is compiled, so the
  # profile is written and thrown away. -shuffle changes no object.
  cover="$(mktemp)"
  trap '/bin/rm -f "$cover"' EXIT
  extra=(-race -covermode=atomic "-coverprofile=$cover")
elif [ -n "${1:-}" ]; then
  echo "usage: $0 [--race | --self-test]" >&2
  exit 2
fi

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root" || exit 1
modules="$(go list -m -f '{{.Dir}}')" || exit 1
warm_modules compile_tests <<< "$modules"
