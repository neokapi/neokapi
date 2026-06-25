#!/usr/bin/env bash
#
# package-cli.sh — archive the prebuilt CLI binaries and write checksums.
#
# Replaces what GoReleaser's `archives` + `checksum` did for the kapi /
# kapi-bowrain CLIs (we build them with cgo + static ICU in a per-OS matrix,
# which GoReleaser OSS can't consume — `builder: prebuilt` is Pro only). The
# matrix uploads each target's binaries as a `cli-bins-<os>-<arch>` artifact;
# this script lays them out into the release archive shapes the brew formulae,
# the docs, and the plugin registry already expect, then emits checksums.txt.
#
# Archive shapes (must stay stable — referenced by formulae/registry/docs).
# Names mirror the Homebrew naming: the CLI toolchain is the kapi-cli / kapi-*
# family, the desktop app is plain "kapi" (see release.yml).
#   kapi-cli_<ver>_<os>_<arch>.tar.gz        -> kapi               (linux/darwin)
#   kapi-bowrain_<ver>_<os>_<arch>.tar.gz    -> bowrain/{kapi-bowrain,manifest.json}
#   kapi-bowrain_<ver>_windows_amd64.zip     -> bowrain/{kapi-bowrain.exe,manifest.json}
#
# Usage: package-cli.sh <version> <bins-dir> <out-dir> <manifest-json>
#   bins-dir   holds the downloaded cli-bins-<os>-<arch>/ subdirectories
#   out-dir    archives + checksums.txt are written here
#   manifest   path to bowrain/cli/cmd/kapi-bowrain/manifest.json
set -euo pipefail

version="${1:?version required}"
bins_dir="${2:?bins dir required}"
out_dir="${3:?out dir required}"
manifest="${4:?manifest.json path required}"

mkdir -p "$out_dir"
# Resolve to an absolute path: the Windows archive is created inside a
# `( cd … && zip … )` subshell, so a relative out_dir (e.g. "dist") would not
# resolve once the CWD changes.
out_dir=$(cd "$out_dir" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi
}

for d in "$bins_dir"/cli-bins-*; do
  [ -d "$d" ] || continue
  base=${d##*/cli-bins-}      # e.g. darwin-arm64
  os=${base%-*}
  arch=${base#*-}

  # --- kapi (linux/darwin only; windows kapi is signed + published out of band) ---
  if [ -f "$d/kapi" ]; then
    stage="$work/kapi-$os-$arch"
    mkdir -p "$stage"
    cp "$d/kapi" "$stage/kapi"
    chmod +x "$stage/kapi"
    tar -czf "$out_dir/kapi-cli_${version}_${os}_${arch}.tar.gz" -C "$stage" kapi
  fi

  # --- kapi-bowrain plugin (bowrain/ dir + manifest.json) ---
  kb_bin=""
  [ -f "$d/kapi-bowrain" ] && kb_bin="$d/kapi-bowrain"
  [ -f "$d/kapi-bowrain.exe" ] && kb_bin="$d/kapi-bowrain.exe"
  if [ -n "$kb_bin" ]; then
    stage="$work/kb-$os-$arch/bowrain"
    mkdir -p "$stage"
    cp "$kb_bin" "$stage/$(basename "$kb_bin")"
    chmod +x "$stage/$(basename "$kb_bin")" 2>/dev/null || true
    cp "$manifest" "$stage/manifest.json"
    if [ "$os" = "windows" ]; then
      ( cd "$work/kb-$os-$arch" && zip -qr "$out_dir/kapi-bowrain_${version}_${os}_${arch}.zip" bowrain )
    else
      tar -czf "$out_dir/kapi-bowrain_${version}_${os}_${arch}.tar.gz" -C "$work/kb-$os-$arch" bowrain
    fi
  fi
done

# checksums.txt over the archives (filenames only, no directory component).
# nullglob so a track that produced no .zip (kapi-only: darwin/linux tarballs,
# no Windows zip) doesn't pass a literal "./*.zip" to sha256.
( cd "$out_dir" && shopt -s nullglob && files=( ./*.tar.gz ./*.zip ) && sha256 "${files[@]}" | sed 's| \./| |' > checksums.txt )
echo "packaged archives:" >&2
ls -l "$out_dir" >&2
