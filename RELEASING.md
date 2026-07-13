# Releasing

How the neokapi repo ships its artifacts. Every lane is **tag-driven**: push a
scoped tag and the matching workflow builds, signs, and publishes. Versions are
read from source (`package.json` / ldflags), and the tag version is checked
against source so a tag can never drift from what actually ships.

| Artifact | Tag pattern | Workflow |
|---|---|---|
| Public `@neokapi` npm packages | `kapi-format-v*`, `kapi-react-v*`, `contract-types-v*`, `engine-v*` | `publish-npm.yml` |
| kapi / bowrain binaries + desktop apps + casks | `v[0-9]*` | `release.yml` |
| Coordinated multi-artifact release | (via `workflow_dispatch`) | `release-coordinated.yml` |
| Native plugins (sat, pdfium, asr, av, vision) | `sat-v*`, `pdfium-v*`, `asr-v*`, `av-v*`, `vision-v*` | `release-<name>.yml` |
| okapi-bridge / integrations | — | `publish-integrations.yml` |
| Windows winget / macOS appcast | (post-release) | `winget.yml`, `appcast-windows.yml` |

This document details the **npm package lane**, which has a one-time
bootstrap step per package that is easy to trip over. The binary and plugin
lanes are self-contained in their workflows.

---

## npm packages (`@neokapi/*`)

Four public packages publish from this repo, all **Apache-2.0**:

| Package | Purpose | Depends on |
|---|---|---|
| `@neokapi/kapi-format` | Format primitives for JS consumers | — |
| `@neokapi/kapi-react` | React bindings | `@neokapi/kapi-format` |
| `@neokapi/contract-types` | Go→TS generated IO-contract + content types | — |
| `@neokapi/engine` | Wasm engine boot loader (versioned ABI) | `@neokapi/contract-types` |

Publishing uses **npm Trusted Publishers (OIDC)** — there is **no `NPM_TOKEN`**.
Each package's trusted-publisher config on npmjs.com points at
`publish-npm.yml` in `neokapi/neokapi`. The workflow requires npm ≥ 11.5.1 for
OIDC provenance (Node 24 bundles a new enough npm; the workflow only *asserts*
the floor — it must not `npm install -g npm@latest`, which breaks the bundled
`sigstore` module and makes `npm publish --provenance` die with
`Cannot find module 'sigstore'`).

### Cutting a release (already-published package)

1. Bump the `version` in the package's `package.json` (and run any codegen /
   `pnpm install` so the lockfile matches).
2. Commit to `main`.
3. Push the scoped tag matching the new version:

   ```bash
   git tag contract-types-v0.2.0
   git push origin contract-types-v0.2.0
   ```

`publish-npm.yml` then builds all four packages in dependency order and
publishes any whose current version isn't yet on npm. The version guard makes
re-runs idempotent — a tag whose version is already published is a no-op, so
re-pushing or `workflow_dispatch` is always safe.

You can also trigger the workflow manually (`workflow_dispatch`) as a fallback;
it publishes whatever versions in `package.json` aren't yet on npm.

### First publish of a brand-new package — one-time bootstrap

**OIDC trusted publishing cannot _create_ a package** — the trusted-publisher
config lives on an existing package's settings page, so the very first version
of a new scope name must be published manually by a maintainer. If you skip
this, the workflow fails with:

```
404 Not Found - PUT https://registry.npmjs.org/@neokapi%2f<name>
… or you do not have permission to publish it
```

Bootstrap once, locally, with an npm account that has publish rights to the
`@neokapi` org:

```bash
npm login                         # authenticate the maintainer account

# From the package dir — use `pnpm pack`, NOT `npm publish` in the dir
# (see "Why pnpm pack" below). Build first so dist/ exists.
pnpm --filter @neokapi/<name> run build
cd packages/<name>
pnpm pack --pack-destination /tmp
npm publish /tmp/neokapi-<name>-<version>.tgz --access public \
  --@neokapi:registry=https://registry.npmjs.org/
```

Then, on npmjs.com → the package's **Settings → Trusted Publisher**, point it at:

- Repository: `neokapi/neokapi`
- Workflow: `publish-npm.yml`

After that, all subsequent versions publish tokenlessly via the tag flow above.

> **Current bootstrap status (2026-07):** `@neokapi/kapi-format` and
> `@neokapi/kapi-react` are live on npmjs and fully OIDC-configured.
> `@neokapi/contract-types@0.1.0` and `@neokapi/engine@0.1.0` are **not yet on
> npmjs** — each needs the one-time bootstrap above before its tag flow works.
> Bootstrap `contract-types` before `engine` (engine depends on it).

### Two gotchas baked into the workflow

**1. `pnpm pack`, not `npm publish` in the dir.** The `@neokapi` packages split
`exports` (TS `src`, for in-repo workspace consumers) from
`publishConfig.exports` (built `dist`, for npm consumers). pnpm's
`publishConfig` field substitution is applied only by `pnpm pack` / `pnpm
publish` — the npm CLI does **not** apply it. So `npm publish` in the package
dir would ship the `src`-based exports and npm consumers would get raw TS. The
workflow runs `pnpm pack` to produce a tarball whose `package.json` already has
the `dist` substitution applied, then `npm publish <tarball>` (npm is kept for
the publish because only the npm CLI implements the OIDC token exchange —
pnpm#9812). Provenance still works with a tarball arg in CI. **Do the same in
the bootstrap.**

**2. The scope-registry override on every `npm view` / `npm publish`.** The repo
root `.npmrc` maps the `@neokapi` scope to **GitHub Packages** for installs, and
a scope-specific registry takes precedence over a plain `--registry=`. So to
query or publish to **npmjs**, you must pass `--@neokapi:registry=https://registry.npmjs.org/`
— a bare `--registry=https://registry.npmjs.org/` is silently overridden by the
`.npmrc` scope mapping and hits GitHub Packages instead. This is why
`npm view @neokapi/<name> --registry=…` can wrongly report a published package
as missing.
