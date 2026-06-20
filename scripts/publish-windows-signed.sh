#!/usr/bin/env bash
#
# Sign the Windows release artifacts with a Certum certificate and add them to
# the (already published) GitHub release. Run on a Mac after the release
# workflow has finished — macOS/Linux assets are published by CI; the Windows
# binaries are produced as workflow *artifacts* and signed here.
#
# For each artifact the signed .exe is re-zipped (portable download). The two
# desktop GUI apps (Bowrain, Kapi) additionally get a signed NSIS installer
# (setup.exe) built here with makensis from the signed .exe — both the inner
# app .exe and the installer are signed, mirroring the macOS app+DMG signing.
# The kapi CLI stays zip-only (installed via winget/scoop).
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

# Repo root (captured before we cd into the work dir) — the NSIS templates and
# icons for the desktop installers live in the checkout.
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# vX.Y.Z[-suffix] → X.Y.Z for the installer's numeric VIProductVersion.
VER="${TAG#v}"
VER_NUM="$(printf '%s' "$VER" | grep -oE '^[0-9]+\.[0-9]+\.[0-9]+')"
[ -n "$VER_NUM" ] || { echo "Could not derive a numeric X.Y.Z version from tag '$TAG'."; exit 1; }

STORETYPE="${JSIGN_STORETYPE:-PKCS11}"
TSA="${JSIGN_TSA:-http://time.certum.pl/}"
# SimplySign cloud has no card PIN and no per-signature prompt — the active
# SimplySign Desktop session is the authorization — so the store password is
# empty by default. (For a PIN-protected token or physical card, set JSIGN_STOREPASS.)
STOREPASS="${JSIGN_STOREPASS:-}"
command -v jsign    >/dev/null || { echo "jsign not found — run 'brew bundle'"; exit 1; }
command -v gh       >/dev/null || { echo "gh not found — run 'brew bundle'"; exit 1; }
# Desktop installers (Bowrain, Kapi) are built from the signed .exe with NSIS.
command -v makensis >/dev/null || { echo "makensis not found — run 'brew bundle'"; exit 1; }
command -v wails3   >/dev/null || { echo "wails3 not found — go install github.com/wailsapp/wails/v3/cmd/wails3@latest"; exit 1; }

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

# Build an NSIS installer (setup.exe) for a Wails desktop app from its
# already-signed .exe. The installer embeds the Microsoft WebView2 bootstrapper,
# so the app launches on machines without the runtime — something a bare .exe in
# a .zip cannot do — and registers a Start-menu entry, Add/Remove-Programs entry
# and uninstaller. App metadata is injected via -D defines (the committed
# wails_tools.nsh only carries placeholder defaults).
#   $1 app dir (relative to repo root)   $2 product/project name
#   $3 signed .exe   $4 arch (amd64|arm64)   $5 output installer path
build_nsis_installer() {
  local app_dir="$1" name="$2" exe="$3" arch="$4" out="$5"
  local nsis_src="$REPO_ROOT/$app_dir/build/windows/nsis"
  local icon="$REPO_ROOT/$app_dir/build/windows/icon.ico"
  [ -f "$nsis_src/project.nsi" ] || { echo "missing $nsis_src/project.nsi"; exit 1; }

  # Replicate the build/windows{,/nsis} + bin layout that the template's relative
  # OutFile ("..\..\..\bin") and MUI_ICON ("..\icon.ico") paths resolve against.
  local b; b="$(mktemp -d)"
  mkdir -p "$b/build/windows/nsis" "$b/bin"
  cp "$icon" "$b/build/windows/icon.ico"
  cp "$nsis_src/project.nsi" "$nsis_src/wails_tools.nsh" "$b/build/windows/nsis/"
  # Fetch the WebView2 bootstrapper the installer embeds (wails.webview2runtime).
  wails3 generate webview2bootstrapper -dir "$b/build/windows/nsis" >/dev/null

  local flag=AMD64; [ "$arch" = "arm64" ] && flag=ARM64
  ( cd "$b/build/windows/nsis" && makensis -V2 \
      -DARG_WAILS_${flag}_BINARY="$exe" \
      -DINFO_PROJECTNAME="$name" \
      -DINFO_PRODUCTNAME="$name" \
      -DINFO_COMPANYNAME="neokapi" \
      -DINFO_PRODUCTVERSION="$VER_NUM" \
      -DINFO_COPYRIGHT="© $(date +%Y) neokapi" \
      project.nsi )
  mv "$b/bin/${name}-${arch}-installer.exe" "$out"
  rm -rf "$b"
}

# Find the release workflow run that built the Windows artifacts for this tag.
RUN_ID="${RUN_ID:-$(gh run list --repo "$REPO" --workflow release.yml \
  --json databaseId,headBranch --jq "[.[] | select(.headBranch==\"$TAG\")][0].databaseId")}"
[ -n "$RUN_ID" ] && [ "$RUN_ID" != "null" ] || {
  echo "Could not find a release.yml run for $TAG — pass RUN_ID=<id> (see 'gh run list')."; exit 1; }
echo ">> Using workflow run $RUN_ID"

