#!/usr/bin/env bash
#
# Download the modules of each Go module directory, with bounded retries.
#
#   scripts/go-mod-download.sh <module-dir>...
#
# The module proxy and the checksum database are third-party services, and they
# drop a request now and then. `go mod tidy -diff` reports that the same way it
# reports a manifest that is not tidy: a non-zero exit. Downloading first, with
# three tries, means a dropped stream costs a few seconds and the tidy check
# that follows reads from the module cache and fails only for drift.
#
# Each module resolves against its own go.mod (GOWORK=off), as `make ci-tidy`
# and `make audit-modules` read it. A module that cannot be downloaded after
# three tries fails the script, naming the directory.
set -uo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 <module-dir>..." >&2
  exit 2
fi

for dir in "$@"; do
  attempt=1
  until (cd "$dir" && GOWORK=off "${GO:-go}" mod download); do
    if [ "$attempt" -ge 3 ]; then
      echo "go-mod-download: could not download the modules of '$dir' after 3 attempts" >&2
      exit 1
    fi
    echo "go-mod-download: download in '$dir' failed (attempt $attempt); retrying" >&2
    attempt=$((attempt + 1))
    sleep $((attempt * 5))
  done
done
