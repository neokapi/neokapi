#!/usr/bin/env bash
#
# record-desktop-scene.sh — record one desktop walkthrough scene against
# the real native Wails app talking to a real bowrain-server.
#
# Usage:
#   scripts/record-desktop-scene.sh <walkthrough-id> <scene-id>
#
# Requires:
#   - macOS (uses AVFoundation + osascript)
#   - bin/Bowrain built (wails3 build DEV=true)
#   - ffmpeg installed
#   - BOWRAIN_TOKEN exported (mint via bowrain/scripts/device-auth.sh)
#   - BOWRAIN_SERVER_URL exported (default: https://dev.bowrain.cloud)
#   - macOS Accessibility + Screen Recording permission granted to the
#     hosting terminal (System Settings → Privacy & Security)
#
# Layout:
#   bowrain/website/scenes/<walkthrough-id>/0N-<scene-id>.applescript
#   bowrain/website/scenes/<walkthrough-id>/0N-<scene-id>.webm   ← output
#
# The recorder resizes the Bowrain window to a known logical size for
# consistent video dimensions, but reads the window position dynamically.
# It also auto-detects the display scale factor so Retina (2x) and
# non-Retina (1x) displays both produce the same output dimensions.

set -euo pipefail

WALKTHROUGH_ID="${1:?Usage: $0 <walkthrough-id> <scene-id>}"
SCENE_ID="${2:?Usage: $0 <walkthrough-id> <scene-id>}"

REPO_ROOT="$(git rev-parse --show-toplevel)"
APP="$REPO_ROOT/bowrain/apps/bowrain/bin/Bowrain"
SCENE_DIR="$REPO_ROOT/bowrain/website/scenes/$WALKTHROUGH_ID"

APPLESCRIPT="$(ls "$SCENE_DIR"/0*-"$SCENE_ID".applescript 2>/dev/null | head -1 || true)"
[ -n "$APPLESCRIPT" ] || { echo "no applescript at $SCENE_DIR/0*-$SCENE_ID.applescript" >&2; exit 1; }

WEBM="${APPLESCRIPT%.applescript}.webm"

: "${BOWRAIN_SERVER_URL:=https://dev.bowrain.cloud}"
: "${BOWRAIN_TOKEN:?must set BOWRAIN_TOKEN — run: BOWRAIN_TOKEN=\$(bash bowrain/scripts/device-auth.sh \$BOWRAIN_SERVER_URL)}"

# Output video dimensions. ffmpeg downsizes the captured pixels to this so
# the result is identical across Retina and non-Retina displays.
OUTPUT_W="${OUTPUT_W:-1280}"
OUTPUT_H="${OUTPUT_H:-800}"

echo "→ scene:    $WALKTHROUGH_ID/$SCENE_ID"
echo "→ script:   $APPLESCRIPT"
echo "→ output:   $WEBM"
echo "→ backend:  $BOWRAIN_SERVER_URL"

[ -x "$APP" ] || { echo "missing $APP — run: cd bowrain/apps/bowrain && wails3 build DEV=true" >&2; exit 1; }

# 1. Launch the real native app with the bypass env vars.
BOWRAIN_SERVER_URL="$BOWRAIN_SERVER_URL" BOWRAIN_TOKEN="$BOWRAIN_TOKEN" \
  "$APP" >/tmp/bowrain-recording.log 2>&1 &
APP_PID=$!
trap 'kill "$APP_PID" 2>/dev/null || true; kill "${FFMPEG_PID:-0}" 2>/dev/null || true' EXIT

# 2. Wait for the window, resize to OUTPUT_W x OUTPUT_H logical points,
#    and capture its actual position.
WINDOW_GEOMETRY=$(osascript <<OSASCRIPT
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
      set size of window 1 to {$OUTPUT_W, $OUTPUT_H}
      set wpos to position of window 1
      set wsize to size of window 1
      return (item 1 of wpos as string) & " " & (item 2 of wpos as string) & " " & (item 1 of wsize as string) & " " & (item 2 of wsize as string)
    end tell
  end tell
end run
OSASCRIPT
)

read -r WIN_X WIN_Y WIN_W WIN_H <<<"$WINDOW_GEOMETRY"
echo "→ window:   pos=($WIN_X,$WIN_Y) size=${WIN_W}x${WIN_H} (logical points)"

# 3. Detect display scale factor by comparing logical / pixel resolution.
#    Falls back to 1 if detection fails.
LOGICAL_W=$(osascript <<'OSA'
tell application "Finder"
  set b to bounds of window of desktop
  return item 3 of b
end tell
OSA
)
PIXEL_W=$(system_profiler SPDisplaysDataType 2>/dev/null | awk '/Resolution: [0-9]+ x [0-9]+/ {print $2; exit}')
SCALE=1
if [ -n "$PIXEL_W" ] && [ -n "$LOGICAL_W" ] && [ "$LOGICAL_W" -gt 0 ]; then
  SCALE=$(( PIXEL_W / LOGICAL_W ))
  [ "$SCALE" -lt 1 ] && SCALE=1
fi
echo "→ scale:    ${SCALE}x (logical ${LOGICAL_W} / pixel ${PIXEL_W})"

CROP_X=$(( WIN_X * SCALE ))
CROP_Y=$(( WIN_Y * SCALE ))
CROP_W=$(( WIN_W * SCALE ))
CROP_H=$(( WIN_H * SCALE ))

# 4. Resolve the AVFoundation screen device index dynamically. "Capture
#    screen 0" is the main display.
SCREEN_INDEX=$(set +o pipefail
ffmpeg -f avfoundation -list_devices true -i "" 2>&1 \
  | sed -n '/AVFoundation video devices:/,/AVFoundation audio devices:/p' \
  | sed -nE 's/.*\[([0-9]+)\] Capture screen 0.*/\1/p' \
  | head -1)
[ -n "$SCREEN_INDEX" ] || { echo "could not find AVFoundation screen device" >&2; exit 1; }
echo "→ av:       screen device index $SCREEN_INDEX"

# Hold for paint + initial data load before starting recording.
sleep 1

# 5. Start ffmpeg cropping to the dynamic window region, scaling output to
#    OUTPUT_W x OUTPUT_H so all videos have identical dimensions regardless
#    of source display scale.
RAW_OUTPUT="$(mktemp -t bowrain-rec-XXXXXX).webm"
trap 'kill "$APP_PID" 2>/dev/null || true; kill "${FFMPEG_PID:-0}" 2>/dev/null || true; rm -f "$RAW_OUTPUT"' EXIT

ffmpeg -y -f avfoundation -capture_cursor 1 -framerate 30 -i "$SCREEN_INDEX" \
  -vf "crop=${CROP_W}:${CROP_H}:${CROP_X}:${CROP_Y},scale=${OUTPUT_W}:${OUTPUT_H}" \
  -c:v libvpx-vp9 -b:v 1M -pix_fmt yuv420p \
  "$RAW_OUTPUT" </dev/null >/tmp/bowrain-ffmpeg.log 2>&1 &
FFMPEG_PID=$!

# 6. Run the scene.
osascript "$APPLESCRIPT"

# 7. Stop ffmpeg cleanly so the file is finalized.
kill -INT "$FFMPEG_PID" 2>/dev/null || true
wait "$FFMPEG_PID" 2>/dev/null || true

# 8. Move the .webm into place.
mv "$RAW_OUTPUT" "$WEBM"
echo "✓ recorded $WEBM ($(du -h "$WEBM" | cut -f1))"

# 9. Quit the app.
kill "$APP_PID" 2>/dev/null || true
wait "$APP_PID" 2>/dev/null || true
trap - EXIT
