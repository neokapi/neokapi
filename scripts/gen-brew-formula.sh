#!/usr/bin/env bash
#
# gen-brew-formula.sh — emit the kapi-cli + bowrain-cli Homebrew formulae.
#
# Replaces GoReleaser's `brews:` block (we no longer run GoReleaser for the
# CLI). Reads the per-archive sha256 from a checksums.txt and writes two Ruby
# formulae that download the matching release archive per OS/arch from the
# public GitHub release (the repo is public).
#
# Usage: gen-brew-formula.sh <version> <repo> <checksums.txt> <out-dir> [channel]
#   repo     e.g. neokapi/neokapi
#   out      formulae are written here
#   channel  "stable" (default) → kapi-cli.rb / bowrain-cli.rb
#            "beta"             → kapi-cli@beta.rb / bowrain-cli@beta.rb
#
# The beta variant is an `@`-versioned Homebrew formula (like node@18): the fast
# track installs alongside, not over, the stable formula. `brew install
# neokapi/tap/kapi-cli@beta` opts a user into the beta channel.
set -euo pipefail

version="${1:?version required}"
repo="${2:?repo (owner/name) required}"
checksums="${3:?checksums.txt required}"
out="${4:?out dir required}"
channel="${5:-stable}"
mkdir -p "$out"

# Channel-derived naming. For beta the formulae get an @beta suffix; Homebrew
# maps kapi-cli@beta → class KapiCliATBeta (the `@` becomes `AT`). The beta
# bowrain formula depends on the beta kapi so the fast track is self-consistent.
case "$channel" in
  stable)
    kapi_name="kapi-cli";       kapi_class="KapiCli"
    bowrain_name="bowrain-cli"; bowrain_class="BowrainCli"
    kapi_dep="neokapi/tap/kapi-cli"
    ;;
  beta)
    kapi_name="kapi-cli@beta";       kapi_class="KapiCliATBeta"
    bowrain_name="bowrain-cli@beta"; bowrain_class="BowrainCliATBeta"
    kapi_dep="neokapi/tap/kapi-cli@beta"
    ;;
  *)
    echo "gen-brew-formula.sh: unknown channel '$channel' (want stable|beta)" >&2
    exit 1
    ;;
esac

base_url="https://github.com/${repo}/releases/download/v${version}"

# sha256 for a release filename, looked up in checksums.txt.
sha_for() {
  local f="$1" s
  s=$(awk -v f="$f" '$2==f {print $1}' "$checksums")
  [ -n "$s" ] || { echo "gen-brew-formula.sh: no checksum for $f in $checksums" >&2; exit 1; }
  printf '%s' "$s"
}

# Emit an `on_macos/on_linux { on_arm/on_intel { url; sha256 } }` body for the
# archive family $1 (e.g. "kapi" or "kapi-bowrain"), indented 2 spaces. macOS is
# Apple-Silicon only (no on_intel); Linux covers both arches.
platform_block() {
  local fam="$1" ext_darwin="tar.gz" ext_linux="tar.gz"
  local f_da="${fam}_${version}_darwin_arm64.${ext_darwin}"
  local f_la="${fam}_${version}_linux_arm64.${ext_linux}"
  local f_li="${fam}_${version}_linux_amd64.${ext_linux}"
  cat <<RUBY
  on_macos do
    on_arm do
      url "${base_url}/${f_da}"
      sha256 "$(sha_for "$f_da")"
    end
  end

  on_linux do
    on_arm do
      url "${base_url}/${f_la}"
      sha256 "$(sha_for "$f_la")"
    end
    on_intel do
      url "${base_url}/${f_li}"
      sha256 "$(sha_for "$f_li")"
    end
  end
RUBY
}

# ---- kapi-cli ----
{
  echo "class ${kapi_class} < Formula"
  echo '  desc "AI-native localization framework — format-aware parsing, concurrent pipelines, and pluggable tools"'
  echo '  homepage "https://github.com/neokapi/neokapi"'
  echo "  version \"${version}\""
  echo '  license "Apache-2.0"'
  echo
  # Bundle the PDFium-backed PDF reader so it is installed with kapi-cli. The
  # plugin formula drops into the shared kapi plugins root; no cycle since
  # kapi-pdfium does not depend on kapi-cli.
  echo '  depends_on "neokapi/tap/kapi-pdfium"'
  echo
  platform_block "kapi"
  cat <<'RUBY'

  # Install kapi plus its multi-call toolbox aliases. kgrep / ksed / kcat / kconv
  # are symlinks to the kapi binary, which dispatches on its invocation name
  # (busybox-style) — no extra binaries, no extra download size.
  def install
    bin.install "kapi"
    bin.install_symlink bin/"kapi" => "kgrep"
    bin.install_symlink bin/"kapi" => "ksed"
    bin.install_symlink bin/"kapi" => "kcat"
    bin.install_symlink bin/"kapi" => "kconv"
  end

  test do
    system "#{bin}/kapi", "version"
    assert_match "grep", shell_output("#{bin}/kgrep --help 2>&1")
  end
end
RUBY
} > "$out/${kapi_name}.rb"

# ---- bowrain-cli ----
{
  echo "class ${bowrain_class} < Formula"
  echo '  desc "Bowrain plugin for kapi — sync .kapi projects with Bowrain Server"'
  echo '  homepage "https://github.com/neokapi/neokapi"'
  echo "  version \"${version}\""
  echo '  license "Apache-2.0"'
  echo
  echo "  depends_on \"${kapi_dep}\""
  echo
  platform_block "kapi-bowrain"
  cat <<'RUBY'

  def install
    plugin_dir = pkgshare/"plugins/bowrain"
    plugin_dir.mkpath
    plugin_dir.install Dir["bowrain/*"]
    # Symlink so HOMEBREW_PREFIX/share/kapi/plugins/bowrain/ resolves
    # to this formula's pkgshare/plugins/bowrain/.
    kapi_share = HOMEBREW_PREFIX/"share/kapi/plugins"
    kapi_share.mkpath
    ln_sf plugin_dir, kapi_share/"bowrain"
  end

  test do
    system HOMEBREW_PREFIX/"share/kapi/plugins/bowrain/kapi-bowrain", "version"
  end
end
RUBY
} > "$out/${bowrain_name}.rb"

echo "wrote $out/${kapi_name}.rb $out/${bowrain_name}.rb (channel=${channel})" >&2