work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT; cd "$work"

echo ">> Downloading Windows artifacts from run $RUN_ID ..."
# Filter to the Windows artifacts only — the run also carries Docker buildx
# "*.dockerbuild" build-record artifacts that `gh run download` cannot unzip
# ("not a valid zip file"), which would abort the whole download under set -e.
gh run download "$RUN_ID" --repo "$REPO" --dir artifacts --pattern '*windows*'

# Collect Windows zips (each artifact lands in its own subdirectory). bash 3.2-safe.
zips=()
while IFS= read -r -d '' z; do zips+=("$z"); done \
  < <(find artifacts -type f -iname '*windows*.zip' -print0)
[ "${#zips[@]}" -gt 0 ] || { echo "No Windows artifacts found in run $RUN_ID."; exit 1; }

signed=()
for z in "${zips[@]}"; do
  bn="$(basename "$z")"
  echo ">> $bn"
  d="$(mktemp -d)"
  unzip -q "$z" -d "$d"
  while IFS= read -r -d '' exe; do
    echo "   signing $(basename "$exe")"
    jsign "${JSIGN_ARGS[@]}" "$exe"
  done < <(find "$d" -type f -name '*.exe' -print0)

  # Portable zip: re-zip the signed .exe under its original artifact name.
  out="$work/$bn"
  ( cd "$d" && zip -qr "$out" . )
  signed+=("$out")

  # Desktop GUI apps also ship a signed NSIS installer built from the signed
  # .exe. The CLI (kapi-cli_*) stays zip-only — CLIs install via winget/scoop.
  # Desktop zips are hyphen-delimited (kapi-<ver>-windows / bowrain-<ver>-windows);
  # the kapi-[0-9]* glob matches the desktop app but not kapi-cli_*/kapi-bowrain_*.
  app_dir=""; app_name=""
  case "$bn" in
    bowrain-[0-9]*windows*) app_dir="bowrain/apps/bowrain"; app_name="Bowrain" ;;
    kapi-[0-9]*windows*)    app_dir="apps/kapi-desktop";    app_name="Kapi" ;;
  esac
  if [ -n "$app_dir" ]; then
    arch=amd64; case "$bn" in *arm64*) arch=arm64 ;; esac
    setup="$work/${bn%-windows-*}-windows-${arch}-setup.exe"
    echo "   building installer ${setup##*/}"
    build_nsis_installer "$app_dir" "$app_name" "$d/${app_name}.exe" "$arch" "$setup"
    echo "   signing $(basename "$setup")"
    jsign "${JSIGN_ARGS[@]}" "$setup"
    signed+=("$setup")
  fi
  rm -rf "$d"
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
      kapi-cli_*_windows_*.zip)
        if ! grep -q "  ${b}\$" "$work/checksums.txt"; then
          printf '%s  %s\n' "$(shasum -a 256 "$f" | awk '{print $1}')" "$b" >> "$work/checksums.txt"
          add=1
        fi ;;
    esac
  done
  [ "$add" = 1 ] && gh release upload "$TAG" --repo "$REPO" --clobber "$work/checksums.txt"
fi

echo "✅ Signed Windows artifacts added to release $TAG:"
for f in "${signed[@]}"; do echo "   • ${f##*/}"; done

# Now that the signed CLI zip + desktop setup.exe are on the release, kick off
# the winget update workflow. winget.yml bumps both Neokapi.KapiCli (portable
# zip) and Neokapi.Kapi (desktop setup.exe); the desktop row fails harmlessly
# until that package is bootstrapped once with `komac new Neokapi.Kapi`.
# Set SKIP_WINGET=1 to skip (e.g. a re-run that only fixes the signed assets).
if [ "${SKIP_WINGET:-0}" = "1" ]; then
  echo ">> SKIP_WINGET=1 — not dispatching winget. Run later: gh workflow run winget.yml -f tag=$TAG"
elif ! command -v gh >/dev/null 2>&1; then
  echo ">> gh not found — skipping winget dispatch. Run later: gh workflow run winget.yml -f tag=$TAG" >&2
else
  echo ">> Dispatching winget publish (winget.yml) for $TAG ..."
  if gh workflow run winget.yml --repo "$REPO" -f tag="$TAG"; then
    echo "   winget.yml dispatched — watch: gh run list --workflow=winget.yml --repo $REPO"
  else
    echo "   ⚠ winget dispatch failed; the signed assets are still published. Retry: gh workflow run winget.yml -f tag=$TAG" >&2
  fi
  # Also generate the Windows in-app-update feed from the freshly-signed desktop
  # zips (the Wails native updater swaps the signed .exe). Same SKIP_WINGET guard.
  echo ">> Dispatching Windows update-feed publish (appcast-windows.yml) for $TAG ..."
  if gh workflow run appcast-windows.yml --repo "$REPO" -f tag="$TAG"; then
    echo "   appcast-windows.yml dispatched — watch: gh run list --workflow=appcast-windows.yml --repo $REPO"
  else
    echo "   ⚠ appcast-windows dispatch failed. Retry: gh workflow run appcast-windows.yml -f tag=$TAG" >&2
  fi
fi
