#!/usr/bin/env bash
#
# Update a docs installation page with direct, platform-specific download links
# for a given release, replacing marker-delimited regions.
#
# Two products, two tag families, two pages:
#   kapi    (default)  — tags `vX.Y.Z`         → web/docs/kapi/get-started/installation.md
#   bowrain (--bowrain) — tags `bowrain-vX.Y.Z` → bowrain/web/docs/docs/getting-started/installation.md
#
# It reads the *actual* assets attached to the GitHub release (via `gh`), so it
# only ever emits links that resolve — Windows assets, which are signed and
# uploaded out of band the morning after the release (scripts/publish-windows-
# signed.sh), simply appear once they exist. Re-run the script after that step to
# fill them in.
#
# A platform with NO asset on the release is refused rather than written: the
# page would otherwise replace working links with a heading and nothing under
# it, which reads as "this platform has no download" and is worse than the
# older links it overwrites.
#
# Usage:
#   ./scripts/update-website-downloads.sh                          # latest STABLE kapi release
#   ./scripts/update-website-downloads.sh v1.2.0-rc1               # a specific kapi tag (incl. pre-release)
#   ./scripts/update-website-downloads.sh --bowrain                # latest STABLE bowrain release
#   ./scripts/update-website-downloads.sh --bowrain bowrain-v1.2.0-rc8
#
# Then review `git diff` on the page, commit, and push (the docs site rebuilds
# from the committed markdown).
set -euo pipefail

REPO="${REPO:-neokapi/neokapi}"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

PRODUCT="kapi"
if [ "${1:-}" = "--bowrain" ]; then
  PRODUCT="bowrain"
  shift
fi

case "$PRODUCT" in
  kapi)
    TAG_PREFIX="v"
    DEFAULT_PAGE="$REPO_ROOT/web/docs/kapi/get-started/installation.md"
    ;;
  bowrain)
    TAG_PREFIX="bowrain-v"
    DEFAULT_PAGE="$REPO_ROOT/bowrain/web/docs/docs/getting-started/installation.md"
    ;;
esac
PAGE="${PAGE:-$DEFAULT_PAGE}"

command -v gh >/dev/null || { echo "gh not found — install the GitHub CLI."; exit 1; }
[ -f "$PAGE" ] || { echo "installation page not found: $PAGE"; exit 1; }

