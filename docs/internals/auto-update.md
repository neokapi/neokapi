# Auto-update & distribution

Status: **Phases 1–4 implemented** (CLI self-update + notifier; cross-platform
desktop in-app updates via the Wails native updater + per-(os,arch) signed
appcasts; winget; Linux apt/yum). All need their release-side wiring exercised
on a real tagged release and the documented operational gates (signing keys,
per-user Windows NSIS, on-device swap validation) before going live. Phase 5
(Velopack) pending. Tracking doc for how neokapi keeps its shipped artifacts up
to date across macOS, Windows, and Linux.

## The model (read this first)

There are exactly two update models, and the **install method decides which one
is legal** for a given binary:

1. **Package-manager-managed** (Homebrew, winget, apt/dnf, npm, scoop). The
   package manager owns the file on disk and tracks its version. If the app
   overwrites itself in place, the manager's recorded version/checksum drifts
   and the next `brew upgrade` / `apt upgrade` may downgrade, refuse, or clobber
   the self-installed copy. **Correct behavior: detect → print the exact upgrade
   command → exit.** (This is what `claude-code` does:
   `Run: brew upgrade claude-code@latest`.)

2. **Self-installed** (`curl | sh`, a direct tarball/zip download, or a Homebrew
   **cask that declares `auto_updates true`**). The app owns its own file and
   **may self-replace**.

> Why VSCode auto-updates *through* a Homebrew cask but claude-code doesn't:
> VSCode's cask sets `auto_updates true`, which tells `brew upgrade` to **back
> off and not touch the app**, so Squirrel.Mac (its signed self-updater) does the
> work without fighting brew. The two systems are deliberately de-conflicted. A
> self-updating desktop app therefore needs **both** a real signed updater **and**
> `auto_updates true` on its cask.

Two hard constraints that recur everywhere:

- **A self-replace must verify a signature before swapping the binary.** We
  already cosign-sign plugin tarballs and verify them in
  `cli/pluginhost/registry` (`VerifyBundle`); the CLI self-updater reuses that
  exact path rather than introducing GPG.
- **Compare against the version of the channel the user actually installed
  from**, never a global "latest". The cask `version` lags the GitHub release;
  comparing to the wrong number is the #1 false-"update available" bug.

## Where we are today (baseline, 2026-06)

- **CLI** (`kapi` + `kapi-bowrain` plugin): Homebrew tap formulae (hand-bumped by
  `release.yml`), raw tarballs/zips on GitHub Releases. **No** winget, **no**
  deb/rpm, **no** version-check, **no** self-update.
