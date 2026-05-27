# Brewfile — macOS dependencies for building, releasing and signing neokapi.
# Install everything with:  brew bundle   (run from the repo root)

# ── Build toolchain ──────────────────────────────────────────────
brew "go"            # framework, CLIs, desktop backends
brew "node"          # frontend workspaces (then run `vp install` at the repo root)
brew "pkg-config"    # CGO builds locate native libraries
brew "icu4c"         # ICU + FTS5 for the framework/desktop CGO builds
                     #   (keg-only — expose it on PKG_CONFIG_PATH; see CLAUDE.md)

# ── Release & code signing ───────────────────────────────────────
brew "gh"            # GitHub CLI — release download/upload/publish, secrets
brew "jsign"         # Authenticode signing of Windows .exe/.zip (pulls openjdk);
                     #   used by scripts/publish-windows-signed.sh
brew "osslsigncode"  # alternative Authenticode signer (PKCS#11)
brew "makensis"      # NSIS — builds the Windows desktop installers (setup.exe)
                     #   in scripts/publish-windows-signed.sh (needs wails3 too)
brew "goreleaser"    # local release builds / `goreleaser check` (matches CI)
brew "cosign"        # Sigstore signing of plugin tarballs (matches CI)

# ── macOS apps (Homebrew Cask) ───────────────────────────────────
cask "simplysign"    # SimplySign Desktop — Certum cloud code signing. Presents
                     #   the cloud cert to the OS as a smart card, so jsign signs
                     #   via JSIGN_STORETYPE=CRYPTOCERTUM (no PKCS#11 dylib).
                     #   Intel build → needs Rosetta 2:
                     #     softwareupdate --install-rosetta --agree-to-license

# ── Not on Homebrew — install manually ───────────────────────────
# • wails3 (desktop apps):
#     go install github.com/wailsapp/wails/v3/cmd/wails3@latest
# • quill (macOS notarization — used in CI, not usually needed locally):
#     https://github.com/anchore/quill
