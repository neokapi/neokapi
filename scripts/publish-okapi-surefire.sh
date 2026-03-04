#!/usr/bin/env bash
#
# publish-okapi-surefire.sh — Build and publish Okapi Surefire XML reports to
# a GitHub release.
#
# Usage:
#   ./scripts/publish-okapi-surefire.sh [OKAPI_VERSION]
#
# Checks out the specified Okapi version, runs Maven tests to generate Surefire
# XML reports, packages them into a tarball, and uploads to a GitHub release.
#
# The script requires a local Okapi checkout (JDK + Maven). It runs
# `mvn test -pl okapi/filters` with -Dmaven.test.failure.ignore=true so all
# reports are generated even when tests fail.
#
# Arguments:
#   OKAPI_VERSION  — Okapi Framework version (default: 1.48.0). Used as the
#                    Git tag (v1.48.0) and the GitHub release tag
#                    (okapi-surefire-1.48.0).
#
# Environment:
#   OKAPI_DIR      — Path to local Okapi checkout (default: ~/src/okapi/Okapi).
#   GITHUB_TOKEN   — GitHub token for creating/uploading the release.
#                    Falls back to `gh auth token` if unset.
#   SKIP_PUBLISH   — If set (e.g. SKIP_PUBLISH=1), build the tarball but don't
#                    upload to GitHub. Useful for local testing.

set -euo pipefail

OKAPI_VERSION="${1:-1.48.0}"
OKAPI_DIR="${OKAPI_DIR:-$HOME/src/okapi/Okapi}"
GITHUB_REPO="gokapi/gokapi"
RELEASE_TAG="okapi-surefire-${OKAPI_VERSION}"
ASSET_NAME="okapi-surefire.tar.gz"

# Resolve token.
GITHUB_TOKEN="${GITHUB_TOKEN:-$(gh auth token 2>/dev/null || true)}"
if [ -z "$GITHUB_TOKEN" ] && [ "${SKIP_PUBLISH:-}" = "" ]; then
    echo "ERROR: No GitHub token. Set GITHUB_TOKEN or log in with gh auth login." >&2
    exit 1
fi

# Find repo root.
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$SCRIPT_DIR/.."

# Validate Okapi checkout.
if [ ! -d "$OKAPI_DIR" ]; then
    echo "ERROR: Okapi checkout not found at $OKAPI_DIR" >&2
    echo "  Set OKAPI_DIR to your local Okapi Framework checkout." >&2
    exit 1
fi

if [ ! -f "$OKAPI_DIR/pom.xml" ]; then
    echo "ERROR: No pom.xml found in $OKAPI_DIR — not an Okapi checkout?" >&2
    exit 1
fi

# Validate JDK + Maven.
if ! command -v mvn &>/dev/null; then
    echo "ERROR: Maven (mvn) not found. Install Maven to run Okapi tests." >&2
    exit 1
fi

if ! command -v java &>/dev/null; then
    echo "ERROR: Java (java) not found. Install JDK to run Okapi tests." >&2
    exit 1
fi

# ---------------------------------------------------------------------------
# Checkout the requested version.
# ---------------------------------------------------------------------------

echo "=== Checking out Okapi v${OKAPI_VERSION} ==="

cd "$OKAPI_DIR"

# Check if the tag exists.
if ! git rev-parse "v${OKAPI_VERSION}" &>/dev/null; then
    echo "ERROR: Tag v${OKAPI_VERSION} not found in Okapi repo at $OKAPI_DIR" >&2
    echo "  Available tags: $(git tag -l 'v1.*' | tail -5 | tr '\n' ' ')" >&2
    exit 1
fi

git checkout "v${OKAPI_VERSION}" --quiet
echo "  Checked out v${OKAPI_VERSION}"

# ---------------------------------------------------------------------------
# Run Maven verify to generate Surefire + Failsafe XML.
# ---------------------------------------------------------------------------

echo ""
echo "=== Running Maven verify (mvn verify -pl okapi/filters -T4) ==="
echo "  This may take several minutes..."

# Run verify (not just test) to include Failsafe integration tests (*IT.java).
# This generates both target/surefire-reports/ and target/failsafe-reports/.
# -T4 runs 4 threads for faster builds.
# Failure ignore ensures we get reports even when tests fail.
mvn verify \
    -pl okapi/filters \
    -am \
    -T4 \
    -Dmaven.test.failure.ignore=true \
    -q \
    2>&1 | tail -5

echo "  Maven verify complete."

# ---------------------------------------------------------------------------
# Collect Surefire + Failsafe XML into a flat per-filter structure.
#
# Layout: okapi-surefire/{filter}/TEST-*.xml
#
# We glob both surefire-reports/ (unit tests) and failsafe-reports/
# (integration tests, *IT.java) from Maven's target directories and copy
# each XML into a directory named after the filter module.
# ---------------------------------------------------------------------------

echo ""
echo "=== Collecting Surefire + Failsafe XML reports ==="

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

OUT="$WORK_DIR/okapi-surefire"
mkdir -p "$OUT"

TOTAL_FILES=0
TOTAL_FILTERS=0

