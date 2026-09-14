#!/usr/bin/env bash
#
# fetch-corpus.sh — Download Tier B format corpora from the format-corpus release.
#
# Usage:
#   ./scripts/fetch-corpus.sh             # fetch every corpus-*.tar.gz asset
#   ./scripts/fetch-corpus.sh <id>        # fetch one format's corpus
#   FORMAT=<id> ./scripts/fetch-corpus.sh # same, via environment
#
# The corpus store (docs/internals/format-maturity.md §2.5) is a GitHub
# release on neokapi/neokapi tagged format-corpus-vN with one asset per
# format: corpus-<id>.tar.gz, published merge-never-drop by
# scripts/publish-corpus.sh. This script resolves the lexically-latest
# format-corpus-v* tag, downloads the requested assets, and extracts each
# into corpus/<tag>/<id>/ at the repository root (gitignored). Tests resolve
# these files via the `corpus:<relpath>` input scheme (FindCorpusRoot in
# core/format/spec/helpers.go) and skip — never fail — when the corpus is
# absent, pointing at `make fetch-corpus`.
#
# The versioned directory makes updates idempotent: a respin under a new
# format-corpus-vN tag is picked up automatically without FORCE_FETCH, and
# already-extracted corpus/<tag>/<id>/ dirs are skipped otherwise. A missing
# release (or a missing per-format asset) is a notice and exit 0, not an
# error — the corpus store starts empty.
#
# Environment:
#   GITHUB_TOKEN — Optional GitHub token for authenticated requests
#                  (avoids rate limits in CI).
#   CORPUS_TAG   — Override the release tag (default: lexically-latest
#                  format-corpus-v* tag on the repo).
#   FORMAT       — Fetch only corpus-<FORMAT>.tar.gz (same as the positional
#                  argument; the environment variable wins).
#   FORCE_FETCH  — If set (e.g. FORCE_FETCH=1), re-download even when the
#                  versioned directory already exists.

set -euo pipefail

REPO="neokapi/neokapi"
TAG_PREFIX="format-corpus-v"
FORMAT="${FORMAT:-${1:-}}"

# Find repo root (directory containing go.work).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/.."
if [ ! -f "$REPO_ROOT/go.work" ]; then
    echo "ERROR: go.work not found at $REPO_ROOT — run from repo root or scripts/" >&2
    exit 1
fi

# Create a temporary directory for downloads and API responses.
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

# Build a curl header file: the API-version header always, plus the auth
# token when available (kept off the process command line). Using one
# unconditional file keeps every curl call identical — an empty optional
# args array would trip `set -u` under macOS's stock bash 3.2.
HEADER_FILE="$TMPDIR/github_headers"
printf 'X-GitHub-Api-Version: 2022-11-28\n' > "$HEADER_FILE"
if [ -n "${GITHUB_TOKEN:-}" ]; then
    printf 'Authorization: token %s\n' "$GITHUB_TOKEN" >> "$HEADER_FILE"
fi
chmod 600 "$HEADER_FILE"

