#!/usr/bin/env bash
# Sign + notarize a macOS (darwin) CLI binary with quill, a cross-platform
# Apple code-signing tool. Driven from a GoReleaser post-build hook so the
# binary is signed *before* it is placed into the release archive.
#
# Usage: quill-sign-darwin.sh <binary-path> <goos> <is-snapshot>
#
# Real releases (is-snapshot=false) do a Developer ID sign + notarization using
# the QUILL_* environment variables (set from CI secrets). Snapshot/local builds
# (is-snapshot=true) fall back to an ad-hoc, offline, no-credentials signature so
# `goreleaser release --snapshot` works without Apple secrets.
#
# Credentials (release builds only), read from the environment by quill:
#   QUILL_SIGN_P12        base64 of the Developer ID Application .p12 (or a path)
#   QUILL_SIGN_PASSWORD   .p12 password
#   QUILL_NOTARY_KEY      base64 of the App Store Connect .p8 key (or a path)
#   QUILL_NOTARY_KEY_ID   App Store Connect key id
#   QUILL_NOTARY_ISSUER   App Store Connect issuer id
#
# Note: quill embeds Apple's intermediate + root certificates and attaches the
# full chain at signing time, so the .p12 need only contain the leaf + key.
set -euo pipefail

bin="${1:?binary path required}"
goos="${2:?goos required}"
snapshot="${3:-false}"

# Only Mach-O (darwin) binaries can be Apple-signed; pass everything else through.
if [ "$goos" != "darwin" ]; then
  exit 0
fi

# GoReleaser passes {{ .Path }} relative to the project root. Resolve it whether
# the hook runs from the root or a build subdirectory.
if [ ! -f "$bin" ] && [ -f "../$bin" ]; then
  bin="../$bin"
fi

echo "quill: sign-and-notarize $bin (snapshot=$snapshot)"
quill sign-and-notarize "$bin" \
  --dry-run="$snapshot" \
  --ad-hoc="$snapshot" \
  -vv
