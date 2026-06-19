#!/usr/bin/env bash
#
# publish-appcast.sh — publish a desktop app's in-app-update feed.
#
# Runs on the macOS release runner after the .app has been Developer-ID signed,
# notarized, and stapled (by `wails3 tool sign --notarize`). It:
#
#   1. zips the notarized .app (the Wails native updater consumes a .zip whose
#      single top-level entry is the .app);
#   2. signs the zip's SHA-256 digest into a Sparkle-style appcast feed with
#      scripts/mkappcast (the signature the Wails updater verifies — see that
#      tool's doc for why generate_appcast is not used);
#   3. uploads the zip to the GitHub release (the appcast enclosure points here);
#   4. publishes the feed to the registry repo, served at
#      https://neokapi.github.io/registry/ alongside cli.json.
#
# Channel is tag-driven: a prerelease tag (vX.Y.Z-rc.N) → beta feed; a full tag
# → stable feed — matching the CLI's stable/beta split.
#
# Required env: UPDATE_ED25519_PRIVATE_KEY (base64 ed25519 private key),
#               REGISTRY_TOKEN (write access to neokapi/registry),
#               GH_TOKEN (for `gh release upload`).
#
# Usage: publish-appcast.sh <title> <name> <version> <ref> <repo> <app-bundle> <arch>
#   title       app display name + zip prefix, e.g. Kapi / Bowrain
#   name        feed basename, e.g. kapi-desktop / bowrain
#   app-bundle  path to the notarized .app (relative to repo root)
set -euo pipefail

title="${1:?title required}"
name="${2:?feed name required}"
version="${3:?version required}"
ref="${4:?ref required}"
repo="${5:?repo required}"
app="${6:?app bundle path required}"
arch="${7:?arch required}"

case "$ref" in
  *-*) channel=beta ;;
  *)   channel=stable ;;
esac

zip="${title}-${version}-macOS-${arch}.app.zip"
echo "Zipping notarized app $app → $zip" >&2
ditto -c -k --sequesterRsrc --keepParent "$app" "$zip"

if [ "$channel" = beta ]; then
  feed="appcast-${name}-beta.xml"
  chan_args=(--channel beta)
else
  feed="appcast-${name}.xml"
  chan_args=()
fi

go run ./scripts/mkappcast gen \
  --title "$title" \
  --version "$version" \
  "${chan_args[@]}" \
  --url-prefix "https://github.com/${repo}/releases/download/${ref}" \
  --out "$feed" \
  "$zip"

echo "Uploading $zip to release $ref" >&2
gh release upload "$ref" "$zip" --clobber

# Publish the feed to the registry repo. Desktop jobs run concurrently and the
# CLI job also pushes here, so rebase-retry around the push.
work="/tmp/registry-${name}"
git clone "https://x-access-token:${REGISTRY_TOKEN}@github.com/neokapi/registry.git" "$work"
cp "$feed" "$work/"
cd "$work"
git config user.email "release-bot@neokapi.dev"
git config user.name "release-bot"
git add "$feed"
if git diff --staged --quiet; then
  echo "appcast unchanged; nothing to publish" >&2
  exit 0
fi
git commit -m "appcast: ${name} ${version} (${channel})"
for i in 1 2 3 4 5; do
  if git push; then
    exit 0
  fi
  echo "push race on registry, rebasing (attempt $i)…" >&2
  git pull --rebase --no-edit || true
done
echo "publish-appcast.sh: failed to push $feed after retries" >&2
exit 1
