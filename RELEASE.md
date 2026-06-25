# Releasing neokapi

This is the maintainer runbook for cutting a release. Releases are **tag-driven**
and split into **two independent tracks** (Apache kapi vs AGPL bowrain), so each
ships on its own cadence and version number:

| Track | Tag | Workflow | Ships |
|-------|-----|----------|-------|
| **kapi** (Apache-2.0) | `vX.Y.Z` | [`release.yml`](.github/workflows/release.yml) | kapi CLI, Kapi Desktop, `kapi-cli` formula, `kapi` cask, `cli.json` self-update, winget |
| **bowrain** (AGPL-3.0) | `bowrain-vX.Y.Z` | [`release-bowrain.yml`](.github/workflows/release-bowrain.yml) | `kapi-bowrain` plugin, Bowrain Desktop, `bowrain-server`/`worker`/`web`/`keycloak` images, `bowrain-cli` formula, `bowrain` cask, `manifest-plugins.json` registration |

The tag prefixes don't overlap (`v[0-9]*` vs `bowrain-v[0-9]*`), so a push to one
track never triggers the other. (The per-plugin workflows own their own
`<plugin>-v*` namespaces — see `release-sat.yml` etc.) In both tracks CI builds
and publishes everything except the Windows binaries — those are produced as CI
artifacts and signed locally (Certum/SimplySign is a Mac-local step), then added
to the published release.

> The two tracks share the framework + `cli` modules, so the `kapi-bowrain`
> plugin must stay protocol-compatible with the kapi CLI users have installed.
> There is no CI-enforced compatibility gate yet, so cut a `bowrain-v*` release
> from a commit whose plugin matches a released kapi — they need not be the same
> commit, but keep them close.

### Coordinated (simultaneous) release

Independent cadence is the default. When you want kapi **and** bowrain to appear
in the package managers at the same moment (e.g. a feature that spans the CLI and
the plugin), use the coordinated path instead of two separate tags:

```bash
make release-coordinated kapi=1.3.4 bowrain=2.1.0   # either may be blank
```

This dispatches [`release-coordinated.yml`](.github/workflows/release-coordinated.yml),
which runs both tracks via their reusable `workflow_call` entry points with
`coordinated: true`. Both build in parallel and then **wait at a manual-approval
gate** — the `coordinated-release` GitHub Environment — before any tap/registry
write. Approve both pending deployments together (Actions UI or `gh run watch`)
and the Homebrew formulae/casks + the plugin/CLI registry commits land within
seconds of each other. Windows signing is still the same Mac-local follow-up per
track (`make release-windows` / `make release-bowrain-windows`).

