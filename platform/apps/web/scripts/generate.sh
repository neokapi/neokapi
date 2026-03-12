#!/usr/bin/env bash
# Full pipeline: start server, seed data, capture screenshots + recordings, stop server.
# Usage: ./generate.sh [--build]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WEB_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "=== Gokapi Web App Screenshot & Recording Generation ==="
echo ""

# Step 1: Start Docker stack
echo "--- Step 1: Starting Docker stack ---"
"$SCRIPT_DIR/start-server.sh" "${1:-}"
echo ""

# Step 2: Install Playwright dependencies if needed
echo "--- Step 2: Checking Playwright ---"
cd "$WEB_DIR"
if [ ! -d "node_modules/@playwright" ]; then
  npm install --save-dev @playwright/test
fi
npx playwright install chromium --with-deps 2>/dev/null || true
echo ""

# Step 3: Run screenshot tests
echo "--- Step 3: Capturing screenshots ---"
npx playwright test --config playwright.config.ts
echo ""

# Step 4: Run recording tests
echo "--- Step 4: Recording screencasts ---"
npx playwright test --config playwright.recordings.config.ts
echo ""

# Step 5: Copy recordings to website (all 3 themes)
echo "--- Step 5: Copying recordings to website ---"
THEME=glass  "$SCRIPT_DIR/copy-recordings.sh"
THEME=light  "$SCRIPT_DIR/copy-recordings.sh"
THEME=aurora "$SCRIPT_DIR/copy-recordings.sh"
echo ""

# Step 6: Stop Docker stack
echo "--- Step 6: Stopping Docker stack ---"
"$SCRIPT_DIR/stop-server.sh"
echo ""

echo "=== Done! Screenshots and recordings are ready. ==="
echo "Screenshots: website/static/img/web-app/"
echo "Recordings:  website/static/video/web-app/"
