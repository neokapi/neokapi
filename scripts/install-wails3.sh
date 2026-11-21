#!/usr/bin/env bash
#
# Install the wails3 CLI at the Wails version the desktop apps build against.
#
# The CLI generates the bindings and drives the build, so it has to be the
# version of the library the apps link. `go install …@latest` installs whatever
# upstream released last: from 2026-11-01 that was a 3.1.0 generator beside a
# 3.0.0 library. This reads the version from the two go.mod files that require
# Wails, refuses to go on when they disagree, and installs exactly that.
#
# Runs in bash on Linux, macOS and the Windows runners (Git Bash), so the
# release workflows and `make wails3-cli` install the same CLI.
#
#   scripts/install-wails3.sh            install
#   scripts/install-wails3.sh --version  print the version and stop
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
readonly MODULE="github.com/wailsapp/wails/v3"

# required_version prints the version of Wails a go.mod requires.
required_version() {
  awk -v module="$MODULE" '$1 == module { print $2; exit } $1 == "require" && $2 == module { print $3; exit }' "$1"
}

kapi="$(required_version "$root/apps/kapi-desktop/go.mod")"
bowrain="$(required_version "$root/bowrain/go.mod")"
if [ -z "$kapi" ] || [ -z "$bowrain" ]; then
  echo "install-wails3: could not read the Wails version (kapi-desktop='$kapi', bowrain='$bowrain')" >&2
  exit 1
fi
if [ "$kapi" != "$bowrain" ]; then
  echo "install-wails3: the desktop apps require different Wails versions (kapi-desktop=$kapi, bowrain=$bowrain)" >&2
  echo "install-wails3: one CLI cannot generate both apps' bindings; align the two go.mod files" >&2
  exit 1
fi

if [ "${1:-}" = "--version" ]; then
  echo "$kapi"
  exit 0
fi
echo "install-wails3: installing wails3 $kapi"
"${GO:-go}" install "$MODULE/cmd/wails3@$kapi"