- **Desktops** (Kapi / Bowrain, Wails v3): signed+notarized DMG via cask,
  NSIS+jsign zip on Windows, bare tarball on Linux. **No** in-app updater of any
  kind (Wails v3 ships one; we don't use it). Casks have **no** `auto_updates`.
- **Plugins** (pdfium/vision/sat/bowrain): already good — cosign keyless-signed
  tarballs, a registry, signature verification in `pluginhost`, and
  `kapi plugin install/update`. **Reuse this infrastructure for the CLI itself.**

## Release channels: `stable` + `beta` (tag-driven, beta is a superset)

Two channels, selected purely by how a release is **tagged** — no promotion step:

- **`stable`** ← a full tag `vX.Y.Z`. The curated track; the default for fresh
  installs and for `kapi update` / the notifier.
- **`beta`** ← the **fast ring**. It carries every prerelease **and** every final
  — a strict superset of stable — so a beta user is always at least as current as
  stable and **never falls behind** between prereleases.

This superset shape is the key design choice. The alternative (beta = prereleases
only) strands beta users on the last `-rc` when a final/patch ships to stable; the
superset avoids that, at the cost of beta builds sometimes running a *final*
(non-prerelease) version — which the sticky channel below handles.

**Publishing (`release.yml` + `scripts/publish-appcast.sh`).** Driven by the tag:

| | prerelease tag `vX.Y.Z-rc.N` | final tag `vX.Y.Z` |
|---|---|---|
| `cli.json` / plugin registry | `--channel beta` (beta-only entry) | `--channel ""` → a **universal** entry that `registry.Resolve` matches for *any* channel |
| Homebrew formulae | `kapi-cli-beta` / `bowrain-cli-beta` only | **both** `kapi-cli` + `kapi-cli-beta` (and bowrain) at the final version |
| Desktop appcast feeds | `…-beta.xml` only | **both** `….xml` and `…-beta.xml` |
| Desktop casks | `kapi@beta` / `bowrain@beta` only | **both** `kapi` + `kapi@beta` (and bowrain) |

So a final reaches beta users on every install method (Homebrew formula, tarball
self-update via `cli.json`, and the desktop cask/appcast).

- **Homebrew naming**: the beta CLI is `kapi-cli-beta` / `bowrain-cli-beta` (class
  `KapiCliBeta`), **not** `@beta` — `Formulary.class_s` only rewrites `@`→`AT`
  before a digit, so an `@beta` formula expects the invalid class `KapiCli@beta`
  and can never load. The desktop apps are **casks**, where `@beta` *is* a legal
  token, so the beta desktop is `kapi@beta` / `bowrain@beta`. The `-beta` formula
  and `@beta` cask each `conflicts_with` their stable counterpart (same `kapi`
  binary / same `.app`), so a user is on one channel at a time. `brew install
  neokapi/tap/kapi-cli-beta` / `--cask neokapi/tap/kapi@beta` opts in.

**Client selection — sticky, shared (`core/channel`).** All apps (the CLI and both
desktops) resolve the channel through `core/channel`, a Viper-free framework
package so the bowrain desktop can use it too. Precedence:

1. `KAPI_UPDATE_CHANNEL` env (and, CLI-only, an explicit `update.channel` in
   `kapi.yaml` via `--channel` / config).
2. A **persisted** one-line preference next to `kapi.yaml` (`update-channel`).
3. The channel **inferred from the running build's version** (a prerelease ⇒
   `beta`, else `stable`).

The persisted preference is what makes beta **sticky**: a fresh prerelease build
pins `beta` on first run (`channel.EnsurePinned`), so when it later updates to a
*final* version (which alone would infer `stable`) it **stays on beta**. Because
the preference lives in one shared file, the CLI and both desktops follow one
channel per machine. The notifier is channel-aware — a beta Homebrew user is told
`brew upgrade kapi-cli-beta`.

> Caveat: only Homebrew publishes `-beta`/`@beta` variants today. winget/scoop
> beta tracks are a later add; until then their nudges use the base package name.

## Decision matrix (target state)

| Surface | Primary update path | Discoverability mirrors | Background auto-update? |
|---|---|---|---|
| `kapi` CLI | `kapi update` (self-replace on tarball install; nudge on managed) | brew (Mac+Linux), winget, deb/rpm | No — by design (nudge) |
| Kapi/Bowrain desktop (macOS) | Sparkle appcast + `auto_updates true` cask | Homebrew cask | **Yes** |
| Desktop (Windows) | Wails v3 updater / NSIS poller vs signed manifest | winget | Yes (app-driven) |
| Desktop (Linux) | AppImage+zsync **or** Flatpak | Flathub | Flatpak: yes; AppImage: app-driven |
| Plugins | `kapi plugin update` (already shipped) | registry | No (explicit) |

### Key external realities (mid-2026)

- **winget has no native background auto-update.** `winget upgrade --all` is
  manual; the third-party Winget-AutoUpdate (WAU) fills the gap. So winget buys
  discoverability + a one-command upgrade we can nudge toward — not push updates.
- **True background updates on Linux = Flatpak or Snap only.** AppImage, bare
  deb, and tarball are manual unless we build an updater (AppImageUpdate/zsync) or
  the user enables unattended-upgrades.
- **EV certs no longer buy a SmartScreen bypass.** SmartScreen is reputation-
  based (per-cert + per-file-hash); budget for a warm-up period on new Windows
  builds regardless of cert tier.
- **Velopack** would be the dream single-framework (Win/Mac/Linux, deltas)
  answer, but **has no Go binding yet** (listed "Planned"). Watch-item, not a
  choice today.
