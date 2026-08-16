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
#   kapi-cli_<ver>_<os>_<arch>.tar.gz        -> kapi, LICENSE      (linux/darwin)
#   kapi-bowrain_<ver>_<os>_<arch>.tar.gz    -> bowrain/{kapi-bowrain,manifest.json,LICENSE}
#   kapi-bowrain_<ver>_windows_amd64.zip     -> bowrain/{kapi-bowrain.exe,manifest.json,LICENSE}
#
# Every archive carries the license text of the work inside it. Both licenses
# this repository uses require it — Apache-2.0 §4(a) ("You must give any other
# recipients of the Work or Derivative Works a copy of this License") and
# AGPL-3.0 §4 ("give all recipients a copy of this License along with the
# Program") — and the SPDX string in a manifest or a formula is a label, not the
# grant. `scripts/check-archive-licenses.sh` packages a dummy tree and asserts
# it, so an archive family added without license text fails rather than ships.
#
# The plugin manifest inside the archive is stamped with the release version.
# The committed manifest carries a dev default — the same one the linker
# replaces in the binary — and the manifest is what `kapi plugins list` prints
# and what a registry comparison reads, so an unstamped copy tells every user
# they hold a version older than everything.
#
# Usage: package-cli.sh <version> <bins-dir> <out-dir> <manifest-json>
#   bins-dir   holds the downloaded cli-bins-<os>-<arch>/ subdirectories
#   out-dir    archives + checksums.txt are written here
#   manifest   path to bowrain/plugin/cmd/kapi-bowrain/manifest.json
#
#        package-cli.sh --self-test
#   packages a dummy tree through this script and asserts what the archives
#   declare about themselves. This script runs only on a release tag, so
#   nothing before a release exercises it otherwise.
set -euo pipefail

# ── self-test ────────────────────────────────────────────────────────────────
#
# Runs the real packager as a subprocess and reads the archives it produced, so
# what is asserted is what a user would untar. The license contract over the
# same archives is `scripts/check-archive-licenses.sh`; this covers what the
# archive says it is.
self_test() {
  local root work bins out manifest status=0 staged
  root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
  manifest="$root/bowrain/plugin/cmd/kapi-bowrain/manifest.json"
  work=$(mktemp -d)
  # shellcheck disable=SC2064  # expand $work now, not at trap time
  trap "rm -rf '$work'" RETURN

  bins="$work/bins/cli-bins-linux-amd64"
  out="$work/out"
  mkdir -p "$bins"
  printf 'dummy\n' >"$bins/kapi"
  printf 'dummy\n' >"$bins/kapi-bowrain"

  bash "${BASH_SOURCE[0]}" 9.9.9-selftest "$work/bins" "$out" "$manifest" >/dev/null 2>&1

  local archive="$out/kapi-bowrain_9.9.9-selftest_linux_amd64.tar.gz"
  if [ ! -f "$archive" ]; then
    echo "✖ self-test: the packager produced no $(basename "$archive")"
    return 1
  fi
  echo "✓ self-test: the archive filename carries the release version"

  staged=$(tar -xOf "$archive" bowrain/manifest.json)
  if printf '%s' "$staged" | grep -q '"version": "9.9.9-selftest"'; then
    echo "✓ self-test: the staged manifest declares the release version"
  else
    echo "✖ self-test: the staged manifest does not declare the release version:"
    printf '%s\n' "$staged" | grep '"version"' | sed 's/^/    /'
    status=1
  fi

  if printf '%s' "$staged" | grep -q '0\.0\.0-dev'; then
    echo "✖ self-test: the staged manifest still carries the dev default"
    status=1
  else
    echo "✓ self-test: the dev default is gone from the staged manifest"
  fi

  # The stamp is a rewrite of the staged copy: the committed manifest is a
  # source file a release must not edit, and the rest of it must survive.
  if grep -q '"version": "0.0.0-dev"' "$manifest"; then
    echo "✓ self-test: the committed manifest is untouched"
  else
    echo "✖ self-test: the packager rewrote the committed manifest"
    status=1
  fi
  if printf '%s' "$staged" | grep -q '"plugin": "bowrain"'; then
    echo "✓ self-test: the rest of the manifest survives the stamp"
  else
    echo "✖ self-test: the stamp did not preserve the manifest's other fields"
    status=1
  fi

  return "$status"
}

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit
fi

version="${1:?version required}"
bins_dir="${2:?bins dir required}"
out_dir="${3:?out dir required}"
manifest="${4:?manifest.json path required}"

# The license text each track ships, resolved from this script's location so the
# packager works from any CWD.
repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
kapi_license="$repo_root/LICENSE"
# The plugin subtree carries its own LICENSE: kapi-bowrain is built from
# bowrain/plugin and links nothing else under bowrain/, so it ships Apache-2.0
# rather than the AGPL-3.0 of the server beside it. `make
# check-module-boundaries` asserts that linkage, so if this ever becomes the
# wrong text the build says so before a release does.
plugin_license="$repo_root/bowrain/plugin/LICENSE"
for f in "$kapi_license" "$plugin_license"; do
  [ -f "$f" ] || { echo "package-cli.sh: missing $f" >&2; exit 1; }
done

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
    cp "$kapi_license" "$stage/LICENSE"
    tar -czf "$out_dir/kapi-cli_${version}_${os}_${arch}.tar.gz" -C "$stage" kapi LICENSE
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
    # Stamp the release version into the staged manifest so the published
    # plugin reports the version it was cut at. The committed manifest carries
    # a dev default (matching the `pluginVersion` the linker stamps into the
    # binary), and the manifest is what `kapi plugins list` and any registry
    # comparison read. Only the top-level "version" (2-space indent) is
    # rewritten, never a nested one — the same substitution the per-plugin
    # packagers make.
    sed 's/^  "version": *"[^"]*"/  "version": "'"$version"'"/' "$manifest" > "$stage/manifest.json"
    cp "$plugin_license" "$stage/LICENSE"
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
