#!/usr/bin/env bash
# Guard: every macOS minimum the repository declares is the one in
# scripts/lib/macos-minimum, and that one is at least what the Go toolchain
# builds for.
#
#   scripts/check-macos-minimum.sh              check the repository
#   scripts/check-macos-minimum.sh --self-test  prove it on a scratch tree
#
# A desktop app's Info.plist (LSMinimumSystemVersion), its Taskfile and the
# release workflows (MACOSX_DEPLOYMENT_TARGET, -mmacosx-version-min) each state
# a macOS minimum. When they say less than the binary needs, the app installs
# and launches on a system it cannot run on and fails there, instead of macOS
# saying it needs a newer system. The Go toolchain's linker records the oldest
# macOS it supports in cmd/link/internal/ld/macho.go; Go 1.27 builds for macOS
# 13 while the apps declared 10.15 and the release workflows 11.0.
#
# Exit status: 0 every declaration agrees, 1 one does not, 2 an input is
# unreadable.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# go_minimum prints the macOS version the Go linker builds for, as major.minor.
go_minimum() {
  local macho
  macho="$(go env GOROOT)/src/cmd/link/internal/ld/macho.go"
  sed -nE 's/^[[:space:]]*macOS[[:space:]]*=[[:space:]]*macVersionFlag\{([0-9]+),[[:space:]]*([0-9]+),.*/\1.\2/p' "$macho" 2>/dev/null | head -1
}

# version_lt reports whether major.minor $1 is lower than $2.
version_lt() {
  local a1 a2 b1 b2
  IFS=. read -r a1 a2 <<< "$1"
  IFS=. read -r b1 b2 <<< "$2"
  [ "${a1:-0}" -lt "${b1:-0}" ] || { [ "${a1:-0}" -eq "${b1:-0}" ] && [ "${a2:-0}" -lt "${b2:-0}" ]; }
}

# check <root> <go minimum> prints every disagreement and returns 1 if any.
check() {
  local dir=$1 gomin=$2 want problems=0 file line value
  want="$(tr -d '[:space:]' < "$dir/scripts/lib/macos-minimum" 2>/dev/null)"
  if [ -z "$want" ]; then
    echo "check-macos-minimum: scripts/lib/macos-minimum is missing or empty" >&2
    return 2
  fi
  if [ -z "$gomin" ]; then
    echo "check-macos-minimum: cannot read the Go linker's macOS minimum" >&2
    return 2
  fi
  if version_lt "$want" "$gomin"; then
    echo "check-macos-minimum: scripts/lib/macos-minimum says $want, but the Go toolchain builds for macOS $gomin" >&2
    problems=1
  fi
  while IFS=: read -r file line value; do
    [ -n "$file" ] || continue
    if [ "$value" != "$want" ]; then
      echo "check-macos-minimum: $file:$line declares macOS $value; the minimum is $want (scripts/lib/macos-minimum)" >&2
      problems=1
    fi
  done < <(cd "$dir" && declarations)
  return "$problems"
}

# declarations prints file:line:version for each macOS minimum declared in the
# tracked files that state one.
declarations() {
  local files
  files="$(git ls-files 2>/dev/null || find . -type f | sed 's|^\./||')"
  grep -E '(^|/)(Info(\.dev)?\.plist|Taskfile\.yml|Makefile)$|^\.github/workflows/.*\.ya?ml$|^scripts/.*\.sh$' <<< "$files" |
    grep -v '^scripts/check-macos-minimum\.sh$' |
    while IFS= read -r f; do
      [ -f "$f" ] || continue
      # Info.plist: the <string> on the line after the key.
      awk -v f="$f" '/<key>LSMinimumSystemVersion<\/key>/ { want = 1; next }
        want && match($0, /<string>[0-9.]+<\/string>/) {
          v = substr($0, RSTART + 8, RLENGTH - 17); sub(/\.0$/, ".0", v); print f ":" NR ":" v; want = 0 }' "$f"
      grep -noE 'MACOSX_DEPLOYMENT_TARGET[:=][[:space:]]*"?[0-9]+\.[0-9]+' "$f" |
        sed -E "s|^([0-9]+):.*[^0-9.]([0-9]+\.[0-9]+)$|$f:\1:\2|"
      grep -noE 'mmacosx-version-min=[0-9]+\.[0-9]+' "$f" |
        sed -E "s|^([0-9]+):.*=([0-9]+\.[0-9]+)$|$f:\1:\2|"
    done
}

self_test() {
  local tmp failures=0 out
  tmp="$(mktemp -d)" || return 2
  mkdir -p "$tmp/scripts/lib" "$tmp/app/build/darwin" "$tmp/.github/workflows"
  echo "13.0" > "$tmp/scripts/lib/macos-minimum"
  printf '<dict>\n  <key>LSMinimumSystemVersion</key>\n  <string>13.0</string>\n</dict>\n' > "$tmp/app/build/darwin/Info.plist"
  printf 'env:\n  CGO_CFLAGS: "-mmacosx-version-min=13.0"\n  MACOSX_DEPLOYMENT_TARGET: "13.0"\n' > "$tmp/app/build/darwin/Taskfile.yml"
  printf 'run: echo "MACOSX_DEPLOYMENT_TARGET=13.0" >> env.txt\n' > "$tmp/.github/workflows/release.yml"

  expect() { # expect <want status> <name>
    out="$(check "$tmp" "$3" 2>&1)"
    local status=$?
    if [ "$status" -ne "$1" ]; then
      echo "self-test: $2: exit $status, want $1: $out" >&2
      failures=$((failures + 1))
    fi
  }
  expect 0 "every declaration agrees" 13.0
  expect 1 "the minimum is below the Go toolchain's" 14.0
  sed -i.bak 's/13.0/10.15/' "$tmp/app/build/darwin/Info.plist"
  expect 1 "an Info.plist below the minimum" 13.0
  sed -i.bak 's/10.15/13.0/' "$tmp/app/build/darwin/Info.plist"
  sed -i.bak 's/version-min=13.0/version-min=11.0/' "$tmp/app/build/darwin/Taskfile.yml"
  expect 1 "a -mmacosx-version-min below the minimum" 13.0
  sed -i.bak 's/version-min=11.0/version-min=13.0/' "$tmp/app/build/darwin/Taskfile.yml"
  sed -i.bak 's/TARGET=13.0/TARGET=11.0/' "$tmp/.github/workflows/release.yml"
  expect 1 "a workflow deployment target below the minimum" 13.0
  sed -i.bak 's/TARGET=11.0/TARGET=13.0/' "$tmp/.github/workflows/release.yml"
  expect 2 "an unreadable Go minimum" ""
  rm -rf "${tmp:?}"
  if [ "$failures" -gt 0 ]; then
    return 1
  fi
  echo "check-macos-minimum: self-test passed"
}

case "${1:-}" in
  --self-test)
    self_test
    exit $?
    ;;
  "")
    check "$root" "$(go_minimum)"
    status=$?
    [ "$status" -eq 0 ] && echo "check-macos-minimum: every declaration says macOS $(tr -d '[:space:]' < "$root/scripts/lib/macos-minimum")"
    exit "$status"
    ;;
  *)
    echo "usage: scripts/check-macos-minimum.sh [--self-test]" >&2
    exit 2
    ;;
esac
