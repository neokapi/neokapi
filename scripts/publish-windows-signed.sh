#!/usr/bin/env bash
#
# Sign the Windows release artifacts with a Certum certificate and add them to
# the (already published) GitHub release. Run on a Mac after the release
# workflow has finished — macOS/Linux assets are published by CI; the Windows
# binaries are produced as workflow *artifacts* and signed here.
#
# Prereqs — have the certificate mounted first:
#   • SimplySign cloud (what neokapi uses): install SimplySign Desktop (brew
#     bundle) and LOG IN via its menu-bar app (mobile OTP) so the cloud cert is
#     mounted. The active session authorizes signing — there is NO card PIN and
#     NO per-signature prompt — so JSIGN_STOREPASS stays empty and the alias is
#     auto-discovered from the token. Just point at the PKCS#11 config:
#       export JSIGN_KEYSTORE=~/simplysign-pkcs11.cfg
#     where ~/simplysign-pkcs11.cfg contains:
#       name = SimplySign
#       library = /usr/local/lib/libSimplySignPKCS.dylib
#     (The SimplySign session is time-limited — log in shortly before signing.)
#   • PIN-protected token / physical card: set JSIGN_STORETYPE=CRYPTOCERTUM (card)
#     or keep PKCS11, plus JSIGN_STOREPASS=<PIN> and JSIGN_ALIAS=<label>.
#
# Usage:  ./scripts/publish-windows-signed.sh v1.2.3
#         RUN_ID=123456789 ./scripts/publish-windows-signed.sh v1.2.3   # pin the run
set -euo pipefail

TAG="${1:?usage: publish-windows-signed.sh <tag>   e.g. v1.2.3}"
REPO="${REPO:-neokapi/neokapi}"

STORETYPE="${JSIGN_STORETYPE:-PKCS11}"
TSA="${JSIGN_TSA:-http://time.certum.pl/}"
# SimplySign cloud has no card PIN and no per-signature prompt — the active
# SimplySign Desktop session is the authorization — so the store password is
# empty by default. (For a PIN-protected token or physical card, set JSIGN_STOREPASS.)
STOREPASS="${JSIGN_STOREPASS:-}"
command -v jsign >/dev/null || { echo "jsign not found — run 'brew bundle'"; exit 1; }
command -v gh    >/dev/null || { echo "gh not found — run 'brew bundle'"; exit 1; }

if [ "$STORETYPE" = "PKCS11" ]; then
  : "${JSIGN_KEYSTORE:?PKCS11 store needs JSIGN_KEYSTORE=/path/to/pkcs11.cfg (SunPKCS11 config)}"
fi

# Signing alias. For SimplySign (PKCS11) this is the certificate's PKCS#11 object
# label, which changes on every cert reissue — so auto-discover it from the live
# token when JSIGN_ALIAS isn't set (requires the SimplySign Desktop session).
ALIAS="${JSIGN_ALIAS:-}"
if [ -z "$ALIAS" ] && [ "$STORETYPE" = "PKCS11" ]; then
  ALIAS="$(keytool -list -storetype PKCS11 -keystore NONE \
             -providerclass sun.security.pkcs11.SunPKCS11 -providerarg "$JSIGN_KEYSTORE" \
             -storepass "$STOREPASS" </dev/null 2>/dev/null \
           | awk -F', ' '/PrivateKeyEntry/ {print $1; exit}')"
  [ -n "$ALIAS" ] && echo ">> Auto-resolved signing alias: $ALIAS"
fi
[ -n "$ALIAS" ] || { echo "Could not resolve the signing alias — set JSIGN_ALIAS (find it with: pkcs11-tool --module <lib> -O --type cert)."; exit 1; }

JSIGN_ARGS=( --storetype "$STORETYPE" --storepass "$STOREPASS"
             --alias "$ALIAS" --tsaurl "$TSA" --tsmode RFC3161 )
[ "$STORETYPE" = "PKCS11" ] && JSIGN_ARGS+=( --keystore "$JSIGN_KEYSTORE" )

# Find the release workflow run that built the Windows artifacts for this tag.
RUN_ID="${RUN_ID:-$(gh run list --repo "$REPO" --workflow release.yml \
  --json databaseId,headBranch --jq "[.[] | select(.headBranch==\"$TAG\")][0].databaseId")}"
[ -n "$RUN_ID" ] && [ "$RUN_ID" != "null" ] || {
  echo "Could not find a release.yml run for $TAG — pass RUN_ID=<id> (see 'gh run list')."; exit 1; }
echo ">> Using workflow run $RUN_ID"

work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT; cd "$work"

echo ">> Downloading Windows artifacts from run $RUN_ID ..."
gh run download "$RUN_ID" --repo "$REPO" --dir artifacts

# Collect Windows zips (each artifact lands in its own subdirectory). bash 3.2-safe.
zips=()
while IFS= read -r -d '' z; do zips+=("$z"); done \
  < <(find artifacts -type f -iname '*windows*.zip' -print0)
[ "${#zips[@]}" -gt 0 ] || { echo "No Windows artifacts found in run $RUN_ID."; exit 1; }

signed=()
for z in "${zips[@]}"; do
  echo ">> $(basename "$z")"
  d="$(mktemp -d)"
  unzip -q "$z" -d "$d"
  while IFS= read -r -d '' exe; do
    echo "   signing $(basename "$exe")"
    jsign "${JSIGN_ARGS[@]}" "$exe"
  done < <(find "$d" -type f -name '*.exe' -print0)
  out="$work/$(basename "$z")"
  ( cd "$d" && zip -qr "$out" . )
  rm -rf "$d"
  signed+=("$out")
done

echo ">> Uploading signed Windows assets to release $TAG ..."
gh release upload "$TAG" --repo "$REPO" --clobber "${signed[@]}"

# Add the CLI Windows zips to checksums.txt (consistency with the other CLI
# platforms; desktop zips were never listed there).
if gh release download "$TAG" --repo "$REPO" --pattern checksums.txt --dir "$work" 2>/dev/null; then
  add=0
  for f in "${signed[@]}"; do
    b="$(basename "$f")"
    case "$b" in
      kapi_*_windows_*.zip)
        if ! grep -q "  ${b}\$" "$work/checksums.txt"; then
          printf '%s  %s\n' "$(shasum -a 256 "$f" | awk '{print $1}')" "$b" >> "$work/checksums.txt"
          add=1
        fi ;;
    esac
  done
  [ "$add" = 1 ] && gh release upload "$TAG" --repo "$REPO" --clobber "$work/checksums.txt"
fi

echo "✅ Signed Windows artifacts added to release $TAG: ${signed[*]##*/}"
