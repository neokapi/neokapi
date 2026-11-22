#!/usr/bin/env bash
# Print the oldest macOS a build targets.
#
#   scripts/macos-minimum.sh                 the minimum kapi, kapi-bowrain and
#                                            the desktop apps share
#   scripts/macos-minimum.sh <plugin>        a plugin's minimum: its own
#                                            plugins/<plugin>/macos-minimum when
#                                            one exists, the shared one otherwise
#   scripts/macos-minimum.sh --codename <v>  the Homebrew symbol for version v,
#                                            for `depends_on macos:`
#
# The shared minimum lives in scripts/lib/macos-minimum. A plugin declares its
# own only when a library it bundles needs a newer macOS than the shared
# minimum, and the file says which library. The release workflows build to this
# value, scripts/check-macho-minimum.sh holds the built binaries to it, and the
# Homebrew generators write it into the Casks and formulae.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# minimum prints the first non-comment line of a minimum file.
minimum() {
  local v
  v="$(grep -vE '^[[:space:]]*(#|$)' "$1" | head -1 | tr -d '[:space:]')"
  echo "$v"
}

case "${1:-}" in
  "")
    minimum "$root/scripts/lib/macos-minimum"
    ;;
  --codename)
    major="${2:-}"
    major="${major%%.*}"
    case "$major" in
      13) echo ventura ;;
      14) echo sonoma ;;
      15) echo sequoia ;;
      26) echo tahoe ;;
      *)
        echo "macos-minimum: no Homebrew symbol known for macOS ${2:-<none>}; add it here" >&2
        exit 1
        ;;
    esac
    ;;
  -*)
    echo "usage: scripts/macos-minimum.sh [<plugin> | --codename <version>]" >&2
    exit 2
    ;;
  *)
    if [ ! -d "$root/plugins/$1" ]; then
      echo "macos-minimum: no plugin named $1 under plugins/" >&2
      exit 2
    fi
    if [ -f "$root/plugins/$1/macos-minimum" ]; then
      minimum "$root/plugins/$1/macos-minimum"
    else
      minimum "$root/scripts/lib/macos-minimum"
    fi
    ;;
esac
