# Regenerating documentation assets (videos, screenshots, scenes)

This is the maintainer runbook for re-recording and republishing every media
asset embedded on the four public surfaces:

| Surface | Path | Asset home |
|---|---|---|
| kapi landing | `web/landing` | committed in `web/landing/public/` |
| kapi docs | `web/docs` (baseUrl `/web/neokapi/docs/`) | `docs-assets` GitHub release |
| bowrain landing | `bowrain/web/landing` | committed in `bowrain/web/landing/public/` |
| bowrain docs | `bowrain/web/docs` (baseUrl `/web/bowrain/docs/`) | `bowrain-docs-assets` GitHub release |

**Landing pages** carry their own committed images — nothing to regenerate
unless a screenshot in `public/` is replaced. Everything below is about the two
Docusaurus docs sites, whose video/image assets are **gitignored** and shipped
through a GitHub release that CI stages at build time (CI never records).

## 0. One-time setup

```bash
# Shared Gemini key for narration (all worktrees read this; honours $XDG_CONFIG_HOME)
mkdir -p ~/.config/neokapi
printf 'GEMINI_API_KEY=...\n' > ~/.config/neokapi/harness.env && chmod 600 ~/.config/neokapi/harness.env

brew install vhs ffmpeg            # vhs for terminal scenes
make build build-kapi-bowrain-plugin   # kapi + the kapi-bowrain plugin (fts5 + icu4c)
make harness-deps                  # harness node deps + Playwright chromium
cd harness && vpx tsx src/cli/run.ts --list   # sanity: list demos
```

The harness auto-detects the enclosing checkout as the repo. Narration
pace/voice live in committed constants (`harness/src/narrate/synth.ts`), never
in env.

## Asset inventory — what produces what

| Asset family | Embedded on | Produced by | Lands in |
|---|---|---|---|
| kapi terminal scenes (`/video/kapi/01-*.webm`) | kapi docs recipes/quickstart | `make kapi-scenes` (VHS) | `web/docs/static/video/kapi/` |
| kapi Claude explainers (`claude-app-i18n`, `claude-translate-document`) | `kapi/get-started/use-with-claude.mdx` | harness demos `02`,`03` (live Claude) | `web/docs/static/video/kapi/` |
| kapi shell explainers (`kapi-checks-guardrail`, `toolbox-explainer`) | checks / toolbox pages | harness demos `05`,`09` (scripted shell) | `web/docs/static/video/kapi/` |
| Kapi Desktop tour (`kapi-desktop-*`) | `kapi/desktop/overview.mdx` | harness desktop demos (×6) | `web/docs/static/video/kapi/` |
| bowrain CLI scenes (`/video/bowrain-cli/0N-*.webm`) | bowrain walkthroughs | `make bowrain-kapi-scenes` (VHS, needs a server) | `bowrain/web/docs/static/video/bowrain-cli/` |
| bowrain web walkthrough scenes (`/video/bowrain/<id>/0N-*.webm`) | bowrain walkthroughs | Playwright `scenes` (needs a server) + `stage-scenes.sh` | `bowrain/web/docs/static/video/bowrain/` |
| bowrain web framed videos (`/video/bowrain-web/bowrain-web-*`) | `server/web-overview.mdx` | harness demos `bowrain-web-*` (needs a server) | `bowrain/web/docs/static/video/bowrain-web/` |
| bowrain desktop framed videos (`/video/bowrain-desktop/bowrain-desktop-*`) | `desktop/overview.mdx` | harness demos `bowrain-desktop-*` | `bowrain/web/docs/static/video/bowrain-desktop/` |
| bowrain web screenshots (`/img/web-app/{light,dark}/*.png`) | `server/web-overview.mdx` | `bowrain/web/docs/scenes/bowrain-web-screenshots` (Playwright) | `bowrain/web/docs/static/img/web-app/` |
| bowrain desktop screenshots (`/img/bowrain/{light,dark}/*.png`) | `desktop/overview.mdx` | harness desktop wbridge screenshot pass | `bowrain/web/docs/static/img/bowrain/` |

## 1. kapi docs