> **One-time setup:** create a repo Environment named `coordinated-release` with
> *required reviewers*. Without reviewers the gate is a no-op (both publish
> immediately, so appearance is only as simultaneous as the two build times). The
> gate is bypassed entirely for routine tag-push releases — only the coordinated
> dispatch sets `coordinated: true`. Correctness never depends on the gate: the
> `bowrain-cli` formula `depends_on` `kapi-cli` and `min_kapi_version` gates the
> registry, so a transient skew is always install-consistent; the gate only buys
> a coordinated launch *moment*. winget (Microsoft's `winget-pkgs` PR queue) and
> apt/yum propagation have their own latency and are not gated.

> **Verify on the first coordinated release — cosign identity.** The archives are
> cosign keyless-signed and the registry (`cli.json` / `manifest-plugins.json`)
> records the expected signer as `…/release.yml@refs/tags/<tag>` /
> `…/release-bowrain.yml@refs/tags/<tag>`. Running via `workflow_call` can change
> the Fulcio certificate's SAN to the *coordinator* workflow/ref, which would make
> `kapi update` / `kapi plugin install` reject the artifact. Before relying on a
> coordinated release, confirm `cosign verify-blob` against the published archive
> with the recorded identity, and adjust the `--cert-identity` in the
> registry-update step (or fall back to independent tag pushes) if it differs.
> Homebrew formulae/casks are unaffected (they verify by sha256, not cosign).

## TL;DR

```bash
# kapi track (Apache)
make release v=1.3.4                  # pre-flight + tag v1.3.4 + push → CI builds & publishes
gh run watch                          # follow the release workflow
make release-windows v=1.3.4          # after CI: sign Windows + finalize (SimplySign Desktop logged in)
make release-winget v=1.3.4           # optional: submit the signed CLI to winget-pkgs

# bowrain track (AGPL)
make release-bowrain v=2.1.0          # pre-flight + tag bowrain-v2.1.0 + push → CI builds & publishes
gh run watch
make release-bowrain-windows v=2.1.0  # after CI: sign Bowrain Windows + publish its update feed
```

A leading `v` is tolerated: `v=1.3.4` and `v=v1.3.4` both tag `v1.3.4`
(`release-bowrain` likewise tags `bowrain-v2.1.0`). winget is kapi-only, so the
bowrain Windows step skips it automatically.

## The model

| Stage | Where | What |
|-------|-------|------|
| Build + publish | CI (`release.yml` / `release-bowrain.yml`) | kapi: kapi CLI, Kapi Desktop, cask, `cli.json`. bowrain: `kapi-bowrain` plugin, Bowrain Desktop, Docker images, cask, plugin registry |
| macOS signing | CI | Desktop `.app`/DMG — Developer ID + notarized; kapi CLI darwin binaries — Developer ID + notarized via quill |
| Windows signing | **local Mac** | kapi/desktop `.exe` — Authenticode via the Certum cert through SimplySign. The track is inferred from the tag prefix (`v*` vs `bowrain-v*`). |
| Plugin trust | CI (`release-bowrain.yml`) | `kapi-bowrain` tarballs cosign/Sigstore-signed (supply-chain, not OS code signing — see [AD-007](web/docs/contribute/architecture/007-plugin-system.md)) |

Why Windows is split out: the Certum certificate is held in Certum's cloud HSM and
reached through **SimplySign Desktop**, which only runs on a logged-in Mac — it
can't run on GitHub-hosted runners. So CI emits the Windows binaries as workflow
**artifacts**, and `make release-windows` signs them locally and uploads them to
the already-published release.

Consequence: between CI finishing and `make release-windows`, the GitHub release
has macOS/Linux assets but **no Windows assets yet**. Homebrew casks (desktop)
and the plugin registry are macOS/Linux/plugin-only, so they are unaffected.

## What gets signed

| Artifact | Platform | Signing | Stage |
|----------|----------|---------|-------|
| `Kapi.app` / `Bowrain.app` + DMG | macOS | Developer ID + notarized (`wails3 tool sign --notarize`, `notarytool`, stapled) | CI |
| `kapi` CLI binary (in tar.gz) | macOS | Developer ID + notarized (quill, `scripts/quill-sign-darwin.sh`) | CI |
| `kapi.exe`, desktop `.exe` (in zips) | Windows | Authenticode (jsign + Certum, `scripts/publish-windows-signed.sh`) | local |
| tarballs | Linux | none (no OS code-signing concept); `checksums.txt` | CI |
| `kapi-bowrain` tarball/zip | all | cosign / Sigstore keyless | CI |

## Signing identities

- **Apple — Developer ID Application: `Skissefabrikken AS` (Team `8X6GKF24MG`)**, plus an App Store Connect API key for notarization. Stored as CI secrets (see below).
- **Windows — Certum "Standard Code Signing in the Cloud" (OV)**, certificate subject `CN=Skissefabrikken AS`. Reached via SimplySign. There is **no card PIN** and **no per-signature approval** — a logged-in SimplySign Desktop session authorizes signing.

The Windows publisher name (`Skissefabrikken AS`) matches the macOS Developer ID,
so both platforms show the same publisher.

> Azure Artifact Signing was evaluated and rejected: it is geo-restricted to
> US/Canada/EU/UK, and Norway (EEA, not EU) is ineligible.

## Prerequisites

### CI (one-time; already configured)

GitHub Actions repo secrets used by `release.yml`:

| Secret | Purpose |
|--------|---------|
| `APPLE_DEVELOPER_ID_P12_BASE64`, `APPLE_DEVELOPER_ID_P12_PASSWORD` | Developer ID cert + key |
| `APPLE_SIGN_IDENTITY`, `APPLE_TEAM_ID` | signing identity / team |
| `APPLE_API_KEY_P8_BASE64`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID` | App Store Connect API key (notarization) |
| `HOMEBREW_TAP_TOKEN`, `REGISTRY_TOKEN` | cask + plugin-registry updates |

### Maintainer's Mac (for the Windows signing step)

1. `brew bundle` at the repo root — installs `jsign`, `gh`, and the
   **SimplySign Desktop** cask (see [`Brewfile`](Brewfile)).
2. Open **SimplySign Desktop** (menu-bar app) and log in with your Certum
   SimplySign account + the mobile-app OTP. The session is time-limited, so log
   in shortly before signing.
3. Create `~/simplysign-pkcs11.cfg`:
   ```
   name = SimplySign
   library = /usr/local/lib/libSimplySignPKCS.dylib
   ```
   (`make release-windows` defaults `JSIGN_KEYSTORE` to this path.)

### winget (one-time; optional)

- A **classic** GitHub PAT with `public_repo` scope, stored as the repo secret
  `WINGET_TOKEN`, on an account that has forked `microsoft/winget-pkgs`.
- Bootstrap the package once: `komac new Neokapi.Kapi` (creates the first
  manifest; a moderator approves it). After that, releases update it.

## Step by step

1. **Pre-flight.** Make sure `main` is green in CI and you are on an up-to-date
   `main` with a clean working tree. (`release.yml` also gates on the parity
   workflow having passed for the tagged commit.)
2. **Tag + push:**
   ```bash
   make release v=1.3.4
   ```
   Guards: clean tree, on `main`, in sync with `origin/main`, tag not already
   present. It then creates an annotated tag and pushes it.
3. **Watch CI:** `gh run watch`. The workflow publishes the macOS/Linux assets,
   Docker images, casks, and the plugin registry, and uploads the unsigned
   Windows binaries as artifacts.
4. **Sign Windows + finalize** (with SimplySign Desktop logged in):
   ```bash
   make release-windows v=1.3.4
   ```
   This downloads the Windows artifacts from the run, signs the CLI and desktop
   `.exe`s (auto-discovering the certificate alias from the live token),
   timestamps them via `http://time.certum.pl/`, uploads them to the release,
   and updates `checksums.txt`.
5. **winget (optional):**
   ```bash
   make release-winget v=1.3.4
   ```
6. **Verify** (see below).

## Verifying a release

- **macOS:** download the DMG through a browser on a clean machine; it should
  open with no Gatekeeper warning. Spot-check: `spctl -a -t install -vvv <App>.dmg`.
- **Windows CLI binary (from the Mac):**
  ```bash
  osslsigncode verify -in kapi.exe
  ```
  The signature, message digest, and timestamp should verify. A
  `unable to get local issuer certificate` error is **expected on macOS** — the
  Certum code-signing root isn't in the Mac's TLS bundle. Windows trusts it
  (Certum is in the Microsoft Trusted Root Program). For the real check, run
  `signtool verify /pa kapi.exe` on Windows.

## Troubleshooting

| Symptom | Cause / fix |
|---------|-------------|
| `make release-windows`: "Could not find a release.yml run for <tag>" | CI hasn't finished, or the run isn't associated with the tag. Wait, or pass `RUN_ID=<id>` (`gh run list`). For bowrain it looks for a `release-bowrain.yml` run for the `bowrain-v*` tag. |
| jsign: "no certificate found" / empty alias | SimplySign Desktop isn't logged in, or the session expired — log in again. |
| `osslsigncode verify`: `unable to get local issuer certificate` | Expected on macOS (no Certum code-signing root locally). Not a real failure; verify on Windows if unsure. |
| Cert reissued (annual, ≤459-day validity) | No change needed — the alias is auto-discovered from the token. If you ever pin it, set `JSIGN_ALIAS`. |

## Reference

- [`.github/workflows/release.yml`](.github/workflows/release.yml) — the kapi release pipeline (`v*`)
- [`.github/workflows/release-bowrain.yml`](.github/workflows/release-bowrain.yml) — the bowrain release pipeline (`bowrain-v*`)
- [`.github/workflows/release-coordinated.yml`](.github/workflows/release-coordinated.yml) — joint launch: runs both tracks behind the `coordinated-release` approval gate
- [`.github/workflows/winget.yml`](.github/workflows/winget.yml) — winget submission (dispatch-only, kapi-only)
- [`scripts/publish-windows-signed.sh`](scripts/publish-windows-signed.sh) — local Windows signing
- [`scripts/quill-sign-darwin.sh`](scripts/quill-sign-darwin.sh) — macOS CLI signing in CI
- [`Brewfile`](Brewfile) — maintainer toolchain
- [AD-007: Plugin System](web/docs/contribute/architecture/007-plugin-system.md) — plugin signing vs. OS notarization
- Tracking issue: [#655](https://github.com/neokapi/neokapi/issues/655)