# Resolve the tag: explicit arg, else the latest non-prerelease release in this
# product's tag family. Family matching is anchored (`v<digit>` / `bowrain-v<digit>`)
# so plugin tag families (vision-v0.2.2, bowrain-v-adjacent names) never match.
TAG="${1:-}"
if [ -z "$TAG" ]; then
  if [ "$PRODUCT" = "kapi" ]; then
    FAMILY_FILTER='select(.tagName|test("^v[0-9]"))'
  else
    FAMILY_FILTER='select(.tagName|test("^bowrain-v[0-9]"))'
  fi
  TAG="$(gh release list --repo "$REPO" --exclude-drafts --limit 60 \
    --json tagName,isPrerelease,isLatest \
    --jq "map(${FAMILY_FILTER}) | map(select(.isPrerelease==false)) | (map(select(.isLatest))[0] // .[0]) | .tagName")"
  [ -n "$TAG" ] && [ "$TAG" != "null" ] || { echo "could not resolve a latest stable ${PRODUCT} release; pass a tag explicitly."; exit 1; }
  echo ">> No tag given — using latest stable ${PRODUCT} release: $TAG"
fi
V="${TAG#"$TAG_PREFIX"}"
BASE="https://github.com/${REPO}/releases/download/${TAG}"

echo ">> Reading assets for $TAG ..."
ASSETS="$(gh release view "$TAG" --repo "$REPO" --json assets --jq '.assets[].name')"
[ -n "$ASSETS" ] || { echo "release $TAG has no assets."; exit 1; }

# Here-string, not a pipe: under `set -o pipefail` a `printf … | grep -q` reports
# the WRITE failure when grep exits on the first match and printf takes EPIPE.
have() { grep -qxF "$1" <<<"$ASSETS"; }

# A platform's links, under its heading. The heading is emitted only when at
# least one of its assets is on the release — a heading with nothing under it
# reads as "this platform has no download" on a page whose whole job is to hand
# out downloads, and it is strictly worse than the older links it would replace.
# Platforms that resolved nothing are collected and reported as a refusal below,
# so the page keeps what it has until the missing assets are published.
MISSING=0
EMPTY_SECTIONS=()
section() { # <heading> <label> <asset> [<label> <asset> …]
  local heading="$1"; shift
  local lines="" hits=0
  while [ "$#" -ge 2 ]; do
    local label="$1" asset="$2"; shift 2
    if have "$asset"; then
      lines+="$(printf -- '- **%s**: [`%s`](%s/%s)' "$label" "$asset" "$BASE" "$asset")"$'\n'
      hits=$((hits + 1))
    else
      MISSING=$((MISSING + 1))
      echo ">> (skip, not yet on release) $asset" >&2
    fi
  done
  if [ "$hits" -eq 0 ]; then
    EMPTY_SECTIONS+=("$heading")
    return 0
  fi
  echo "$heading"
  printf '%s' "$lines"
}

# --- kapi CLI binary downloads (family: kapi-cli_<ver>_<os>_<arch>) ---
cli_block() {
  echo "Direct downloads for **kapi ${V}** (CLI):"
  echo
  section "**macOS** (Apple Silicon)" \
    "macOS arm64" "kapi-cli_${V}_darwin_arm64.tar.gz"
  echo
  section "**Linux**" \
    "Linux amd64 (tar.gz)" "kapi-cli_${V}_linux_amd64.tar.gz" \
    "Linux arm64 (tar.gz)" "kapi-cli_${V}_linux_arm64.tar.gz" \
    "Linux amd64 (.deb)" "kapi-cli_${V}_amd64.deb" \
    "Linux arm64 (.deb)" "kapi-cli_${V}_arm64.deb" \
    "Linux amd64 (.rpm)" "kapi-cli_${V}_amd64.rpm" \
    "Linux arm64 (.rpm)" "kapi-cli_${V}_arm64.rpm"
  echo
  section "**Windows** (Authenticode-signed, portable zip)" \
    "Windows amd64" "kapi-cli_${V}_windows_amd64.zip" \
    "Windows arm64" "kapi-cli_${V}_windows_arm64.zip"
  echo
  echo "Verify a download against [\`checksums.txt\`](${BASE}/checksums.txt)."
}

# --- Kapi Desktop downloads (family: kapi-<ver>-<os>-<arch>) ---
desktop_block() {
  echo "Direct downloads for **Kapi Desktop ${V}**:"
  echo
  section "**macOS** (Apple Silicon)" \
    "macOS arm64 (.dmg)" "kapi-${V}-macOS-arm64.dmg"
  echo
  section "**Windows** (signed installer)" \
    "Windows amd64 (installer)" "kapi-${V}-windows-amd64-setup.exe" \
    "Windows arm64 (installer)" "kapi-${V}-windows-arm64-setup.exe" \
    "Windows amd64 (portable zip)" "kapi-${V}-windows-amd64.zip" \
    "Windows arm64 (portable zip)" "kapi-${V}-windows-arm64.zip"
  echo
  section "**Linux**" \
    "Linux amd64 (tar.gz)" "kapi-${V}-linux-amd64.tar.gz" \
    "Linux arm64 (tar.gz)" "kapi-${V}-linux-arm64.tar.gz"
}

# --- Bowrain Desktop downloads (family: bowrain-<ver>-<os>-<arch>) ---
# Note: the Windows artifact is an Authenticode-signed portable ZIP of
# Bowrain.exe — there is no installer. The macOS DMG is Apple Silicon only.
bowrain_desktop_block() {
  echo "Direct downloads for **Bowrain Desktop ${V}**:"
  echo
  section "**macOS** (Apple Silicon)" \
    "macOS arm64 (.dmg)" "bowrain-${V}-macOS-arm64.dmg"
  echo
  section "**Windows** (Authenticode-signed, portable zip)" \
    "Windows amd64" "bowrain-${V}-windows-amd64.zip" \
    "Windows arm64" "bowrain-${V}-windows-arm64.zip"
  echo
  section "**Linux**" \
    "Linux amd64 (tar.gz)" "bowrain-${V}-linux-amd64.tar.gz" \
    "Linux arm64 (tar.gz)" "bowrain-${V}-linux-arm64.tar.gz"
  echo
  echo "Verify a download against [\`checksums.txt\`](${BASE}/checksums.txt)."
}

# Splice generated content between <!-- BEGIN:NAME --> and <!-- END:NAME -->.
splice() { # <marker-name> <content-file>
  local name="$1" content="$2"
  local begin="<!-- BEGIN:${name} -->" end="<!-- END:${name} -->"
  grep -qF "$begin" "$PAGE" || { echo "marker '$begin' not found in $PAGE"; exit 1; }
  grep -qF "$end"   "$PAGE" || { echo "marker '$end' not found in $PAGE";   exit 1; }
  local tmp; tmp="$(mktemp)"
  awk -v b="$begin" -v e="$end" -v f="$content" '
    $0 ~ b {print; while ((getline line < f) > 0) print line; close(f); skip=1; next}
    $0 ~ e {skip=0}
    !skip {print}
  ' "$PAGE" > "$tmp"
  mv "$tmp" "$PAGE"
}

# Generate first, splice second. A platform that resolved no asset at all means
# this release cannot yet describe itself, and replacing the page's current
# links with a set that silently omits a platform is a regression a reader has
# no way to see. Refuse, leave the page alone, and name what is missing: the
# order is forced — publish the assets, then refresh the page.
refuse_if_a_platform_is_empty() {
  [ "${#EMPTY_SECTIONS[@]}" -gt 0 ] || return 0
  echo "refusing to update $PAGE for $TAG: no assets on the release for" >&2
  local heading
  for heading in "${EMPTY_SECTIONS[@]}"; do
    echo "  - ${heading//\*/}" >&2
  done
  echo "The page keeps its current links. Publish the missing assets (the Windows" >&2
  echo "binaries are signed out of band by scripts/publish-windows-signed.sh), then" >&2
  echo "re-run this script." >&2
  exit 1
}

if [ "$PRODUCT" = "kapi" ]; then
  cli_tmp="$(mktemp)"; desk_tmp="$(mktemp)"
  cli_block     > "$cli_tmp"
  desktop_block > "$desk_tmp"
  refuse_if_a_platform_is_empty
  splice "downloads-cli"     "$cli_tmp"
  splice "downloads-desktop" "$desk_tmp"
  rm -f "$cli_tmp" "$desk_tmp"
else
  bow_tmp="$(mktemp)"
  bowrain_desktop_block > "$bow_tmp"
  refuse_if_a_platform_is_empty
  splice "downloads-bowrain-desktop" "$bow_tmp"
  rm -f "$bow_tmp"
fi

echo ">> Updated $PAGE for $TAG."
if [ "$MISSING" -gt 0 ]; then
  echo ">> NOTE: $MISSING expected asset(s) were not on the release yet (likely the"
  echo "   Windows binaries, signed out of band). Re-run after publish-windows-signed.sh."
fi
echo ">> Review with: git -C \"$REPO_ROOT\" diff ${PAGE#"$REPO_ROOT"/}"
