#!/usr/bin/env bash
# Check that every Mach-O a build produced records the macOS minimum it
# declares, and that the build log has no "built for newer macOS version"
# warning.
#
#   scripts/check-macho-minimum.sh [--minimum <version>] [--log <build log>] <path>...
#
# The minimum is scripts/lib/macos-minimum unless --minimum names another; a
# plugin release passes its own (scripts/macos-minimum.sh <plugin>).
#
# Each path is a binary, a static archive (every member object is checked) or
# a directory such as an app bundle (every Mach-O file under it is checked).
# A universal binary is checked for each architecture. The script exits 1 when
# an object records another minimum, records none, or the log holds the
# warning, and 2 when it is given nothing to check.
#
# scripts/check-macos-minimum.sh keeps the declarations in step; this script
# checks what the linker actually wrote, so a step that drops the deployment
# target, or an object built for a newer system, is caught where it happens.
set -uo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
want="$(tr -d '[:space:]' < "$root/scripts/lib/macos-minimum")"
log=""
paths=()
while [ "$#" -gt 0 ]; do
  case "$1" in
    --minimum) want="${2:-}"; shift 2 ;;
    --log) log="${2:-}"; shift 2 ;;
    *) paths+=("$1"); shift ;;
  esac
done
[ "${#paths[@]}" -gt 0 ] || { echo "usage: scripts/check-macho-minimum.sh [--minimum <version>] [--log <file>] <path>..." >&2; exit 2; }
[ -n "$want" ] || { echo "check-macho-minimum: no minimum given" >&2; exit 2; }

problems=0
checked=0

# minimums prints the minos of every LC_BUILD_VERSION, and the version of every
# LC_VERSION_MIN_MACOSX, that otool reports for one file.
minimums() {
  otool -arch all -l "$1" 2>/dev/null | awk '
    /cmd LC_BUILD_VERSION/ { b = 1; next }
    /cmd LC_VERSION_MIN_MACOSX/ { v = 1; next }
    b && $1 == "minos" { print $2; b = 0 }
    v && $1 == "version" { print $2; v = 0 }'
}

check_file() {
  local f=$1 found
  found="$(minimums "$f")"
  if [ -z "$found" ]; then
    echo "check-macho-minimum: $f records no macOS minimum" >&2
    problems=1
    return
  fi
  while IFS= read -r v; do
    checked=$((checked + 1))
    if [ "$v" != "$want" ]; then
      echo "check-macho-minimum: $f records macOS $v; the minimum is $want" >&2
      problems=1
    fi
  done <<< "$found"
}

for p in "${paths[@]}"; do
  if [ -d "$p" ]; then
    while IFS= read -r -d '' f; do
      if file -b "$f" | grep -q 'Mach-O'; then
        check_file "$f"
      fi
    done < <(find "$p" -type f -print0)
  elif [ -f "$p" ]; then
    check_file "$p"
  else
    echo "check-macho-minimum: $p does not exist" >&2
    problems=1
  fi
done

if [ -n "$log" ]; then
  if grep -E "built for newer '?macOS'? version|building for macOS-[0-9.]+, but linking with" "$log" >&2; then
    echo "check-macho-minimum: the build log above links an object built for a newer macOS" >&2
    problems=1
  fi
fi

if [ "$problems" -eq 0 ]; then
  echo "check-macho-minimum: ${checked} load command(s) record macOS ${want}"
fi
exit "$problems"
