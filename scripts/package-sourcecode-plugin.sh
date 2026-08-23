#!/usr/bin/env bash
# Build + package one platform's kapi-sourcecode plugin tarball for distribution
# via the neokapi/registry (installable with `kapi plugins install sourcecode`).
#
# kapi-sourcecode is a cgo binary — the tree-sitter grammars are C — but unlike
# every other cgo plugin here it bundles NOTHING beside itself. The grammars are
# vendored source compiled into the binary by the Go build, so there is no shared
# library to place, no rpath to bake, and no runtime lookup to configure. That is
# also why it must be built on a native runner per platform rather than
# cross-compiled.
#
# Tarball layout (kapi-sourcecode_<version>_<os>_<arch>.tar.gz):
#
#     kapi-sourcecode[.exe]                the plugin binary, at the root
#     manifest.json                        the plugin manifest, at the root
#     formats/sourcecode/schema.json       the format's config schema
#     LICENSE                              Apache-2.0, the license of the work
#
# The LICENSE is not decoration. Apache-2.0 §4(a): "You must give any other
# recipients of the Work or Derivative Works a copy of this License." An SPDX
# string in the manifest is a label, not the grant — the same argument
# scripts/check-archive-licenses.sh makes for the CLI archives.
#
# Usage (flags or env; flags win):
#
#     scripts/package-sourcecode-plugin.sh --version 0.1.0 --out-dir dist/
#
# Echoes the tarball path and sha256, and appends them to $GITHUB_OUTPUT (as
# `tarball`, `tarball_name` and `sha256`) when that is set. The release workflow
# needs the bare NAME as well as the path: its registry job runs on a different
# runner, where the build runner's path means nothing.
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

VERSION="${VERSION:-0.0.0-dev}"
OUT_DIR="${OUT_DIR:-$repo_root/dist}"
GOOS_IN="${GOOS:-$(go env GOOS)}"
GOARCH_IN="${GOARCH:-$(go env GOARCH)}"
PREBUILT_BIN="${PREBUILT_BIN:-}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version)      VERSION="$2"; shift 2 ;;
    --out-dir)      OUT_DIR="$2"; shift 2 ;;
    # A macOS release leg signs the binary before packaging, so it hands the
    # already-signed artifact in rather than having this script rebuild it.
    --prebuilt-bin) PREBUILT_BIN="$2"; shift 2 ;;
    *) echo "package-sourcecode-plugin: unknown flag: $1" >&2; exit 2 ;;
  esac
done

BIN_NAME="kapi-sourcecode"
[ "$GOOS_IN" = "windows" ] && BIN_NAME="kapi-sourcecode.exe"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
mkdir -p "$OUT_DIR" "$STAGE/formats/sourcecode"

if [ -n "$PREBUILT_BIN" ]; then
  cp "$PREBUILT_BIN" "$STAGE/$BIN_NAME"
else
  # GOWORK=off: the plugin modules sit outside go.work on purpose, and a
  # workspace build would resolve the framework from the workspace rather than
  # through the module's own replace directive.
  ( cd "$repo_root/plugins/sourcecode" && GOWORK=off CGO_ENABLED=1 \
      GOOS="$GOOS_IN" GOARCH="$GOARCH_IN" \
      go build -trimpath -ldflags "-s -w -X github.com/neokapi/neokapi/core/version.Version=$VERSION" \
        -o "$STAGE/$BIN_NAME" ./cmd/kapi-sourcecode )
fi

cp "$repo_root/plugins/sourcecode/manifest.json" "$STAGE/manifest.json"
cp "$repo_root/plugins/sourcecode/formats/sourcecode/schema.json" "$STAGE/formats/sourcecode/schema.json"
cp "$repo_root/LICENSE" "$STAGE/LICENSE"

TARBALL="kapi-sourcecode_${VERSION}_${GOOS_IN}_${GOARCH_IN}.tar.gz"
TARBALL_PATH="$OUT_DIR/$TARBALL"
tar -czf "$TARBALL_PATH" -C "$STAGE" "$BIN_NAME" manifest.json formats LICENSE

if command -v sha256sum >/dev/null 2>&1; then
  SHA256="$(sha256sum "$TARBALL_PATH" | awk '{print $1}')"
else
  SHA256="$(shasum -a 256 "$TARBALL_PATH" | awk '{print $1}')"
fi
echo "$SHA256  $TARBALL" > "$TARBALL_PATH.sha256"

echo "package-sourcecode-plugin: $TARBALL_PATH"
echo "package-sourcecode-plugin: sha256 $SHA256"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  {
    echo "tarball=$TARBALL_PATH"
    echo "tarball_name=$TARBALL"
    echo "sha256=$SHA256"
  } >> "$GITHUB_OUTPUT"
fi
