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
# Channel is tag-driven: a prerelease tag (vX.Y.Z-rc.N) → beta feed.
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

# Never block on a credential prompt, and abort a clone/push that stalls instead
# of hanging the CI step indefinitely. macOS runners occasionally hang on the
# registry git transfer (observed: a >1h hang at "Cloning into …"); these env
# vars make git give up after ~30s of no progress so the retry loop can recover.
# (macOS runners have no `timeout`/`gtimeout`, so we rely on git's own knobs.)
export GIT_TERMINAL_PROMPT=0
export GIT_HTTP_LOW_SPEED_LIMIT="${GIT_HTTP_LOW_SPEED_LIMIT:-1000}"
export GIT_HTTP_LOW_SPEED_TIME="${GIT_HTTP_LOW_SPEED_TIME:-30}"

title="${1:?title required}"
name="${2:?feed name required}"
version="${3:?version required}"
ref="${4:?ref required}"
repo="${5:?repo required}"
source="${6:?source artifact/app required}"
os="${7:?os required}"
arch="${8:?arch required}"

case "$ref" in
  *-*) channel=beta ;;
  *)   channel=stable ;;
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

# Feed file: per (os, arch) [and channel].
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

echo "Uploading $artifact to release $ref" >&2
gh release upload "$ref" "$artifact" --clobber

# Publish the feed to the registry repo. Many jobs push here concurrently
# (per-platform desktop feeds + the CLI cli.json), so rebase-retry the push.
work="/tmp/registry-${name}-${os}-${arch}"
for i in 1 2 3; do
  rm -rf "$work"
  if git clone "https://x-access-token:${REGISTRY_TOKEN}@github.com/neokapi/registry.git" "$work"; then
    break
  fi
  echo "registry clone stalled/failed (attempt $i), retrying…" >&2
  [ "$i" = 3 ] && { echo "publish-appcast.sh: registry clone failed after retries" >&2; exit 1; }
  sleep $((i * 5))
done
cp "$feed" "$work/"
cd "$work"
git config user.email "release-bot@neokapi.dev"
git config user.name "release-bot"
git add "$feed"
if git diff --staged --quiet; then
  echo "appcast unchanged; nothing to publish" >&2
  exit 0
fi
git commit -m "appcast: ${name} ${version} ${os}/${arch} (${channel})"
for i in 1 2 3 4 5; do
  if git push; then
    exit 0
  fi
  echo "push race on registry, rebasing (attempt $i)…" >&2
  git pull --rebase --no-edit || true
done
echo "publish-appcast.sh: failed to push $feed after retries" >&2
exit 1
