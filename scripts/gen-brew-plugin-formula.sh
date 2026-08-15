#!/usr/bin/env bash
#
# gen-brew-plugin-formula.sh — emit the Homebrew formula for one kapi plugin.
#
# The plugin release workflows (release-asr.yml, release-av.yml,
# release-pdfium.yml, release-vision.yml) already publish tarballs, sign them
# and register them in the plugin registry. The tap formula was the one consumer
# left to a hand edit, and it drifted: `kapi-asr` sat at 0.1.0 while asr-v0.1.1
# had shipped, and `kapi-vision` at 0.2.0 against vision-v0.2.2 — so `brew
# install neokapi/tap/kapi-asr` served a superseded release for two months while
# the registry served the current one. This script is the per-plugin counterpart
# of gen-brew-formula.sh (which does the same for the kapi and bowrain CLIs), so
# the formula is rendered from the release's own checksums rather than
# transcribed.
#
# Every string a plugin formula carries — description, install layout,
# self-check invocation, test assertion — lives in the plugin table below. It is
# the single place a plugin's Homebrew presentation is edited.
#
# Homebrew covers macOS arm64 and Linux amd64/arm64. All four plugin build
# matrices produce exactly those three (plus Windows legs, which brew never
# sees), so a missing checksum for any of the three is a broken build rather
# than an unsupported platform — it is refused, never quietly omitted. A formula
# whose on_macos block came out empty installs nothing and reports success.
#
# Usage:
#     gen-brew-plugin-formula.sh <plugin> <version> <repo> <checksums-dir> <out-dir> [tag]
#     gen-brew-plugin-formula.sh --self-test    # prove the generator both ways
#
#   plugin         asr | av | pdfium | vision
#   repo           e.g. neokapi/neokapi
#   checksums-dir  directory of *.sha256 files, each "<sha256>  <tarball name>"
#                  (the per-leg checksum artifacts the release workflows upload)
#   out-dir        the formula is written to <out-dir>/kapi-<plugin>.rb
#   tag            release tag the tarballs are attached to; defaults to
#                  "<plugin>-v<version>"
#
# Wired into .github/workflows/_publish-plugin-formula.yml, and self-tested in
# the repo-guards job of ci.yml.
#
# Failures return rather than exit, and every call site tests the return: a
# caller that wraps this in `if`, `||` or a pipeline suspends `set -e` for
# everything underneath it, so `set -e` alone would let a missing checksum
# render a formula with a hole in it. The self-test is such a caller, and that
# is how it caught the first draft.
set -euo pipefail

usage() {
  sed -n '/^# Usage:/,/^#     .*--self-test/p' "$0" | sed 's/^# \{0,1\}//'
}

# ── the plugin table ─────────────────────────────────────────────────────────
# Sets, for the named plugin: the Ruby class, the `desc` line, the self-check
# arguments (as a Ruby argument tail and as a shell argument tail — the same
# invocation, written for `system` and for `shell_output`), and the three prose
# lines that differ per plugin: what the tarball contains, what the first real
# use would otherwise stall on, and what the self-check proves. The plugin id is
# also the directory under share/kapi/plugins and the plugins/<id> homepage path.
plugin_meta() {
  case "$1" in
    asr)
      class="KapiAsr"
      desc="Speech-recognition plugin for kapi — transcribe audio/video via whisper.cpp"
      ruby_args=', "asr"'
      shell_args=" asr"
      layout="kapi-asr binary + manifest.json + NOTICE + bundled whisper-cli (+ its shared libs) and a default ggml-*.bin model"
      optin="ASR is opt-in"
      firstuse="kapi's first transcription"
      testnote="The self-check prints the resolved whisper-cli + model paths and exits 0; this also exercises that the bundled whisper-cli resolves via @loader_path."
      ;;
    av)
      class="KapiAv"
      desc="Video-demux dependency plugin for kapi — bundles LGPL ffmpeg/ffprobe"
      ruby_args=', "av"'
      shell_args=" av"
      layout="kapi-av binary + manifest.json + NOTICE + bundled LGPL ffmpeg and ffprobe"
      optin="video is opt-in"
      firstuse="kapi's first video demux"
      testnote="The self-check prints the resolved ffmpeg/ffprobe paths and exits 0."
      ;;
    pdfium)
      class="KapiPdfium"
      desc="PDFium-backed PDF reader plugin for kapi (correct CID/CJK text + geometry)"
      ruby_args=""
      shell_args=""
      layout="kapi-pdfium binary + manifest.json + lib/<bundled libpdfium>"
      optin="kapi-cli depends_on this one, so the reverse edge would cycle"
      firstuse="kapi's first PDF read"
      testnote="Bare invocation prints the self-check line and exits 0; this also exercises that the bundled libpdfium resolves via the binary's rpath."
      ;;
    vision)
      class="KapiVision"
      desc="Document-vision plugin for kapi — PP-OCRv5 OCR + PP-DocLayoutV3 layout"
      ruby_args=', "command", "vision"'
      shell_args=" command vision"
      layout="kapi-vision binary + manifest.json + lib/<bundled onnxruntime> + models/<PP-OCRv5 assets>"
      optin="OCR is opt-in, not bundled with the CLI"
      firstuse="kapi's first vision dispatch"
      testnote="The self-check constructs the engine (loading the bundled onnxruntime via the binary's rpath) and lists the model assets, then exits 0."
      ;;
    *)
      echo "gen-brew-plugin-formula.sh: unknown plugin '$1' (want asr|av|pdfium|vision)" >&2
      return 1
      ;;
  esac
}

