#!/usr/bin/env bash
#
# fetch-okapi-testdata.sh — Download Okapi filter test data from GitHub release.
#
# Usage:
#   ./scripts/fetch-okapi-testdata.sh
#
# Downloads the okapi-testdata tarball from a GitHub release and extracts it
# to ./okapi-testdata/<version>/ at the repository root. The versioned directory
# makes updates idempotent: bumping TESTDATA_VERSION automatically picks up
# new data without needing FORCE_FETCH.
#
# Environment:
#   GITHUB_TOKEN         — Optional GitHub token for authenticated requests
#                          (avoids rate limits in CI).
#   OKAPI_TESTDATA_TAG   — Override the release tag (default: okapi-testdata-1.48.0).
#   TESTDATA_VERSION     — Override the local directory version (default: 1.48.0-v2).
#   FORCE_FETCH          — If set (e.g. FORCE_FETCH=1), re-download even when
#                          the versioned directory already exists.

set -euo pipefail

REPO="neokapi/okapi-bridge"
TAG="${OKAPI_TESTDATA_TAG:-okapi-testdata-1.48.0}"
VERSION="${TESTDATA_VERSION:-1.48.0-v3}"
ASSET_NAME="okapi-testdata.tar.gz"

# Find repo root (directory containing go.work).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/.."
if [ ! -f "$REPO_ROOT/go.work" ]; then
    echo "ERROR: go.work not found at $REPO_ROOT — run from repo root or scripts/" >&2
    exit 1
fi

TARGET_DIR="$REPO_ROOT/okapi-testdata/$VERSION"

# Skip if already present and not forced.
if [ -d "$TARGET_DIR" ] && [ "${FORCE_FETCH:-}" = "" ]; then
    echo "okapi-testdata/$VERSION/ already exists. Set FORCE_FETCH=1 to re-download."
    exit 0
fi

echo "Fetching $ASSET_NAME from release $TAG → okapi-testdata/$VERSION/..."

# Create a temporary directory for the download.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Build auth header file if token is available, without exposing the token
# on the process command line.
AUTH_HEADER_FILE=""
if [ -n "${GITHUB_TOKEN:-}" ]; then
    AUTH_HEADER_FILE="$TMPDIR/github_auth_header"
    printf 'Authorization: token %s\n' "$GITHUB_TOKEN" > "$AUTH_HEADER_FILE"
    chmod 600 "$AUTH_HEADER_FILE"
fi

# Resolve the asset download URL via the GitHub API. Authenticated requests
# to the browser-style /releases/download/ URL don't reliably follow the CDN
# redirect, so we use the API to get the asset URL and download with the
# Accept: application/octet-stream header (same approach as fetch-okapi-bridge.sh).
API_URL="https://api.github.com/repos/$REPO/releases/tags/$TAG"
echo "  Resolving asset from: $API_URL"

CURL_AUTH_ARGS=()
if [ -n "$AUTH_HEADER_FILE" ]; then
    CURL_AUTH_ARGS=(-H "@$AUTH_HEADER_FILE")
fi

ASSET_URL=$(curl -sL \
    "${CURL_AUTH_ARGS[@]}" \
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
    echo "ERROR: Asset '$ASSET_NAME' not found in release '$TAG'." >&2
    echo "  Make sure the release '$TAG' exists with asset '$ASSET_NAME'" >&2
    echo "  in https://github.com/$REPO/releases" >&2
    exit 1
fi

echo "  Downloading from: $ASSET_URL"
HTTP_CODE=$(curl -sL -w "%{http_code}" \
    "${CURL_AUTH_ARGS[@]}" \
    -H "Accept: application/octet-stream" \
    -o "$TMPDIR/$ASSET_NAME" \
    "$ASSET_URL")

if [ "$HTTP_CODE" != "200" ]; then
    echo "ERROR: Download failed with HTTP $HTTP_CODE" >&2
    echo "  Make sure the release '$TAG' exists with asset '$ASSET_NAME'" >&2
    echo "  in https://github.com/$REPO/releases" >&2
    exit 1
fi

# Extract to versioned target directory.
echo "Extracting to okapi-testdata/$VERSION/..."
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR" --strip-components=1 2>/dev/null \
    || tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR"

FILE_COUNT=$(find "$TARGET_DIR" -type f | wc -l | tr -d ' ')
echo "Done. Extracted $FILE_COUNT files to okapi-testdata/$VERSION/"
