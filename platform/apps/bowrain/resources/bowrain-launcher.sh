#!/bin/bash
# CLI launcher for Bowrain — launches the .app bundle via `open` so the
# process detaches and returns control to the shell immediately.
#
# This script lives at Bowrain.app/Contents/Resources/bin/bowrain.
# Resolve symlinks (e.g. Homebrew /opt/homebrew/bin/bowrain → real path)
# before computing the .app bundle: bin/ → Resources/ → Contents/ → Bowrain.app/
SOURCE="$0"
while [ -L "$SOURCE" ]; do
  DIR="$(cd -P "$(dirname "$SOURCE")" && pwd)"
  SOURCE="$(readlink "$SOURCE")"
  [[ "$SOURCE" != /* ]] && SOURCE="$DIR/$SOURCE"
done
APP_DIR="$(cd -P "$(dirname "$SOURCE")/../../.." && pwd)"

if [ $# -eq 0 ]; then
  open "$APP_DIR"
else
  open "$APP_DIR" --args "$@"
fi