```bash
# terminal scenes (cheap, no AI/stack)
make kapi-scenes

# desktop tour — one at a time (each self-manages an isolated real stack via wbridge)
cd harness
for id in projects content flows config explorer okapi; do
  vpx tsx src/cli/run.ts kapi-desktop-$id --force --theme=both
done

# Claude + shell explainers (02/03 are live, billed Claude sessions; 05/09 are scripted)
vpx tsx src/cli/run.ts 02-nextjs-zero-to-i18n --force --theme=both
vpx tsx src/cli/run.ts 03-translate-docx     --force --theme=both
vpx tsx src/cli/run.ts 05-ai-checks-guardrail --force --theme=both
vpx tsx src/cli/run.ts 09-toolbox-find-replace --force --theme=both
cd ..

# publish → docs-assets release (merges, never drops), then make it live
make publish-docs-assets
gh workflow run docs-kapi.yml --ref main   # pages-deploy.yml auto-deploys on success
```

The harness publish stage writes `<publishAs>-{light,dark}.webm` + `.jpg`
posters straight into `web/docs/static/video/kapi/`.

## 2. bowrain docs (needs a running stack)

```bash
# Bring up the full local stack (server + worker + deps, keyless `demo` provider)
make -C bowrain stack-up-web        # serves SPA + API at http://localhost:8080
export BOWRAIN_BACKEND_URL=http://localhost:8080

# Seed + auth (device flow → JWT planted as the bowrain_session cookie)
# (see harness/scripts/seed-*.mjs and bowrain/scripts/device-auth.sh)
```

### 2a. bowrain framed videos (harness)

```bash
cd harness
# collaboration is two genuine users — seed both first:
node scripts/seed-collaboration.mjs > /tmp/collab.json   # prints both tokens + project/item/locale
# export the env it printed (BOWRAIN_SESSION_TOKEN, BOWRAIN_PEER_TOKEN, …), then:
vpx tsx src/cli/run.ts bowrain-web-collaboration --force --theme=both \
  --docs-dir=../bowrain/web/docs/static/video/bowrain-web

for id in editor governance review correction-loop; do
  vpx tsx src/cli/run.ts bowrain-web-$id --force --theme=both \
    --docs-dir=../bowrain/web/docs/static/video/bowrain-web
done
for id in dashboard flows; do
  vpx tsx src/cli/run.ts bowrain-desktop-$id --force --theme=both \
    --docs-dir=../bowrain/web/docs/static/video/bowrain-desktop
done
cd ..
```

### 2b. bowrain web walkthrough scenes (Playwright)

```bash
cd bowrain/web/docs
BOWRAIN_SESSION_TOKEN=<jwt> vpx playwright test --config playwright.config.ts
bash scripts/stage-scenes.sh        # scenes/<id>/0N-*.webm → static/video/bowrain/<id>/
cd ../../..
```

### 2c. bowrain CLI scenes (VHS)

```bash
BOWRAIN_BACKEND_URL=http://localhost:8080 make bowrain-kapi-scenes
```

### 2d. bowrain screenshots (Playwright, light + dark)

```bash
cd bowrain/web/docs
BOWRAIN_SESSION_TOKEN=<jwt> vpx playwright test --config playwright.config.ts scenes/bowrain-web-screenshots
cd ../../..
```

### 2e. publish + deploy

```bash
make publish-bowrain-docs-assets
gh workflow run docs-bowrain.yml --ref main
```

## 3. Verify live

CDN edges can serve a stale tarball for ~15–25 min after a release upload, even
when a local `gh release download` already has the fresh one. **Verify by byte
size, not HTTP 200**: compare the live `Content-Length` against the local file
and re-trigger the workflow until they match.

```bash
# example: compare one asset
live=$(curl -sI https://neokapi.github.io/web/neokapi/docs/video/kapi/kapi-desktop-projects-dark.webm | awk '/content-length/{print $2}' | tr -d '\r')
local=$(stat -f%z web/docs/static/video/kapi/kapi-desktop-projects-dark.webm)
echo "live=$live local=$local"
```

## Notes / footguns

- **One render at a time** keeps CPU sane — the harness already renders demos
  sequentially; don't fan them out.
- All recordings run against **real** backends (no mocks): real Keycloak/`demo`
  provider, real bowrain-server, real SQLite/Postgres.
- `ThemedVideo` (`@neokapi/docs-shared`) resolves `src` through `useBaseUrl`;
  always use root-absolute `/video/...` / `/img/...` paths in MDX, never bare.
- Every served `.webm` must be **bt709 / limited-range** — the harness publish
  step and the scene make-targets re-tag accordingly (Chrome's VP9 path is
  strict; Safari is lenient).
- bowrain videos carry the **Bowrain** brand lockup (logo + indigo wordmark);
  this is the `brand: bowrain` card brand in `harness/src/remotion/components/Cards.tsx`.
</content>
