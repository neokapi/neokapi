#!/usr/bin/env bash
# Run a command with a temporary directory of its own, and fail when the
# command leaves anything in it.
#
#   scripts/check-temp-leaks.sh <command> [args...]
#   scripts/check-temp-leaks.sh --self-test
#
# A test run that leaves files in the system temporary directory fills the
# disk a little more each time: on one machine with several agents running
# tests in parallel worktrees, 6,055 skeleton files, hundreds of socket
# directories and a 481 MB data root accumulated in a day. Each was a store
# nobody closed, a daemon killed before it could clean up, or a test that
# wrote outside t.TempDir(). This script makes such a leak fail the run that
# caused it.
#
# The command runs with TMPDIR set to a fresh directory, so os.TempDir() in Go,
# os.tmpdir() in Node and every child process they start write there. When the
# command ends, anything left in the directory is listed and the script exits
# 1, or with the command's own status when that is not zero. The directory is
# removed either way.
#
# Two kinds of entry are reused by every run rather than added per run, so
# they do not count:
#   node-compile-cache   Node's own module compile cache.
#   jest_<user>          Jest's transform cache.
set -uo pipefail

# allowed reports whether an entry left in the directory is one of the reused
# entries above.
allowed() {
  case "$1" in
    node-compile-cache) return 0 ;;
    jest_*) return 0 ;;
  esac
  return 1
}

# run_checked runs "$@" with a fresh TMPDIR and reports what it leaves.
run_checked() {
  local dir status leaked=() entry
  # The base is kept short: a Unix socket path is limited to about 100 bytes,
  # and daemons put their sockets under TMPDIR.
  dir="$(mktemp -d "${CHECK_TEMP_LEAKS_BASE:-/tmp}/ktl.XXXXXX")" || return 2
  TMPDIR="$dir" "$@"
  status=$?
  while IFS= read -r entry; do
    [ -n "$entry" ] || continue
    allowed "$entry" || leaked+=("$entry")
  done < <(ls -A "$dir")
  rm -rf "${dir:?}"
  if [ "${#leaked[@]}" -gt 0 ]; then
    echo "check-temp-leaks: '$*' left ${#leaked[@]} entr$([ "${#leaked[@]}" -eq 1 ] && echo y || echo ies) in its temporary directory:" >&2
    printf '  %s\n' "${leaked[@]:0:40}" >&2
    [ "${#leaked[@]}" -gt 40 ] && echo "  ... and $((${#leaked[@]} - 40)) more" >&2
    echo "check-temp-leaks: close what you open, remove what you create, and write test files under t.TempDir()." >&2
    [ "$status" -ne 0 ] && return "$status"
    return 1
  fi
  return "$status"
}

# The self-test commands are single-quoted on purpose: $TMPDIR expands in the
# child shell, where it names the directory under test.
# shellcheck disable=SC2016
self_test() {
  local failures=0 out status
  expect() { # expect <want status> <name> <command...>
    local want=$1 name=$2
    shift 2
    out="$(run_checked "$@" 2>&1)"
    status=$?
    if [ "$status" -ne "$want" ]; then
      echo "self-test: $name: exit $status, want $want: $out" >&2
      failures=$((failures + 1))
    fi
  }
  expect 0 "a command that leaves nothing" bash -c 'touch "$TMPDIR/x" && rm "$TMPDIR/x"'
  expect 1 "a leaked file" bash -c 'touch "$TMPDIR/neokapi-skeleton-1"'
  expect 1 "a leaked directory" bash -c 'mkdir "$TMPDIR/kapi-sourcecode-1"'
  expect 1 "a leaked socket-like file beside an allowed entry" bash -c 'mkdir "$TMPDIR/node-compile-cache" && touch "$TMPDIR/kapi-fake-1.sock"'
  expect 1 "a lock directory under the temporary directory" bash -c 'mkdir "$TMPDIR/kapi-locks-501" && touch "$TMPDIR/kapi-locks-501/a.lock"'
  expect 0 "tool caches" bash -c 'mkdir "$TMPDIR/node-compile-cache" "$TMPDIR/jest_dy"'
  expect 3 "a failing command keeps its status" bash -c 'exit 3'
  expect 3 "a failing command that also leaks keeps its status" bash -c 'touch "$TMPDIR/y"; exit 3'
  expect 0 "TMPDIR reaches the command" bash -c 'case "$TMPDIR" in */ktl.*) exit 0 ;; *) exit 1 ;; esac'
  out="$(run_checked bash -c 'touch "$TMPDIR/z"' 2>&1)"
  if ! grep -q '^  z$' <<< "$out"; then
    echo "self-test: a leaked entry is not named" >&2
    failures=$((failures + 1))
  fi
  if [ "$failures" -gt 0 ]; then
    return 1
  fi
  echo "check-temp-leaks: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi
if [ "$#" -eq 0 ]; then
  echo "usage: scripts/check-temp-leaks.sh <command> [args...]" >&2
  exit 2
fi
run_checked "$@"
