#!/usr/bin/env bash
#
# fetch-okapi-bridge.sh — Download the Okapi bridge JAR from GitHub release.
#
# Usage:
#   ./scripts/fetch-okapi-bridge.sh
#
# Downloads the bridge tarball from the gokapi/okapi-bridge repository,
# extracts the JAR, and places it in a versioned directory at:
#   ~/.cache/gokapi/bridge/<bridge-version>-okapi<okapi-version>/okapi-bridge.jar
#
# This versioned layout prevents confusion when switching between bridge or
# Okapi Framework versions. It mirrors the okapi-testdata approach.
#
# Environment:
#   GITHUB_TOKEN         — GitHub token for authenticated requests (required;
#                          must have access to gokapi/okapi-bridge).
#   GH_TOKEN             — Alternative token variable (used by gh CLI).
#   OKAPI_BRIDGE_VERSION — Bridge version tag (default: v2.1.1).
#   OKAPI_VERSION        — Okapi Framework version suffix (default: 1.48.0).
#   FORCE_FETCH          — If set (e.g. FORCE_FETCH=1), re-download even when
#                          the JAR already exists.

set -euo pipefail

REPO="gokapi/okapi-bridge"
BRIDGE_VERSION="${OKAPI_BRIDGE_VERSION:-v2.1.1}"
OKAPI_VERSION="${OKAPI_VERSION:-1.48.0}"
ASSET_NAME="okapi-bridge-${BRIDGE_VERSION}-okapi${OKAPI_VERSION}.tar.gz"

# Resolve the token from either GITHUB_TOKEN or GH_TOKEN.
TOKEN="${GITHUB_TOKEN:-${GH_TOKEN:-}}"

BRIDGE_DIR="$HOME/.cache/gokapi/bridge/${BRIDGE_VERSION}-okapi${OKAPI_VERSION}"
JAR_PATH="$BRIDGE_DIR/okapi-bridge.jar"

# Skip if already present and not forced.
if [ -f "$JAR_PATH" ] && [ "${FORCE_FETCH:-}" = "" ]; then
    echo "okapi-bridge.jar already exists at $JAR_PATH. Set FORCE_FETCH=1 to re-download."
    echo "GOKAPI_BRIDGE_JAR=$JAR_PATH"
    exit 0
fi

# Create a temporary directory for the download.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Fetching $ASSET_NAME from $REPO @ $BRIDGE_VERSION..."

if [ -z "$TOKEN" ]; then
    echo "ERROR: No GitHub token found. Set GITHUB_TOKEN or GH_TOKEN." >&2
    echo "  The token must have access to the $REPO repository." >&2
    exit 1
fi

AUTH_HEADER_FILE="$TMPDIR/github_auth_header"
printf 'Authorization: token %s\n' "$TOKEN" > "$AUTH_HEADER_FILE"
chmod 600 "$AUTH_HEADER_FILE"

# Use the API endpoint to resolve the asset URL.
API_URL="https://api.github.com/repos/$REPO/releases/tags/$BRIDGE_VERSION"
echo "  Resolving asset from: $API_URL"

ASSET_URL=$(curl -sL \
    -H "@$AUTH_HEADER_FILE" \
    -H "Accept: application/vnd.github+json" \
    "$API_URL" \
    | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    for a in d.get('assets', []):
        if a['name'] == '$ASSET_NAME':
            print(a['url'])
            break
except: pass
" 2>/dev/null)

if [ -z "$ASSET_URL" ]; then
    echo "ERROR: Asset '$ASSET_NAME' not found in release '$BRIDGE_VERSION'." >&2
    echo "  The token may lack access to $REPO (cross-repo)." >&2
    echo "  In CI, set GOKAPI_REGISTRY_TOKEN (org secret) with access to $REPO." >&2
    exit 1
fi

HTTP_CODE=$(curl -sL -w "%{http_code}" \
    -H "@$AUTH_HEADER_FILE" \
    -H "Accept: application/octet-stream" \
    -o "$TMPDIR/bridge.tar.gz" \
    "$ASSET_URL")

if [ "$HTTP_CODE" != "200" ]; then
    echo "ERROR: Download failed with HTTP $HTTP_CODE" >&2
    exit 1
fi

echo "  Download succeeded."

# Extract the JAR from the tarball.
echo "Extracting JAR..."
tar -xzf "$TMPDIR/bridge.tar.gz" -C "$TMPDIR/" --include='*/gokapi-bridge-jar-with-dependencies.jar' --strip-components=0 2>/dev/null \
    || tar -xzf "$TMPDIR/bridge.tar.gz" -C "$TMPDIR/"

# Find the JAR (may be at root or in a subdirectory depending on tar structure).
JAR_FILE=$(find "$TMPDIR" -name 'gokapi-bridge-jar-with-dependencies.jar' -type f | head -1)

if [ -z "$JAR_FILE" ]; then
    echo "ERROR: JAR not found in tarball." >&2
    exit 1
fi

# Move to target location.
mkdir -p "$BRIDGE_DIR"
mv "$JAR_FILE" "$JAR_PATH"

echo "Installed to $JAR_PATH"
echo "GOKAPI_BRIDGE_JAR=$JAR_PATH"
