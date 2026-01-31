#!/bin/bash
# CLI launcher for Bowrain — launches the .app bundle via `open` so the
# process detaches and returns control to the shell immediately.
#
# This script lives at Bowrain.app/Contents/Resources/bin/bowrain.
# Resolve the .app bundle: bin/ → Resources/ → Contents/ → Bowrain.app/
APP_DIR="$(cd "$(dirname "$0")/../../.." && pwd)"

if [ $# -eq 0 ]; then
  open "$APP_DIR"
else
  open "$APP_DIR" --args "$@"
fi