# Collect from both surefire-reports (unit) and failsafe-reports (integration).
for report_dir in "$OKAPI_DIR"/okapi/filters/*/target/surefire-reports "$OKAPI_DIR"/okapi/filters/*/target/failsafe-reports; do
    if [ ! -d "$report_dir" ]; then
        continue
    fi

    # Extract filter name from path (e.g. .../filters/html/target/... → html)
    filter_path="${report_dir%/target/*}"
    filter="$(basename "$filter_path")"

    # Count XML files.
    xml_count=$(find "$report_dir" -name "TEST-*.xml" -type f | wc -l | tr -d ' ')
    if [ "$xml_count" -eq 0 ]; then
        continue
    fi

    # Copy XML files to flat structure (may merge surefire + failsafe for same filter).
    mkdir -p "$OUT/$filter"
    cp "$report_dir"/TEST-*.xml "$OUT/$filter/"
done

# Print per-filter stats from the collected output.
for filter_dir in "$OUT"/*/; do
    filter="$(basename "$filter_dir")"
    xml_count=$(find "$filter_dir" -name "TEST-*.xml" -type f | wc -l | tr -d ' ')

    pass=0 fail=0 skip=0 error=0
    for xml_file in "$filter_dir"/TEST-*.xml; do
        # Extract counts from the testsuite element (macOS-compatible sed).
        suite_tests=$(sed -n 's/.*tests="\([0-9]*\)".*/\1/p' "$xml_file" 2>/dev/null | head -1)
        suite_tests="${suite_tests:-0}"
        suite_fail=$(sed -n 's/.*failures="\([0-9]*\)".*/\1/p' "$xml_file" 2>/dev/null | head -1)
        suite_fail="${suite_fail:-0}"
        suite_error=$(sed -n 's/.*errors="\([0-9]*\)".*/\1/p' "$xml_file" 2>/dev/null | head -1)
        suite_error="${suite_error:-0}"
        suite_skip=$(sed -n 's/.*skipped="\([0-9]*\)".*/\1/p' "$xml_file" 2>/dev/null | head -1)
        suite_skip="${suite_skip:-0}"
        pass=$((pass + suite_tests - suite_fail - suite_error - suite_skip))
        fail=$((fail + suite_fail))
        error=$((error + suite_error))
        skip=$((skip + suite_skip))
    done

    total=$((pass + fail + error + skip))
    echo "  $filter: $xml_count files, $total tests (pass=$pass fail=$fail error=$error skip=$skip)"

    TOTAL_FILES=$((TOTAL_FILES + xml_count))
    TOTAL_FILTERS=$((TOTAL_FILTERS + 1))
done

echo ""
echo "  Total: $TOTAL_FILES XML files across $TOTAL_FILTERS filters"

# ---------------------------------------------------------------------------
# Package the tarball.
# ---------------------------------------------------------------------------

echo ""
echo "=== Packaging tarball ==="

cd "$WORK_DIR"
tar czf "$ASSET_NAME" okapi-surefire/

SIZE=$(du -sh "$ASSET_NAME" | cut -f1)
echo "  $ASSET_NAME: $SIZE ($TOTAL_FILES files across $TOTAL_FILTERS filters)"

# Also copy to repo root for immediate local use.
cp "$ASSET_NAME" "$REPO_ROOT/$ASSET_NAME"
echo "  Copied to $REPO_ROOT/$ASSET_NAME"

# ---------------------------------------------------------------------------
# Publish to GitHub release.
# ---------------------------------------------------------------------------

if [ "${SKIP_PUBLISH:-}" != "" ]; then
    echo ""
    echo "=== SKIP_PUBLISH set — not uploading to GitHub ==="
    echo "Tarball available at: $REPO_ROOT/$ASSET_NAME"
    exit 0
fi

echo ""
echo "=== Publishing to GitHub release: $RELEASE_TAG ==="

# Delete existing release if present (to replace the asset).
if gh release view "$RELEASE_TAG" --repo "$GITHUB_REPO" &>/dev/null; then
    echo "  Deleting existing release $RELEASE_TAG..."
    gh release delete "$RELEASE_TAG" --repo "$GITHUB_REPO" --yes --cleanup-tag
fi

RELEASE_BODY="Surefire XML test reports from Okapi Framework v${OKAPI_VERSION}.

Generated by running Maven tests on the okapi/filters module. Contains all
TEST-*.xml files from Surefire reports, organized by filter name.

- Filters: ${TOTAL_FILTERS}
- XML files: ${TOTAL_FILES}
- Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
- Script: scripts/publish-okapi-surefire.sh"

gh release create "$RELEASE_TAG" \
    --repo "$GITHUB_REPO" \
    --title "Okapi Surefire Reports v${OKAPI_VERSION}" \
    --notes "$RELEASE_BODY" \
    "$WORK_DIR/$ASSET_NAME"

echo ""
echo "Done. Release: https://github.com/$GITHUB_REPO/releases/tag/$RELEASE_TAG"

# Clean up the local tarball copy.
rm -f "$REPO_ROOT/$ASSET_NAME"
