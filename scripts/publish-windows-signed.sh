#!/usr/bin/env bash
#
# Sign the Windows release artifacts with a Certum certificate and add them to
# the (already published) GitHub release. Run on a Mac after the release
# workflow has finished — macOS/Linux assets are published by CI; the Windows
# binaries are produced as workflow *artifacts* and signed here.
#
# Prereqs — unlock the certificate first:
#   • Certum cloud (SimplySign): open SimplySign Desktop, log in (mobile OTP), then
#       export JSIGN_STORETYPE=PKCS11 JSIGN_KEYSTORE=/path/to/pkcs11.cfg
#     where pkcs11.cfg contains:
#       name=simplysign
#       library=/path/to/<simplysign-pkcs11>.dylib
#   • Certum card (CryptoCertum): insert the token, then
#       export JSIGN_STORETYPE=CRYPTOCERTUM
#   • Always:  export JSIGN_STOREPASS=<certificate PIN>
#              export JSIGN_ALIAS="Skissefabrikken AS"   # certificate CN (default)
#
# Usage:  ./scripts/publish-windows-signed.sh v1.2.3
#         RUN_ID=123456789 ./scripts/publish-windows-signed.sh v1.2.3   # pin the run
set -euo pipefail

TAG="${1:?usage: publish-windows-signed.sh <tag>   e.g. v1.2.3}"
REPO="${REPO:-neokapi/neokapi}"

STORETYPE="${JSIGN_STORETYPE:-PKCS11}"
ALIAS="${JSIGN_ALIAS:-Skissefabrikken AS}"
TSA="${JSIGN_TSA:-http://time.certum.pl/}"
: "${JSIGN_STOREPASS:?set JSIGN_STOREPASS to the certificate PIN}"
command -v jsign >/dev/null || { echo "jsign not found — run 'brew bundle'"; exit 1; }
command -v gh    >/dev/null || { echo "gh not found — run 'brew bundle'"; exit 1; }

JSIGN_ARGS=( --storetype "$STORETYPE" --storepass "$JSIGN_STOREPASS"
             --alias "$ALIAS" --tsaurl "$TSA" --tsmode RFC3161 )
if [ "$STORETYPE" = "PKCS11" ]; then
  : "${JSIGN_KEYSTORE:?PKCS11 store needs JSIGN_KEYSTORE=/path/to/pkcs11.cfg}"
  JSIGN_ARGS+=( --keystore "$JSIGN_KEYSTORE" )
fi

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

# Optionally open/refresh the winget-pkgs PR for the signed CLI (opt-in:
# WINGET_SUBMIT=1). Prereqs: the package exists in winget-pkgs (one-time
# `komac new Neokapi.Kapi`) and a WINGET_TOKEN repo secret. See winget.yml.
if [ "${WINGET_SUBMIT:-0}" = "1" ]; then
  echo ">> Triggering winget submission for $TAG ..."
  gh workflow run winget.yml --repo "$REPO" -f tag="$TAG"
fi

echo "✅ Signed Windows artifacts added to release $TAG: ${signed[*]##*/}"
