#!/usr/bin/env bash
#
# publish-appcast.sh — publish a desktop app's in-app-update feed for one
# (os, arch).
#
# The Wails native updater swaps exactly one extracted top-level entry into
# os.Executable(), so each update artifact is a single-entry archive:
#   macOS   — a .zip of the .app           (zipped here from the notarized .app)
#   Windows — a .zip of the signed .exe     (already built/signed; pass it in)
#   Linux   — a .tar.gz of the binary       (already built; pass it in)
#
# It signs the artifact's SHA-256 digest into a Sparkle appcast (the signature
# the Wails verifier checks — see scripts/mkappcast for why generate_appcast is
# not used), uploads the artifact to the release, and publishes the feed to the
# registry repo (https://neokapi.github.io/registry/).
#
# One feed PER (os, arch): the appcast provider filters by sparkle:os but NOT by
# arch, so arch is disambiguated by the feed URL. The app fetches
# appcast-<name>-<runtime.GOOS>-<runtime.GOARCH>[-beta].xml.
#
# Channel is tag-driven: a prerelease tag (vX.Y.Z-rc.N) → beta feed only; a final
# tag (vX.Y.Z) → both the stable and beta feeds. Beta is a superset fast ring
# (see docs/internals/auto-update.md, "Release channels").
#
# Required env: UPDATE_ED25519_PRIVATE_KEY, REGISTRY_TOKEN, GH_TOKEN.
#
# Usage: publish-appcast.sh <title> <name> <version> <ref> <repo> <source> <os> <arch>
#   title    app display name (and macOS zip prefix), e.g. Kapi / Bowrain
#   name     feed basename, e.g. kapi / bowrain
#   source   .app dir (os=darwin) | .zip (os=windows) | .tar.gz (os=linux)
#   os       darwin | windows | linux   (the app's runtime.GOOS; names the feed)
#   arch     arm64 | amd64
set -euo pipefail

# Never block on a credential prompt.
export GIT_TERMINAL_PROMPT=0

# Hard wall-clock timeout for a single command. macOS runners ship no
# `timeout`/`gtimeout`, so we roll our own. The registry write now goes through
# the Contents API (publish_feed_via_registry_api), not git clone/push, because
# GitHub-hosted macOS runners deterministically stall on `git clone github.com`
# in the connect/TLS phase; this wrapper still guards `gh release upload` and the
# Contents API calls so a stalled request is killed and retried (observed:
# multi-hour hangs before any bytes
# move). Wrapping each network call in with_timeout makes the surrounding retry
# loops actually recover: a stalled call is killed at the deadline and retried.
with_timeout() {
  local secs="$1"; shift
  "$@" &
  local pid=$!
  (
    sleep "$secs"
    kill -TERM "$pid" 2>/dev/null || true
    sleep 5
    kill -KILL "$pid" 2>/dev/null || true
  ) &
  local killer=$!
  local rc=0
  wait "$pid" 2>/dev/null || rc=$?
  kill -TERM "$killer" 2>/dev/null || true
  wait "$killer" 2>/dev/null || true
  return "$rc"
}

# Upsert one file into the neokapi/registry repo via the GitHub Contents API,
# authenticated with REGISTRY_TOKEN. This replaces a git clone/commit/push:
# GitHub-hosted macOS runners deterministically stall on `git clone github.com`
# in the connect phase — every retry hangs, even with a hard timeout — which
# silently dropped the macOS in-app-update feeds for every release. `gh api`
# talks to api.github.com, which works on those same runners (the `gh release
# upload` calls below prove it). Each feed is a distinct path, so concurrent
# appcast jobs never contend for the same file, and no rebase dance is needed.
# Args: <file> <commit-message>. Returns nonzero only if all attempts fail.
publish_feed_via_registry_api() {
  local file="$1" msg="$2" path b64 sha
  path="$file"   # feeds live at the registry repo root
  b64=$(base64 < "$file" | tr -d '\n')
  for i in 1 2 3 4 5; do
    # Current blob sha if the file already exists (empty string for a new file).
    sha=$(GH_TOKEN="$REGISTRY_TOKEN" with_timeout 60 \
      gh api "repos/neokapi/registry/contents/${path}" --jq '.sha' 2>/dev/null || true)
    local args=(--method PUT "repos/neokapi/registry/contents/${path}"
                -f "message=${msg}" -f "content=${b64}")
    [ -n "$sha" ] && args+=(-f "sha=${sha}")
    if GH_TOKEN="$REGISTRY_TOKEN" with_timeout 120 gh api "${args[@]}" >/dev/null 2>&1; then
      echo "published ${path} to registry via contents API (attempt $i)" >&2
      return 0
    fi
    echo "registry contents API write failed for ${path} (attempt $i), retrying…" >&2
    sleep $((i * 5))
  done
  echo "publish-appcast.sh: failed to publish ${path} via contents API" >&2
  return 1
}

title="${1:?title required}"
name="${2:?feed name required}"
version="${3:?version required}"
ref="${4:?ref required}"
repo="${5:?repo required}"
source="${6:?source artifact/app required}"
os="${7:?os required}"
arch="${8:?arch required}"

# A prerelease publishes to the beta feed only; a final publishes to BOTH the
# stable and beta feeds, so the beta channel also carries finals and beta users
# never fall behind stable.
case "$ref" in
  *-*) channels="beta" ;;
  *)   channels="stable beta" ;;
esac

# sparkle:os value the Wails appcast provider matches against runtime.GOOS.
case "$os" in
  darwin)  sparkle_os=macos ;;
  windows) sparkle_os=windows ;;
  linux)   sparkle_os=linux ;;
  *) echo "publish-appcast.sh: unknown os '$os' (want darwin|windows|linux)" >&2; exit 1 ;;
esac

# Resolve the update artifact. macOS is zipped here from the notarized .app;
# Windows/Linux artifacts are already built (and on the release).
if [ "$os" = darwin ]; then
  artifact="${title}-${version}-macOS-${arch}.app.zip"
  echo "Zipping notarized app $source → $artifact" >&2
  ditto -c -k --sequesterRsrc --keepParent "$source" "$artifact"
else
  artifact="$source"
fi

echo "Uploading $artifact to release $ref" >&2
for i in 1 2 3; do
  if with_timeout 300 gh release upload "$ref" "$artifact" --clobber; then
    break
  fi
  echo "release upload stalled/failed (attempt $i), retrying…" >&2
  [ "$i" = 3 ] && { echo "publish-appcast.sh: release upload failed after retries" >&2; exit 1; }
  sleep $((i * 5))
done

# One signed feed per channel: a final writes the stable AND beta feeds; a
# prerelease writes the beta feed only. mkappcast must run from the repo root,
# so each channel's git work happens in an isolated subshell (its `cd` and
# `exit` stay local; a failed push still fails the script via `set -e`).
for channel in $channels; do
  suffix=""; [ "$channel" = beta ] && suffix="-beta"
  feed="appcast-${name}-${os}-${arch}${suffix}.xml"
  chan_args=(); [ "$channel" = beta ] && chan_args=(--channel beta)

  go run ./scripts/mkappcast gen \
    --title "$title" \
    --version "$version" \
    --os "$sparkle_os" \
    "${chan_args[@]}" \
    --url-prefix "https://github.com/${repo}/releases/download/${ref}" \
    --out "$feed" \
    "$artifact"

  # Publish the feed to the registry repo via the Contents API (see
  # publish_feed_via_registry_api) — git clone/push hangs on macOS runners.
  publish_feed_via_registry_api "$feed" \
    "appcast: ${name} ${version} ${os}/${arch} (${channel})"
done
