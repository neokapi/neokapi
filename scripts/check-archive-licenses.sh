#!/usr/bin/env bash
#
# Guard: every release archive carries the license text of the work inside it.
#
# The npm side of this is `check-package-licenses.sh`. This is the release-
# archive side, and it exists because the same defect shipped here too: the
# `kapi-cli_*` tarballs were a bare binary and the `kapi-bowrain_*` archives
# were a binary plus a manifest, with no license text on either track — while
# both licenses this repository uses require it. Apache-2.0 §4(a): "You must
# give any other recipients of the Work or Derivative Works a copy of this
# License." AGPL-3.0 §4: "give all recipients a copy of this License along with
# the Program." An SPDX string in a plugin manifest, a Homebrew formula or a
# registry entry is a label; it is not the grant, and it is not a copy.
#
# The check packages a dummy tree through the real `package-cli.sh` rather than
# reading it, so it tests what actually ships. Adding an archive family without
# license text fails here instead of on a user's disk.
#
# EXPECTED below is the contract: archive-name prefix → the path inside the
# archive that must hold license text, and the license the text must be. A
# track whose license changes updates this table in the same commit that moves
# it, so the two can never drift apart silently.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# A dummy build output for one target, enough for the packager to produce one
# archive of every family it knows how to build.
bins="$work/bins/cli-bins-linux-amd64"
mkdir -p "$bins"
printf 'dummy\n' > "$bins/kapi"
printf 'dummy\n' > "$bins/kapi-bowrain"

bash scripts/package-cli.sh 0.0.0-check "$work/bins" "$work/out" \
  bowrain/plugin/cmd/kapi-bowrain/manifest.json >/dev/null 2>&1

# archive prefix : path inside the archive : SPDX id its text must match
EXPECTED="
kapi-cli_:LICENSE:Apache-2.0
kapi-bowrain_:bowrain/LICENSE:Apache-2.0
"

# Identify a license by a phrase only that license contains.
# Markers are matched with a bash substring test, never `printf | grep -q`:
# under pipefail, grep exiting on its first match leaves printf a broken pipe,
# and a correct license text is then reported as the wrong one.
license_marker() {
  case "$1" in
    Apache-2.0) printf '%s' 'Apache License' ;;
    AGPL-3.0)   printf '%s' 'GNU AFFERO GENERAL PUBLIC LICENSE' ;;
    *) echo "check-archive-licenses.sh: unknown license id '$1'" >&2; exit 1 ;;
  esac
}

rc=0
for row in $EXPECTED; do
  [ -n "$row" ] || continue
  prefix=${row%%:*}; rest=${row#*:}
  path=${rest%%:*}; spdx=${rest#*:}
  marker=$(license_marker "$spdx")

  found=0
  for archive in "$work/out/$prefix"*.tar.gz "$work/out/$prefix"*.zip; do
    [ -f "$archive" ] || continue
    found=1
    case "$archive" in
      *.tar.gz) text=$(tar -xOf "$archive" "$path" 2>/dev/null || true) ;;
      *.zip)    text=$(unzip -p "$archive" "$path" 2>/dev/null || true) ;;
    esac
    if [ -z "$text" ]; then
      echo "ERROR: $(basename "$archive") ships no license text at $path" >&2
      rc=1
    elif [[ $text != *"$marker"* ]]; then
      echo "ERROR: $(basename "$archive"): $path is not $spdx text" >&2
      rc=1
    fi
  done

  if [ "$found" = 0 ]; then
    echo "ERROR: package-cli.sh produced no '$prefix*' archive — the guard's" >&2
    echo "       EXPECTED table names a family the packager no longer builds." >&2
    rc=1
  fi
done

[ $rc -eq 0 ] || exit 1
echo "check-archive-licenses: every release archive ships its license text"