# Emit "$*" as a Ruby comment block, indented $indent spaces and wrapped so no
# line exceeds Homebrew's rubocop line length. The per-plugin prose in the table
# above is written as one sentence per field; this is what turns it into a
# comment a `brew style` run does not object to.
comment() {
  local indent="$1"; shift
  printf '%s\n' "$*" | fold -s -w $((78 - indent)) | sed -e 's/[[:space:]]*$//' \
    -e "s/^/$(printf "%${indent}s")# /"
}

# sha256 for a release filename, looked up across every *.sha256 in the
# checksums directory. Refuses an absent or ambiguous entry.
sha_for() {
  local file="$1" matches count
  matches=$(awk -v f="$file" '$2==f {print $1}' "$checksums_dir"/*.sha256 2>/dev/null || true)
  count=$(printf '%s\n' "$matches" | grep -c . || true)
  case "$count" in
    1) printf '%s' "$matches" ;;
    0)
      echo "gen-brew-plugin-formula.sh: no checksum for ${file} in ${checksums_dir}" >&2
      return 1
      ;;
    *)
      echo "gen-brew-plugin-formula.sh: conflicting checksums for ${file} in ${checksums_dir}" >&2
      return 1
      ;;
  esac
}

# Emit the on_macos/on_linux url+sha256 blocks, indented two spaces.
platform_block() {
  local base_url="$1" da la li
  da=$(sha_for "kapi-${plugin}_${version}_darwin_arm64.tar.gz") || return 1
  la=$(sha_for "kapi-${plugin}_${version}_linux_arm64.tar.gz") || return 1
  li=$(sha_for "kapi-${plugin}_${version}_linux_amd64.tar.gz") || return 1
  cat <<RUBY
  on_macos do
    on_arm do
      url "${base_url}/kapi-${plugin}_${version}_darwin_arm64.tar.gz"
      sha256 "${da}"
    end
  end

  on_linux do
    on_arm do
      url "${base_url}/kapi-${plugin}_${version}_linux_arm64.tar.gz"
      sha256 "${la}"
    end
    on_intel do
      url "${base_url}/kapi-${plugin}_${version}_linux_amd64.tar.gz"
      sha256 "${li}"
    end
  end
RUBY
}

generate() {
  plugin="${1:?plugin required}"
  version="${2:?version required}"
  local repo="${3:?repo (owner/name) required}"
  checksums_dir="${4:?checksums dir required}"
  local out="${5:?out dir required}"
  local tag="${6:-${plugin}-v${version}}"

  local class desc ruby_args shell_args layout optin firstuse testnote
  plugin_meta "$plugin" || return 1

  if [ ! -d "$checksums_dir" ]; then
    echo "gen-brew-plugin-formula.sh: no such checksums dir: ${checksums_dir}" >&2
    return 1
  fi

  local bin="kapi-${plugin}"
  local path="share/\"kapi/plugins/${plugin}"
  local file="${out}/kapi-${plugin}.rb"

  # Rendered to a temporary file and moved into place only once every checksum
  # resolved, so a refused render leaves no half-written formula for a caller to
  # commit.
  mkdir -p "$out"
  local tmp
  tmp=$(mktemp "${out}/.kapi-${plugin}.rb.XXXXXX")

  {
    echo "class ${class} < Formula"
    # Kept under 80 characters — `brew audit --strict` rejects longer ones.
    echo "  desc \"${desc}\""
    echo "  homepage \"https://github.com/${repo}/tree/main/plugins/${plugin}\""
    # Declared explicitly, never inferred: Homebrew's Version.detect on a URL
    # ending in `_darwin_arm64.tar.gz` returns "64" (it latches onto `arm64`).
    echo "  version \"${version}\""
    echo '  license "Apache-2.0"'
    echo
    platform_block "https://github.com/${repo}/releases/download/${tag}" || exit 1
    echo
    comment 2 "Plugin layout: ${layout}." \
      "Install the whole self-contained tree under the keg's own" \
      "share/kapi/plugins/${plugin}; Homebrew then links it to" \
      "HOMEBREW_PREFIX/share/kapi/plugins/${plugin}, the shared kapi plugins" \
      "root \`kapi\` discovers. Installing into the keg (rather than writing" \
      "HOMEBREW_PREFIX directly, which the install sandbox denies with EPERM" \
      "because that path belongs to another formula) keeps the install" \
      "sandbox-safe and lets \`brew uninstall\` clean up." \
      "No depends_on kapi-cli — ${optin}."
    cat <<RUBY
  def install
    (${path}").install Dir["*"]
  end

RUBY
    comment 2 "Absorb macOS Gatekeeper's one-time first-exec assessment of the" \
      "plugin binary at install time instead of stalling ${firstuse}." \
      "Best-effort: a failure just means the first real exec pays it instead."
    cat <<RUBY
  def post_install
    system ${path}/${bin}"${ruby_args}
  rescue
    nil
  end

  test do
RUBY
    comment 4 "$testnote"
    cat <<RUBY
    assert_match "${bin}",
      shell_output("#{share}/kapi/plugins/${plugin}/${bin}${shell_args} 2>&1")
  end
end
RUBY
  } > "$tmp" || { rm -f "$tmp"; return 1; }

  mv "$tmp" "$file"
  echo "wrote ${file} (plugin=${plugin}, version=${version}, tag=${tag})" >&2
}

# ── self-test ────────────────────────────────────────────────────────────────
# Proves the generator both ways: a complete checksum set renders a formula that
# names every platform artifact and the plugin's own share path, and an
# incomplete one is refused rather than rendered with a hole in it.

# Write a checksums directory for $1 (plugin) at $2 (version) into $3, omitting
# the legs named in $4 (space-separated <goos>_<goarch>).
seed_checksums() {
  local plugin="$1" version="$2" dir="$3" omit="${4:-}" leg
  mkdir -p "$dir"
  for leg in darwin_arm64 linux_arm64 linux_amd64 windows_amd64; do
    case " $omit " in *" $leg "*) continue ;; esac
    # A deterministic stand-in digest: the generator only ever copies it.
    printf '%s  kapi-%s_%s_%s.tar.gz\n' \
      "$(printf '%s' "${plugin}${leg}" | shasum -a 256 | awk '{print $1}')" \
      "$plugin" "$version" "$leg" > "${dir}/${leg}.sha256"
  done
}

fail() { echo "✖ self-test: $*" >&2; exit 1; }

self_test() {
  local work
  work=$(mktemp -d)
  # shellcheck disable=SC2064  # expand $work now; the trap outlives the local
  trap "rm -rf '$work'" EXIT

  # 1. A complete set renders every plugin, and each formula names its own
  #    artifacts, version and share path.
  local p leg f
  for p in asr av pdfium vision; do
    seed_checksums "$p" 9.9.9 "${work}/sums-${p}"
    ( generate "$p" 9.9.9 neokapi/neokapi "${work}/sums-${p}" "${work}/out" ) 2>/dev/null \
      || fail "${p}: a complete checksum set was refused"

    f="${work}/out/kapi-${p}.rb"
    [ -f "$f" ] || fail "${p}: no formula written"
    grep -q "^  version \"9.9.9\"\$" "$f" || fail "${p}: the version is not declared"
    for leg in darwin_arm64 linux_arm64 linux_amd64; do
      grep -q "kapi-${p}_9.9.9_${leg}.tar.gz" "$f" \
        || fail "${p}: ${leg} is missing from the formula"
    done
    # The three brew platforms and only those: a windows leg in the checksums
    # must not reach a formula.
    ! grep -q windows "$f" || fail "${p}: a windows artifact reached the formula"
    grep -q "kapi/plugins/${p}" "$f" || fail "${p}: the shared plugins path is wrong"
    [ "$(grep -c '      sha256 "' "$f")" = 3 ] || fail "${p}: expected three sha256 lines"
    # Balanced Ruby: every do/def opens a block the file closes.
    [ "$(grep -cE '^\s*(class|def|do$|.* do$)' "$f")" = "$(grep -cE '^\s*end$' "$f")" ] \
      || fail "${p}: the formula's blocks do not balance"
  done
  echo "✓ self-test: every plugin renders from its release checksums"

  # 2. A hole in the checksums is refused, not papered over. macOS is the leg
  #    that would otherwise emit an empty on_macos block.
  seed_checksums vision 9.9.9 "${work}/sums-hole" darwin_arm64
  local rc=0
  ( generate vision 9.9.9 neokapi/neokapi "${work}/sums-hole" "${work}/out-hole" ) 2>/dev/null || rc=$?
  [ "$rc" -ne 0 ] || fail "a missing darwin/arm64 checksum was rendered rather than refused"
  [ ! -e "${work}/out-hole/kapi-vision.rb" ] || fail "a refused render still left a formula behind"
  echo "✓ self-test: a missing platform checksum stops the generator"

  # 3. An unknown plugin is refused before anything is written.
  rc=0
  ( generate nosuch 9.9.9 neokapi/neokapi "${work}/sums-vision" "${work}/out-unknown" ) 2>/dev/null || rc=$?
  [ "$rc" -ne 0 ] || fail "an unknown plugin was accepted"
  echo "✓ self-test: an unknown plugin is refused"
}

case "${1:-}" in
  --self-test) self_test ;;
  -h | --help | "") usage ;;
  *) generate "$@" ;;
esac
