# Auto-update & distribution

Status: **Phase 1 implemented** (CLI self-update + notifier; release-side wiring
lands on the next tagged release). Phases 2–5 pending. Tracking doc for how
neokapi keeps its shipped artifacts up to date across macOS, Windows, and Linux.

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

## Release channels: `stable` + `beta` (tag-driven)

Two channels, selected purely by how a release is **tagged** — no promotion step:

- **`stable`** ← a full tag `vX.Y.Z`. The curated track; the default for every
  install and for `kapi update` / the notifier.
- **`beta`** ← a prerelease tag `vX.Y.Z-rc.N` / `-beta.N` (anything with a `-`).
  The fast track for dogfooding and early adopters. Cutting the eventual full
  `vX.Y.Z` *is* the promotion to stable.

`beta` reuses the channel name already in the plugin registry (`stable`/`beta`).
Both the `cli.json` index and the Homebrew tap carry both tracks:

- **Index**: `release.yml` registers each build under its tag-derived channel
  (`registry-update --channel stable|beta`). `registry.Resolve("kapi", "",
  channel, …)` filters on it.
- **Homebrew**: stable ships `kapi-cli` / `bowrain-cli`; beta ships the
  `@`-versioned `kapi-cli@beta` / `bowrain-cli@beta` (class `KapiCliATBeta`),
  so the two tracks install **side by side**. `brew install
  neokapi/tap/kapi-cli@beta` opts in.

**Client selection.** `update.channel` config key (env `KAPI_UPDATE_CHANNEL`),
default `stable`, controls both `kapi update` and the background notifier;
`kapi update --channel beta` overrides per-invocation. The nudge is
channel-aware — a beta Homebrew user is told `brew upgrade kapi-cli@beta`, not
the stable formula. Point the in-repo dogfood project at `beta` to ride the
fast track.

> Caveat: only Homebrew publishes a `@beta` variant today. winget/scoop beta
> tracks are a later add; until then their nudges use the base package name.

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
- [x] `release.yml`: cosign-sign the `kapi_*.tar.gz` archives and publish the
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

### Phase 2 — macOS desktop Sparkle (the VSCode behavior)

- [ ] Sparkle via `go-sparkle`; EdDSA-signed appcast (`generate_appcast`) riding
      on existing Developer-ID signing + notarization.
- [ ] add `auto_updates true` to the `kapi` / `bowrain` casks so `brew upgrade
      --cask` defers to Sparkle.

### Phase 3 — winget + Linux packaging (discoverability + Linux update paths)

- [ ] winget manifests via `winget-releaser` (or `wingetcreate update`) in
      `release.yml`, PR'd to `winget-pkgs` per release.
- [ ] nfpm-built `.deb`/`.rpm` attached to releases; optional own apt/yum repo
      later (Tailscale/gh pattern) for system-managed Linux CLI updates.

### Phase 4 — Windows/Linux desktop updaters

- [ ] Windows: Wails v3 built-in updater **or** NSIS + signed-manifest poller.
- [ ] Linux: AppImage + embedded zsync (AppImageUpdate) and/or a Flatpak on
      Flathub for true background updates. (Today's bare tarball is an update
      dead-end.)

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