- **Homebrew is moving toward `bundle_version`-aware staleness detection** for
  `auto_updates` casks (PRs #21882 → #21962/#21985). Keep cask `version` and the
  app's `CFBundleVersion` truthful so the new audit doesn't flag false upgrades.

## Phased plan

### Phase 1 — CLI update story (implemented; CI-side verified only on next release)

Highest value, lowest risk; reuses existing cosign infra. Mirrors claude-code.

- [x] `core/version.InstallSource` var (stamped via ldflags for channel-specific
      builds; see the **design note** below on why the *shared* archive is left
      unstamped).
- [x] `cli/selfupdate` package (`source.go`, `check.go`, `notify.go`,
      `apply.go`, + tests):
  - [x] install-source detection: `KAPI_INSTALL_SOURCE` env override → build
        flag → path heuristics (Cellar/linuxbrew, winget Packages, scoop) →
        `SourceUnknown`. `CanSelfReplace` adds a writability probe (never
        self-replace a non-writable path; nudge instead).
  - [x] latest-version check against the signed `cli.json` index (reuses
        `registry.FetchIndex`/`Resolve`), cached ~24h under the config dir;
        **gated off** when non-TTY, `CI`/`GITHUB_ACTIONS` set; opt-out via
        `DO_NOT_TRACK=1` and `KAPI_NO_UPDATE_CHECK=1`. Never blocks / affects
        exit code (detached PreRun refresh + cache-only PostRun render).
  - [x] per-source upgrade-command formatting.
- [x] `kapi update` command:
  - managed install → print (with `--run`, execute) the exact upgrade command.
    apt/dnf are never auto-run (need sudo/TTY); on winget/brew a failed
    auto-run falls back to printing.
  - tarball/own-installer → self-replace (stdlib download + atomic temp-file
    rename; on Windows rename the running `.exe` aside first), after verifying
    SHA-256 **and** a cosign signature via `pluginhost/registry.VerifyBundle`
    with the signing identity/issuer **pinned** to the neokapi release workflow.
    Refuses to self-replace an unsigned/untrusted build (no `--unsafe`).
- [x] async, cached, gated notifier wired into the kapi root command
      (`kapi/cmd/kapi/root.go`).
- [x] `release.yml`: cosign-sign the `kapi-cli_*.tar.gz` archives and publish the
      signed `cli.json` index (via `registry-update --plugin kapi --registry
      cli.json`) so tarball self-update can verify. **Only runnable on a real
      tagged release** — not exercised in this environment.

**Design note — why the shared CLI archive is *not* stamped with InstallSource.**
One built binary per platform is consumed by Homebrew, winget, **and** raw
download. If we baked `InstallSource=tarball` into it, a brew/winget install
would wrongly self-replace and corrupt the package manager's state. So the
canonical archive is left unstamped and `Detect()` relies on **path heuristics**
(Cellar/linuxbrew → homebrew, WinGet/Packages → winget, scoop → scoop) plus
`SourceUnknown`+writable → self-replaceable (covers raw tarball). `InstallSource`
is reserved for genuinely channel-specific builds (deb/rpm via nfpm, a
winget-only build) added in Phase 3.

**Follow-ups within Phase 1:**
- Windows `kapi.exe` self-update: the Windows CLI is signed + published out of
  band (`scripts/publish-windows-signed.sh`), so it is not yet in `cli.json`.
  Add the signed Windows zip to the index to enable `kapi update` self-replace
  on direct-download Windows installs.

### Phase 2 — desktop in-app updates (implemented; needs on-device validation)

**Chose the Wails v3 native updater over go-sparkle.** Wails v3 (already our
pinned `alpha.96`) ships `pkg/updater` with a Sparkle-`appcast` provider — pure
Go, **no cgo, no `Sparkle.framework` bundling, no nested-helper codesigning**,
cross-platform (also sets up Phase 4's Windows/Linux desktop updates), and
native `Config.Channel` filtering that maps onto our stable/beta split. It
reuses the Sparkle *appcast* vocabulary, so the feed format is standard.

- [x] `backend/updater.go` in both apps (`apps/kapi-desktop`, `bowrain/apps/bowrain`):
      builds the `appcast` provider for the current channel, pins the ed25519
      public key (`PublicKey`, fail-closed when unset), 6h background check,
      wired via `InitUpdater(app)` at startup. kapi-desktop also adds a
      "Check for Updates…" File-menu item; both expose `CheckForUpdatesNow`.
- [x] channel from `KAPI_UPDATE_CHANNEL` (default stable), per-channel feeds
      (`appcast-<app>-<os>-<arch>[-beta].xml`) so a stable build is never
      offered a beta item, and each platform/arch fetches its own feed.
- [x] `scripts/mkappcast` — the signed-appcast generator + `keygen`. **Crucial
      detail:** the Wails `ed25519` verifier checks `ed25519.Verify(pub,
      sha256(file), sig)`, i.e. the signature is over the artifact's SHA-256
      *digest* — which Sparkle's own `generate_appcast`/`sign_update` do **not**
      produce (they sign the raw file). So `mkappcast` signs the digest itself;
      a unit test reproduces the exact verifier path to guarantee compatibility.
- [x] `scripts/publish-appcast.sh` + `release.yml` (both desktop jobs): zip the
      notarized+stapled `.app`, sign it into the channel's appcast, upload the
      zip to the release, and publish the feed to the registry repo
      (`neokapi.github.io/registry/appcast-*.xml`). No-ops until the signing key
      + `REGISTRY_TOKEN` are set.
- [ ] **Gate — validate on a real notarized build before flipping casks.** The
      native updater's swap helper renames the `.app` in place; Gatekeeper /
      quarantine correctness on a notarized build must be confirmed on-device
      (it is the one thing Sparkle's signed XPC installer would handle for us).
- [ ] add `auto_updates true` to the `kapi` / `bowrain` **casks** (generated by
      the "Update kapi/bowrain cask" heredocs in `release.yml`) so `brew upgrade
      --cask` defers to the in-app updater. **Hold until the gate above passes** —
      flipping it early while the updater can't yet self-update would strand
      cask users.

#### One-time setup runbook (Phase 2)

1. **Generate the updater signing key** (once for both apps):
   ```bash
   go run ./scripts/mkappcast keygen
   ```
   - Commit the printed **public** key (base64) to both
     `apps/kapi-desktop/backend/update-ed25519.pub` and
     `bowrain/apps/bowrain/backend/update-ed25519.pub` (replacing the
     `REPLACE_WITH_…` placeholder — until then the apps fail closed on signed
     releases).
   - Store the **private** key as the `UPDATE_ED25519_PRIVATE_KEY` GitHub secret
     (never commit it).
2. Ensure `REGISTRY_TOKEN` (write access to `neokapi/registry`) is set — it
   already is for the CLI `cli.json` publish.
3. Cut a normal release (`vX.Y.Z` → stable feed, `vX.Y.Z-rc.N` → beta feed).
   `release.yml` publishes the per-platform feeds
   (`appcast-kapi-darwin-arm64.xml`, `…-linux-<arch>.xml`, etc., and `-beta`
   variants) to the registry Pages site; Windows feeds come from
   `appcast-windows.yml` after local signing.
4. Install the resulting DMG, run "Check for Updates…", and confirm the
   download → verify → swap → relaunch works on a notarized build (the gate).
5. Only then: add `auto_updates true` to the cask heredocs in `release.yml`
   ("Update kapi cask" / "Update bowrain cask" steps).

## Release-asset naming

Asset names mirror the Homebrew names so the channels are consistent:

| Product | Homebrew | Release asset |
|---|---|---|
| CLI toolchain | formula `kapi-cli` | `kapi-cli_<ver>_<os>_<arch>.tar.gz` |
| bowrain plugin | formula `bowrain-cli` | `kapi-bowrain_<ver>_…` |
| Kapi desktop | cask `kapi` | `kapi-<ver>-macOS-arm64.dmg` (+ `-windows-`/`-linux-`) |
| Bowrain desktop | cask `bowrain` | `bowrain-<ver>-…` |

So the toolchain is the `kapi-cli` / `kapi-*` family and the apps are plain
`kapi` / `bowrain`. winget mirrors this: `Neokapi.KapiCLI` (CLI),
`Neokapi.Kapi` (desktop). The CLI's `cli.json` self-update index key stays
`kapi` (what the binary looks itself up as) even though its archive is
`kapi-cli_*`. The desktop appcast feeds are `appcast-kapi-<os>-<arch>.xml`. The Go module
dirs (`apps/kapi-desktop`, the `kapi-desktop` artifact label) are unchanged —
only user-facing asset names follow this scheme.

### Phase 3 — winget + Linux packaging (discoverability + Linux update paths)

- [x] **winget already wired** (`.github/workflows/winget.yml`): `winget-releaser`
      submits `Neokapi.KapiCli` (CLI, portable zip) and `Neokapi.Kapi` (desktop,
      NSIS `-setup.exe`) to `winget-pkgs`, dispatched by
      `publish-windows-signed.sh` after the signed Windows assets land. The CLI
      self-update nudge points at `Neokapi.KapiCli`.
- [x] nfpm-built `.deb`/`.rpm` for the kapi CLI (`packaging/nfpm.yaml` +
      `release.yml`): `kapi-cli_<ver>_<arch>.deb` / `.rpm` with the kapi binary +
      toolbox symlinks, attached to the release and listed in `checksums.txt`.
      Direct-download packages (apt/dnf own updates) — not in `cli.json`.
- [x] **Self-hosted apt + yum repos** (`scripts/publish-packages.sh` +
      `release.yml`, stable channel only) served at
      `https://neokapi.github.io/packages/`. apt is a flat repo
      (`apt/pool/*.deb` + signed `InRelease`/`Release.gpg`); yum is
      `createrepo_c` output with a signed `repomd.xml.asc`. The signed indexes
      carry each package's SHA-256, so a tampered package is caught even though
      packages aren't individually signed. Validated end-to-end in
      Ubuntu/Rocky containers (`apt-get update` + `dnf makecache` both accept
      the repo and list `kapi-cli`).

#### apt/yum install (users)

```bash
# Debian/Ubuntu
curl -fsSL https://neokapi.github.io/packages/neokapi.gpg \
  | sudo gpg --dearmor -o /usr/share/keyrings/neokapi.gpg
echo "deb [signed-by=/usr/share/keyrings/neokapi.gpg] https://neokapi.github.io/packages/apt ./" \
  | sudo tee /etc/apt/sources.list.d/neokapi.list
sudo apt update && sudo apt install kapi-cli
```

```bash
# Fedora/RHEL/Rocky
sudo tee /etc/yum.repos.d/neokapi.repo >/dev/null <<'EOF'
[neokapi]
name=neokapi
baseurl=https://neokapi.github.io/packages/yum
enabled=1
gpgcheck=0
repo_gpgcheck=1
gpgkey=https://neokapi.github.io/packages/neokapi.gpg
EOF
sudo dnf install kapi-cli
```

(`gpgcheck=0 repo_gpgcheck=1`: the *index* is signed and carries the rpm
checksums; per-package rpm signing is a possible later hardening.)

#### apt/yum one-time setup (maintainer gate)

1. Create the **`neokapi/packages`** repo (a README is enough) and enable
   GitHub Pages (serve `main` / root) → `https://neokapi.github.io/packages/`.
2. Generate an **RSA-4096** GPG signing key — **not ed25519**; rpm/dnf cannot
   verify ed25519 repomd signatures (apt accepts either):
   ```bash
   gpg --batch --gen-key <<EOF
   %no-protection
   Key-Type: RSA
   Key-Length: 4096
   Name-Real: neokapi packages
   Name-Email: packages@neokapi.dev
   Expire-Date: 0
   %commit
   EOF
   gpg --armor --export-secret-keys packages@neokapi.dev   # → PACKAGES_GPG_PRIVATE_KEY
   ```
3. Set secrets: `PACKAGES_GPG_PRIVATE_KEY` (armored private key) and
   `PACKAGES_TOKEN` (PAT with write to `neokapi/packages`).
4. Cut a stable release — `release.yml` publishes the `.deb`/`.rpm` into the repo.
- [ ] Possible later hardening: per-package `.deb`/`.rpm` GPG signing, a beta
      apt component, and a branded domain (CNAME on the Pages repo).

### Phase 4 — Windows/Linux desktop updaters (investigated; design locked)

The Wails native updater is the **same mechanism** as macOS — it works on all
three OSes because the appcast provider filters items by `sparkle:os`
(`macos`→darwin, `windows`, `linux`). Verified against the Wails v3 source. So
this phase is *additive*: produce Windows + Linux update artifacts and add their
enclosures to the feeds. Key facts from reading `v3/pkg/updater`:

- **It swaps exactly one on-disk target** (`os.Executable()`) by renaming a
  single extracted top-level entry into place — **it cannot run an installer**
  (`.msi`/`.pkg`/`-setup.exe` unsupported in v1). So every update artifact is a
  one-entry archive (the binary/app), which is exactly what we already build.
- **No arch matching** — items are filtered by OS only. Multi-arch needs
  **per-(os,arch) feeds** (`appcast-<app>-<os>-<arch>[-beta].xml`); the app
  picks its URL from `runtime.GOOS`/`GOARCH`. (macOS is arm64-only → one feed.)

**Windows** — artifact = a `.zip` containing exactly one signed `Kapi.exe`
(this is the `kapi-<ver>-windows-<arch>.zip` we already produce). Caveats:
- The swap helper has **no UAC elevation**, so the in-app swap only works for a
  **per-user** install (`%LOCALAPPDATA%`); a Program Files install can't
  self-update. → ship the NSIS installer as **per-user**.
- A binary swap leaves NSIS/registry `DisplayVersion` stale, so **winget would
  perpetually see an "upgrade"**. Pick one authority per install: per-user →
  in-app zip-swap; managed/Program-Files → winget's `-setup.exe` with the
  in-app updater disabled.

**Linux** — artifact = a `.tar.gz` with one top-level binary (this is the
`kapi-<ver>-linux-<arch>.tar.gz` we already produce). The Unix swap is
`RemoveAll`+`Rename`, which works **only if the binary lives in a user-writable
dir** (`~/.local/bin`, `~/Applications`) — document that. AppImage+zsync is a
nicer UX but a **separate** updater path (inside an AppImage, `os.Executable()`
is the read-only FUSE mount, so the Wails swap can't write it); reach for it
only if delta downloads matter. **Flatpak: not an in-app updater — skip.**

- [x] `mkappcast gen --os macos|windows|linux`; `publish-appcast.sh <… os arch>`
      emits per-(os,arch) feeds (`appcast-<name>-<os>-<arch>[-beta].xml`),
      signing the existing Windows `.zip` / Linux `.tar.gz` with the same
      ed25519 digest scheme.
- [x] desktop apps: `feedURL()` keyed on `runtime.GOOS`+`GOARCH`.
- [x] CI: macOS + Linux desktop jobs publish their feeds in `release.yml`;
      Windows feeds via the dispatched `appcast-windows.yml` (triggered by
      `publish-windows-signed.sh` after the signed zips land).
- [ ] **Gates before it's live**: ship the Windows NSIS installer **per-user**
      (the swap can't elevate) and decide the winget-vs-in-app authority per
      install; document/standardize a **user-writable** Linux install dir; then
      validate swap+relaunch on per-user Windows and a writable-dir Linux
      install (mirror of the macOS Gatekeeper gate). AppImage+zsync optional.

### Phase 5 — revisit Velopack

- [ ] If/when a Go binding ships, evaluate collapsing Phases 2–4 onto one
      cross-platform framework.

## Reference implementation notes

- **Go self-update library**: `minio/selfupdate` (checksum + code-sign
  verification + binary patching + rollback; handles the Windows
  rename-running-exe trick). `creativeprojects/go-selfupdate` is the maintained
  high-level wrapper if we ever want straight-from-GitHub-Releases updates.
- **Version-check UX**: async/non-blocking (render the notice *after* the
  command's real output or on next run), ~24h cache, TTY/CI-gated,
  `DO_NOT_TRACK` + `KAPI_NO_UPDATE_CHECK` opt-out, message tailored to the
  detected install source, fail-open on network errors.
- **claude-code env-var design worth mirroring**: an opt-in
  "run the package manager's upgrade for me" flag; a "disable the background
  check only" flag; and a "disable all update paths" flag for managed/enterprise
  fleets.
- **Signing reuse**: `cli/pluginhost/registry.VerifyBundle(ctx, bundleURL,
  sha256Hex, certIdentity, certIssuer, opts)` already verifies a Sigstore bundle
  against the public-good trusted root with the same policy as `cosign
  verify-blob`. The CLI self-updater binds the release archive's SHA-256 to a
  cosign-signed manifest through this call.

## Open decisions

- Whether to also file these phases as GitHub issues for external visibility
  (this doc is the internal tracker).
- Linux GUI: AppImage+zsync vs Flatpak as the *primary* desktop channel.
- Whether `kapi update` should offer an opt-in "run brew/winget for me" by
  default or require an explicit flag (leaning: require a flag).
