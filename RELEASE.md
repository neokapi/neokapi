# Releasing neokapi

This is the maintainer runbook for cutting a release. Releases are **tag-driven**:
pushing a `vX.Y.Z` tag triggers [`.github/workflows/release.yml`](.github/workflows/release.yml),
which builds and publishes everything except the Windows binaries — those are
produced as CI artifacts and signed locally (Certum/SimplySign is a Mac-local
step), then added to the published release.

## TL;DR

```bash
make release v=1.3.4          # pre-flight + tag + push  →  CI builds & publishes
gh run watch                  # follow the release workflow
make release-windows v=1.3.4  # after CI: sign Windows + finalize (SimplySign Desktop logged in)
make release-winget v=1.3.4   # optional: submit the signed CLI to winget-pkgs
```

A leading `v` is tolerated: `v=1.3.4` and `v=v1.3.4` both tag `v1.3.4`.

## The model

| Stage | Where | What |
|-------|-------|------|
| Build + publish | CI (`release.yml`) | kapi CLI + `kapi-bowrain` plugin, desktop apps (Wails v3), Docker images, Homebrew casks, plugin registry |
| macOS signing | CI | Desktop `.app`/DMG — Developer ID + notarized; CLI darwin binaries — Developer ID + notarized via quill |
| Windows signing | **local Mac** | CLI + desktop `.exe` — Authenticode via the Certum cert through SimplySign |
| Plugin trust | CI | `kapi-bowrain` tarballs cosign/Sigstore-signed (supply-chain, not OS code signing — see [AD-007](web/docs/contribute/architecture/007-plugin-system.md)) |

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

1. `brew bundle` at the repo root — installs `jsign`, `gh`, `goreleaser`, and the
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
| `make release-windows`: "Could not find a release.yml run for <tag>" | CI hasn't finished, or the run isn't associated with the tag. Wait, or pass `RUN_ID=<id>` (`gh run list`). |
| jsign: "no certificate found" / empty alias | SimplySign Desktop isn't logged in, or the session expired — log in again. |
| `osslsigncode verify`: `unable to get local issuer certificate` | Expected on macOS (no Certum code-signing root locally). Not a real failure; verify on Windows if unsure. |
| Cert reissued (annual, ≤459-day validity) | No change needed — the alias is auto-discovered from the token. If you ever pin it, set `JSIGN_ALIAS`. |

## Reference

- [`.github/workflows/release.yml`](.github/workflows/release.yml) — the release pipeline
- [`.github/workflows/winget.yml`](.github/workflows/winget.yml) — winget submission (dispatch-only)
- [`scripts/publish-windows-signed.sh`](scripts/publish-windows-signed.sh) — local Windows signing
- [`scripts/quill-sign-darwin.sh`](scripts/quill-sign-darwin.sh) — macOS CLI signing in CI
- [`Brewfile`](Brewfile) — maintainer toolchain
- [AD-007: Plugin System](web/docs/contribute/architecture/007-plugin-system.md) — plugin signing vs. OS notarization
- Tracking issue: [#655](https://github.com/neokapi/neokapi/issues/655)
