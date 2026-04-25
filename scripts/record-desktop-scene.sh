#!/usr/bin/env bash
#
# record-desktop-scene.sh — record one desktop walkthrough scene against the
# real native Wails app talking to a real bowrain-server.
#
# Usage:
#   scripts/record-desktop-scene.sh <walkthrough-id> <scene-id>
#   e.g. scripts/record-desktop-scene.sh bowrain-desktop-tm-explorer browse
#
# Requires:
#   - macOS (uses AVFoundation + osascript)
#   - bin/Bowrain built (wails3 build DEV=true)
#   - ffmpeg installed
#   - BOWRAIN_TOKEN exported (mint via bowrain/scripts/device-auth.sh)
#   - BOWRAIN_SERVER_URL exported (default: https://dev.bowrain.cloud)
#
# Layout:
#   bowrain/website/scenes/<walkthrough-id>/0N-<scene-id>.applescript   ← the script
#   bowrain/website/scenes/<walkthrough-id>/0N-<scene-id>.webm           ← the output
#
# Window geometry is fixed at 1280x800 at (100,100) so crops are reproducible.

set -euo pipefail

WALKTHROUGH_ID="${1:?Usage: $0 <walkthrough-id> <scene-id>}"
SCENE_ID="${2:?Usage: $0 <walkthrough-id> <scene-id>}"

REPO_ROOT="$(git rev-parse --show-toplevel)"
APP="$REPO_ROOT/bowrain/apps/bowrain/bin/Bowrain"
SCENE_DIR="$REPO_ROOT/bowrain/website/scenes/$WALKTHROUGH_ID"

# Find the AppleScript file (matches 0N-<scene-id>.applescript).
APPLESCRIPT="$(ls "$SCENE_DIR"/0*-"$SCENE_ID".applescript 2>/dev/null | head -1 || true)"
if [ -z "$APPLESCRIPT" ]; then
  echo "no applescript found at $SCENE_DIR/0*-$SCENE_ID.applescript" >&2
  exit 1
fi

# Output webm sits next to the script.
WEBM="${APPLESCRIPT%.applescript}.webm"

: "${BOWRAIN_SERVER_URL:=https://dev.bowrain.cloud}"
: "${BOWRAIN_TOKEN:?must set BOWRAIN_TOKEN — run: BOWRAIN_TOKEN=\$(bash bowrain/scripts/device-auth.sh \$BOWRAIN_SERVER_URL)}"

echo "→ scene:    $WALKTHROUGH_ID/$SCENE_ID"
echo "→ script:   $APPLESCRIPT"
echo "→ output:   $WEBM"
echo "→ backend:  $BOWRAIN_SERVER_URL"

# 1. Launch the real native app with the bypass env vars.
[ -x "$APP" ] || { echo "missing $APP — run: cd bowrain/apps/bowrain && wails3 build DEV=true" >&2; exit 1; }

BOWRAIN_SERVER_URL="$BOWRAIN_SERVER_URL" BOWRAIN_TOKEN="$BOWRAIN_TOKEN" \
  "$APP" >/tmp/bowrain-recording.log 2>&1 &
APP_PID=$!
trap 'kill "$APP_PID" 2>/dev/null || true; kill "$FFMPEG_PID" 2>/dev/null || true' EXIT

# 2. Wait for the window to exist + position it deterministically.
osascript <<'OSASCRIPT'
on run
  set startTime to current date
  repeat
    if (current date) - startTime > 30 then error "Bowrain window did not appear within 30s"
    try
      tell application "System Events"
        if exists (window 1 of process "Bowrain") then exit repeat
      end tell
    end try
    delay 0.5
  end repeat
  tell application "System Events"
    tell process "Bowrain"
      set position of window 1 to {100, 100}
      set size of window 1 to {1280, 800}
    end tell
  end tell
end run
OSASCRIPT

# Hold for paint + initial data load before starting recording.
sleep 2

# 3. Start ffmpeg recording — crop full screen to the known window region.
RAW_OUTPUT="$(mktemp -t bowrain-rec-XXXXXX.mov)"
trap 'kill "$APP_PID" 2>/dev/null || true; kill "$FFMPEG_PID" 2>/dev/null || true; rm -f "$RAW_OUTPUT"' EXIT

# avfoundation: "4" is "Capture screen 0" on this machine; verify with:
#   ffmpeg -f avfoundation -list_devices true -i ""
SCREEN_INDEX="${SCREEN_INDEX:-4}"
ffmpeg -y -f avfoundation -capture_cursor 1 -framerate 30 -i "$SCREEN_INDEX" \
  -vf "crop=1280:800:100:100" \
  -c:v libvpx-vp9 -b:v 1M -pix_fmt yuv420p \
  "$RAW_OUTPUT.webm" </dev/null >/tmp/bowrain-ffmpeg.log 2>&1 &
FFMPEG_PID=$!

# 4. Run the scene.
osascript "$APPLESCRIPT"

# 5. Stop ffmpeg cleanly so the moov atom is finalized.
kill -INT "$FFMPEG_PID" 2>/dev/null || true
wait "$FFMPEG_PID" 2>/dev/null || true

# 6. Move the .webm into place.
mv "$RAW_OUTPUT.webm" "$WEBM"
echo "✓ recorded $WEBM ($(du -h "$WEBM" | cut -f1))"

# 7. Quit the app.
kill "$APP_PID" 2>/dev/null || true
wait "$APP_PID" 2>/dev/null || true
trap - EXIT
