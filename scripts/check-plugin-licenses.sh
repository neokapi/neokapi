#!/usr/bin/env bash
#
# Guard: every PLUGIN tarball carries the license text of the work inside it.
#
# scripts/check-archive-licenses.sh makes this argument for the CLI archives and
# enforces it there. It never reached the plugin packagers, and all six shipped a
# binary plus a manifest with no license text for as long as they existed —
# while Apache-2.0 §4(a) requires it: "You must give any other recipients of the
# Work or Derivative Works a copy of this License." An SPDX string in a plugin
# manifest, a registry entry or a Homebrew formula is a label; it is not the
# grant, and it is not a copy.
#
# TWO FORMS, and the difference is not cosmetic:
#
#   PACKAGED — kapi-asr, kapi-av and kapi-sourcecode accept a prebuilt binary,
#   so this runs their REAL packaging script over a dummy tree and reads the
#   license out of the tarball that comes back. It tests what actually ships.
#
#   DECLARED — kapi-check, kapi-sat, kapi-vision and kapi-pdfium always compile,
#   and their builds link native SDKs (onnxruntime, tokenizers, pdfium) that no
#   dummy directory can stand in for. This asserts instead that the script
#   stages the license and, where the tar names its members, archives it. That
#   is weaker: it reads the recipe rather than the meal. Say so rather than
#   letting a green check imply more than it checked.
#
# The fix for the weaker half is to give those four a --prebuilt-bin flag, at
# which point they move up to PACKAGED and this comment shrinks.
#
# Usage:
#     ./scripts/check-plugin-licenses.sh
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$repo_root"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# A phrase only the Apache-2.0 text contains, so a placeholder file or the wrong
# license fails rather than passing on file existence alone. The markers are
# matched with a bash substring test, never `printf | grep -q`: under pipefail,
# grep exiting on its first match leaves printf a broken pipe, and the check
# reports a correct license as the wrong one whenever that race is lost.
readonly APACHE_MARKER='Apache License'

# kapi-av bundles LGPL-2.1+ ffmpeg/ffprobe, so its own Apache-2.0 grant is not
# the whole obligation: LGPL-2.1 §1 wants a copy of that license travelling with
# the library too. Checked separately from the Apache sweep because it applies
# to exactly one plugin and for a different reason.
readonly LGPL_MARKER='GNU LESSER GENERAL PUBLIC LICENSE'

rc=0

# ── PACKAGED ─────────────────────────────────────────────────────────────────
# plugin : path inside the tarball that must hold the license text
#
# The paths differ because the layouts do: asr and av archive their stage
# directory (so everything sits under kapi-<name>/), while sourcecode archives
# the stage's contents (flat at the root).
mkdir -p "$work/in" "$work/out"
printf 'dummy\n' > "$work/in/ffmpeg"
printf 'dummy\n' > "$work/in/ffprobe"
printf 'dummy\n' > "$work/in/whisper-cli"
chmod +x "$work/in/ffmpeg" "$work/in/ffprobe" "$work/in/whisper-cli"

package_one() {
  local plugin="$1"
  printf 'dummy\n' > "$work/in/kapi-$plugin"
  chmod +x "$work/in/kapi-$plugin"
  case "$plugin" in
    asr)
      scripts/package-asr-plugin.sh --version 0.0.0-check \
        --whisper-dir "$work/in" --prebuilt-bin "$work/in/kapi-asr" \
        --out-dir "$work/out" >/dev/null 2>&1
      ;;
    av)
      scripts/package-av-plugin.sh --version 0.0.0-check \
        --ffmpeg-dir "$work/in" --prebuilt-bin "$work/in/kapi-av" \
        --out-dir "$work/out" >/dev/null 2>&1
      ;;
    sourcecode)
      scripts/package-sourcecode-plugin.sh --version 0.0.0-check \
        --prebuilt-bin "$work/in/kapi-sourcecode" \
        --out-dir "$work/out" >/dev/null 2>&1
      ;;
  esac
}

for row in "asr:kapi-asr/LICENSE" "av:kapi-av/LICENSE" "sourcecode:LICENSE"; do
  plugin=${row%%:*}
  path=${row#*:}

  if ! package_one "$plugin"; then
    echo "ERROR: package-$plugin-plugin.sh failed on the dummy tree — the guard" >&2
    echo "       cannot verify what it ships. Run it by hand to see why." >&2
    rc=1
    continue
  fi

  archive=$(find "$work/out" -name "kapi-${plugin}_0.0.0-check_*.tar.gz" | head -n1)
  if [ -z "$archive" ]; then
    echo "ERROR: package-$plugin-plugin.sh produced no tarball" >&2
    rc=1
    continue
  fi

  text=$(tar -xOf "$archive" "$path" 2>/dev/null || true)
  if [ -z "$text" ]; then
    echo "ERROR: $(basename "$archive") ships no license text at $path" >&2
    echo "       Stage the repo LICENSE in package-$plugin-plugin.sh (and name it" >&2
    echo "       in the tar member list if that script lists its members)." >&2
    rc=1
  elif [[ $text != *"$APACHE_MARKER"* ]]; then
    echo "ERROR: $(basename "$archive"): $path is not Apache-2.0 text" >&2
    rc=1
  fi

  # The copyleft half, for the one plugin that has one.
  if [ "$plugin" = av ]; then
    lgpl=$(tar -xOf "$archive" "kapi-av/COPYING.LGPLv2.1" 2>/dev/null || true)
    if [ -z "$lgpl" ]; then
      echo "ERROR: $(basename "$archive") bundles LGPL ffmpeg but ships no LGPL text" >&2
      echo "       at kapi-av/COPYING.LGPLv2.1. See plugins/av/licenses/README.md." >&2
      rc=1
    elif [[ $lgpl != *"$LGPL_MARKER"* ]]; then
      echo "ERROR: $(basename "$archive"): kapi-av/COPYING.LGPLv2.1 is not LGPL text" >&2
      rc=1
    fi
  fi
done

# ── DECLARED ─────────────────────────────────────────────────────────────────
# These four always compile against a native SDK, so the guard reads the script.
# A packager that stages the license but forgets to name it in an explicit tar
# member list ships nothing — that is the failure this half is shaped to catch,
# because it is the one a reader of the diff would miss.
for plugin in check sat vision pdfium; do
  script="scripts/package-$plugin-plugin.sh"

  if ! grep -qE 'cp "\$(REPO_ROOT|repo)/LICENSE"' "$script"; then
    echo "ERROR: $script does not stage the repo LICENSE" >&2
    rc=1
    continue
  fi

  # Only a tar call that NAMES its members can omit the license by accident;
  # one that archives the whole stage picks it up from the copy above.
  tar_line=$(grep -E '^tar .*-C "\$STAGE" "\$BIN_NAME"' "$script" || true)
  if [ -n "$tar_line" ] && ! printf '%s' "$tar_line" | grep -q 'LICENSE'; then
    echo "ERROR: $script stages the LICENSE but its tar member list omits it," >&2
    echo "       so the file is staged and then not archived:" >&2
    echo "       $tar_line" >&2
    rc=1
  fi
done

[ $rc -eq 0 ] || exit 1
echo "check-plugin-licenses: every plugin tarball ships its license text"
echo "  kapi-av also ships the LGPL-2.1 text for its bundled ffmpeg"
echo "  packaged and read: asr, av, sourcecode"
echo "  declared only (native SDK build cannot be faked): check, sat, vision, pdfium"
