#!/usr/bin/env bash
#
# fetch-okapi-surefire.sh — Download Okapi Surefire XML reports from GitHub release.
#
# Usage:
#   ./scripts/fetch-okapi-surefire.sh
#
# Downloads the okapi-surefire tarball from a GitHub release and extracts it
# to ./okapi-surefire/<version>/ at the repository root. The versioned directory
# makes updates idempotent: bumping SUREFIRE_VERSION automatically picks up
# new data without needing FORCE_FETCH.
#
# Environment:
#   GITHUB_TOKEN          — Optional GitHub token for authenticated requests
#                           (avoids rate limits in CI).
#   OKAPI_SUREFIRE_TAG    — Override the release tag (default: okapi-surefire-1.48.0).
#   SUREFIRE_VERSION      — Override the local directory version (default: 1.48.0-v1).
#   FORCE_FETCH           — If set (e.g. FORCE_FETCH=1), re-download even when
#                           the versioned directory already exists.

set -euo pipefail

REPO="gokapi/gokapi"
TAG="${OKAPI_SUREFIRE_TAG:-okapi-surefire-1.48.0}"
VERSION="${SUREFIRE_VERSION:-1.48.0-v1}"
ASSET_NAME="okapi-surefire.tar.gz"

# Resolve token: env var > gh CLI.
GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token 2>/dev/null || true)}"

# Find repo root (directory containing go.work).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/.."
if [ ! -f "$REPO_ROOT/go.work" ]; then
    echo "ERROR: go.work not found at $REPO_ROOT — run from repo root or scripts/" >&2
    exit 1
fi

TARGET_DIR="$REPO_ROOT/okapi-surefire/$VERSION"

# Skip if already present and not forced.
if [ -d "$TARGET_DIR" ] && [ "${FORCE_FETCH:-}" = "" ]; then
    echo "okapi-surefire/$VERSION/ already exists. Set FORCE_FETCH=1 to re-download."
    exit 0
fi

echo "Fetching $ASSET_NAME from release $TAG → okapi-surefire/$VERSION/..."

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

# Resolve the asset download URL via the GitHub API.
API_URL="https://api.github.com/repos/$REPO/releases/tags/$TAG"
echo "  Resolving asset from: $API_URL"

CURL_AUTH_ARGS=()
if [ -n "$AUTH_HEADER_FILE" ]; then
    CURL_AUTH_ARGS=(-H "@$AUTH_HEADER_FILE")
fi

ASSET_URL=$(curl -sL \
    ${CURL_AUTH_ARGS[@]+"${CURL_AUTH_ARGS[@]}"} \
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
    ${CURL_AUTH_ARGS[@]+"${CURL_AUTH_ARGS[@]}"} \
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
echo "Extracting to okapi-surefire/$VERSION/..."
rm -rf "$TARGET_DIR"
mkdir -p "$TARGET_DIR"
tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR" --strip-components=1 2>/dev/null \
    || tar -xzf "$TMPDIR/$ASSET_NAME" -C "$TARGET_DIR"

FILE_COUNT=$(find "$TARGET_DIR" -type f | wc -l | tr -d ' ')
FILTER_COUNT=$(find "$TARGET_DIR" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
echo "Done. Extracted $FILE_COUNT XML files across $FILTER_COUNT filters to okapi-surefire/$VERSION/"
