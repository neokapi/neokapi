#!/usr/bin/env bash
#
# Run the Wails bindings generator and fail on a warning about our own code.
#
#   scripts/wails-generate.sh wails3 generate bindings <flags…>
#
# The generator runs with cgo off (see WAILS_BINDINGS_ENV in the Makefile), so
# it cannot see the symbols Wails defines in its own cgo files, and it warns
# about each of them on every run. Those warnings are expected. A warning about
# any other package means a type of ours did not resolve, and the binding the
# generator wrote for it is wrong. Thirty expected warnings hide the one that
# matters, so this wrapper prints a count of the expected ones and fails on the
# rest.
#
#   scripts/wails-generate.sh --self-test
set -euo pipefail

readonly EXPECTED='/github.com/wailsapp/wails/'

# check_warnings reads the generator's output on stdin, prints every line that
# is not an expected warning, and returns 1 when a warning names our code.
check_warnings() {
  local plain expected=0 ours=0 line
  plain="$(sed -E $'s/\x1b\\[[0-9;]*m//g')"
  while IFS= read -r line; do
    case "$line" in
      *'[warn]'*"$EXPECTED"*) expected=$((expected + 1)) ;;
      *'[warn]'*)
        ours=$((ours + 1))
        echo "$line"
        ;;
      *' warnings emitted'* | *' warning emitted'*) ;;
      *) echo "$line" ;;
    esac
  done <<< "$plain"
  if [ "$expected" -gt 0 ]; then
    echo "wails-generate: $expected warning(s) inside the Wails module, expected with cgo off"
  fi
  if [ "$ours" -gt 0 ]; then
    echo "wails-generate: $ours warning(s) outside the Wails module: a bound type did not resolve" >&2
    return 1
  fi
}

self_test() {
  local out
  out="$(printf '%s\n' \
    ' INFO  Processed: 852 Packages' \
    $' \x1b[30;43m WARNING \x1b[0m [warn] /gomod/github.com/wailsapp/wails/v3@v3.0.0/pkg/application/menu.go:5:1: undefined: newMenuImpl' \
    ' WARNING  1 warnings emitted' | check_warnings)" ||
    { echo "self-test: an expected warning failed the run" >&2; return 1; }
  case "$out" in
    *'Processed: 852'*'1 warning(s) inside the Wails module'*) ;;
    *) echo "self-test: unexpected output: $out" >&2; return 1 ;;
  esac
  case "$out" in *newMenuImpl*) echo "self-test: an expected warning was printed" >&2; return 1 ;; esac

  if out="$(printf '%s\n' \
    ' WARNING  [warn] /work/apps/kapi-desktop/backend/app.go:12:3: undefined: Project' \
    ' WARNING  1 warnings emitted' | check_warnings 2>&1)"; then
    echo "self-test: a warning about our code passed" >&2
    return 1
  fi
  case "$out" in
    *'backend/app.go:12:3'*'outside the Wails module'*) ;;
    *) echo "self-test: the failing warning was not reported: $out" >&2; return 1 ;;
  esac
  echo "wails-generate: self-test passed"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit 0
fi
if [ "$#" -lt 1 ]; then
  echo "usage: $0 <generator command…>   |   $0 --self-test" >&2
  exit 2
fi

log="$(mktemp)"
trap 'rm -f "$log"' EXIT
status=0
"$@" > "$log" 2>&1 || status=$?
if [ "$status" -ne 0 ]; then
  cat "$log"
  echo "wails-generate: the generator exited with status $status" >&2
  exit "$status"
fi
check_warnings < "$log"
