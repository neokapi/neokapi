#!/usr/bin/env bash
#
# fetch-okapi-testdata.sh — Download Okapi filter test data from GitHub release.
#
# Usage:
#   ./scripts/fetch-okapi-testdata.sh
#
# The script downloads the okapi-testdata tarball from the GitHub release
# tagged "okapi-testdata-v1" and extracts it to ./okapi-testdata/ at the
# repository root.
#
# Environment:
#   GITHUB_TOKEN       — Optional GitHub token for authenticated requests
#                        (avoids rate limits in CI).
#   OKAPI_TESTDATA_TAG — Override the release tag (default: okapi-testdata-v1).
#   FORCE_FETCH        — If set (e.g. FORCE_FETCH=1), re-download even when
#                        ./okapi-testdata/ already exists.

set -euo pipefail

REPO="gokapi/gokapi"
TAG="${OKAPI_TESTDATA_TAG:-okapi-testdata-v1}"
ASSET_NAME="okapi-testdata.tar.gz"

# Find repo root (directory containing go.work).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/.."
if [ ! -f "$REPO_ROOT/go.work" ]; then
    echo "ERROR: go.work not found at $REPO_ROOT — run from repo root or scripts/" >&2
    exit 1
fi

TARGET_DIR="$REPO_ROOT/okapi-testdata"

# Skip if already present and not forced.
if [ -d "$TARGET_DIR" ] && [ "${FORCE_FETCH:-}" = "" ]; then
    echo "okapi-testdata/ already exists. Set FORCE_FETCH=1 to re-download."
    exit 0
fi

echo "Fetching $ASSET_NAME from release $TAG..."

# Create a temporary directory for the download.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Build auth header arguments if token is available, without exposing the token
# on the process command line. The header is written to a temp file and passed
# to curl via -H "@file".
CURL_AUTH_ARGS=()
if [ -n "${GITHUB_TOKEN:-}" ]; then
    AUTH_HEADER_FILE="$TMPDIR/github_auth_header"
    printf 'Authorization: token %s\n' "$GITHUB_TOKEN" > "$AUTH_HEADER_FILE"
    chmod 600 "$AUTH_HEADER_FILE"
    CURL_AUTH_ARGS=(-H "@$AUTH_HEADER_FILE")
fi

# Download the release asset via the GitHub API.
DOWNLOAD_URL="https://github.com/$REPO/releases/download/$TAG/$ASSET_NAME"
echo "  URL: $DOWNLOAD_URL"

HTTP_CODE=$(curl -sL -w "%{http_code}" \
    "${CURL_AUTH_ARGS[@]}" \
    -o "$TMPDIR/$ASSET_NAME" \
    "$DOWNLOAD_URL")

if [ "$HTTP_CODE" != "200" ]; then
    echo "ERROR: Download failed with HTTP $HTTP_CODE" >&2
    echo "  Make sure the release '$TAG' exists with asset '$ASSET_NAME'" >&2
    echo "  in https://github.com/$REPO/releases" >&2
    exit 1
fi

# Extract to target directory.
echo "Extracting to $TARGET_DIR..."
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR" --strip-components=1 2>/dev/null \
    || tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR"

FILE_COUNT=$(find "$TARGET_DIR" -type f | wc -l | tr -d ' ')
echo "Done. Extracted $FILE_COUNT files to okapi-testdata/"