# Resolve the release tag: explicit override, else the lexically-latest
# format-corpus-v* tag among the repo's releases.
TAG="${CORPUS_TAG:-}"
if [ -z "$TAG" ]; then
    TAG=$(curl -sL \
        -H "@$HEADER_FILE" \
        -H "Accept: application/vnd.github+json" \
        "https://api.github.com/repos/$REPO/releases?per_page=100" \
        | python3 -c "
import json, sys
try:
    releases = json.load(sys.stdin)
    tags = [r['tag_name'] for r in releases
            if isinstance(r, dict) and str(r.get('tag_name', '')).startswith('$TAG_PREFIX')]
    if tags:
        print(max(tags))
except Exception:
    pass
" 2>/dev/null)
fi

if [ -z "$TAG" ]; then
    echo "notice: no $TAG_PREFIX* release exists yet on $REPO — nothing to fetch."
    echo "  Publish one with 'make publish-corpus' (stages from corpus-staging/<id>/)."
    exit 0
fi

# Fetch the release metadata. A 404 here means the (overridden) tag does not
# exist yet — graceful notice, not an error.
API_URL="https://api.github.com/repos/$REPO/releases/tags/$TAG"
RELEASE_JSON="$TMPDIR/release.json"
echo "Resolving corpus assets from release $TAG..."
HTTP_CODE=$(curl -sL -w "%{http_code}" \
    -H "@$HEADER_FILE" \
    -H "Accept: application/vnd.github+json" \
    -o "$RELEASE_JSON" \
    "$API_URL")
if [ "$HTTP_CODE" = "404" ]; then
    echo "notice: release '$TAG' does not exist yet on $REPO — nothing to fetch."
    echo "  Publish one with 'make publish-corpus' (stages from corpus-staging/<id>/)."
    exit 0
fi
if [ "$HTTP_CODE" != "200" ]; then
    echo "ERROR: listing release '$TAG' failed with HTTP $HTTP_CODE" >&2
    exit 1
fi

# "name<TAB>api-asset-url" per corpus-*.tar.gz asset. Authenticated requests
# to the browser-style /releases/download/ URL don't reliably follow the CDN
# redirect, so we use the API asset URL and download with the
# Accept: application/octet-stream header (same approach as
# fetch-okapi-testdata.sh).
ASSETS=$(python3 - "$RELEASE_JSON" <<'PY'
import json, sys
with open(sys.argv[1]) as f:
    d = json.load(f)
for a in d.get('assets', []):
    name = a.get('name', '')
    if name.startswith('corpus-') and name.endswith('.tar.gz'):
        print(f"{name}\t{a['url']}")
PY
)

if [ -n "$FORMAT" ]; then
    WANT="corpus-$FORMAT.tar.gz"
    MATCHED=$(printf '%s\n' "$ASSETS" | awk -F'\t' -v w="$WANT" '$1 == w')
    if [ -z "$MATCHED" ]; then
        echo "notice: release '$TAG' has no asset '$WANT' — no corpus published for '$FORMAT' yet."
        if [ -n "$ASSETS" ]; then
            echo "  available: $(printf '%s\n' "$ASSETS" | cut -f1 | paste -sd' ' -)"
        fi
        exit 0
    fi
    ASSETS="$MATCHED"
fi

if [ -z "$ASSETS" ]; then
    echo "notice: release '$TAG' has no corpus-*.tar.gz assets yet — nothing to fetch."
    exit 0
fi

FETCHED=0
SKIPPED=0
while IFS=$'\t' read -r NAME URL; do
    [ -n "$NAME" ] || continue
    ID="${NAME#corpus-}"
    ID="${ID%.tar.gz}"
    TARGET_DIR="$REPO_ROOT/corpus/$TAG/$ID"

    # Skip if already present and not forced.
    if [ -d "$TARGET_DIR" ] && [ "${FORCE_FETCH:-}" = "" ]; then
        echo "corpus/$TAG/$ID/ already exists. Set FORCE_FETCH=1 to re-download."
        SKIPPED=$((SKIPPED + 1))
        continue
    fi

    echo "Fetching $NAME from release $TAG → corpus/$TAG/$ID/..."
    HTTP_CODE=$(curl -sL -w "%{http_code}" \
        -H "@$HEADER_FILE" \
        -H "Accept: application/octet-stream" \
        -o "$TMPDIR/$NAME" \
        "$URL")
    if [ "$HTTP_CODE" != "200" ]; then
        echo "ERROR: download of $NAME failed with HTTP $HTTP_CODE" >&2
        exit 1
    fi

    rm -rf "$TARGET_DIR"
    mkdir -p "$TARGET_DIR"
    tar -xzf "$TMPDIR/$NAME" -C "$TARGET_DIR"
    rm -f "$TMPDIR/$NAME"

    FILE_COUNT=$(find "$TARGET_DIR" -type f | wc -l | tr -d ' ')
    echo "  extracted $FILE_COUNT files to corpus/$TAG/$ID/"
    FETCHED=$((FETCHED + 1))
done <<< "$ASSETS"

echo "Done. Fetched $FETCHED format corpus asset(s), skipped $SKIPPED already present."
