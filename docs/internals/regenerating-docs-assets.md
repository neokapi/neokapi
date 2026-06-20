# Regenerating documentation assets (videos, screenshots, scenes)

This is the maintainer runbook for re-recording and republishing every media
asset embedded on the three public surfaces:

| Surface | Path | Asset home |
|---|---|---|
| kapi docs + landing | `web` (baseUrl `/web/neokapi/`) | Cloudflare R2 (`kapi/{video,img,models}/`) |
| bowrain landing | `bowrain/web/landing` | committed in `bowrain/web/landing/public/` |
| bowrain docs | `bowrain/web/docs` (baseUrl `/web/bowrain/docs/`) | Cloudflare R2 (`bowrain/{video,img}/`) |

**Landing pages** carry their own committed images — nothing to regenerate
unless a screenshot in `public/` is replaced. Everything below is about the two
Docusaurus docs sites, whose video/image assets are **gitignored** and published
to the Cloudflare R2 CDN (served at `$DOCS_CDN_URL`); the sites reference them by
URL via `ThemedVideo` / `ThemedImage` / the Vision Lab (CI never records, never
stages — see `web/docs/contribute/notes-internal/cdn-assets.md`).

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
| kapi terminal scenes (`/video/kapi/01-*.webm`) | kapi docs recipes/quickstart | `make kapi-scenes` (VHS) | `web/static/video/kapi/` |
| kapi Claude explainers (`claude-app-i18n`, `claude-translate-document`) | `kapi/get-started/use-with-claude.mdx` | harness demos `02`,`03` (live Claude) | `web/static/video/kapi/` |
| kapi shell explainers (`kapi-checks-guardrail`, `toolbox-explainer`) | checks / toolbox pages | harness demos `05`,`09` (scripted shell) | `web/static/video/kapi/` |
| Kapi Desktop tour (`kapi-desktop-*`) | `kapi/desktop/overview.mdx` | harness desktop demos (×6) | `web/static/video/kapi/` |
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

# publish → R2 CDN (videos + images), then make it live
make publish-cdn-videos publish-cdn-images
gh workflow run docs-kapi.yml --ref main   # pages-deploy.yml auto-deploys on success
```

The harness publish stage writes `<publishAs>-{light,dark}.webm` + `.jpg`
posters straight into `web/static/video/kapi/`.

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
make publish-cdn-bowrain-videos publish-cdn-bowrain-images
gh workflow run docs-bowrain.yml --ref main
```

## 3. Verify live

R2 + Cloudflare edges can serve a stale object briefly after an overwrite.
**Verify by byte size, not HTTP 200**: compare the live `Content-Length` on the
CDN against the local file and re-publish until they match.

```bash
# example: compare one asset on the CDN
live=$(curl -sI https://cdn.bowrain.cloud/kapi/video/kapi/kapi-desktop-projects-dark.webm | awk '/content-length/{print $2}' | tr -d '\r')
local=$(stat -f%z web/static/video/kapi/kapi-desktop-projects-dark.webm)
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

## Desktop/web render reliability (important)

The framed desktop/web videos embed a Playwright screencast that Remotion's
OffthreadVideo seeks into per beat. The rust compositor is fragile here:

- **Separate capture from render for bowrain.** Capture needs the Docker stack
  up (server + Playwright). Rendering needs *RAM* — Docker Desktop's VM holds
  ~15 GB even when containers are only stopped, which starves Remotion's
  compositor and causes intermittent `Could not extract frame from compositor:
  Request closed` / `delayRender timeout` failures. Workflow that works:
  1. Stack up → `--only=capture` every bowrain demo (web + desktop).
  2. **Quit Docker Desktop entirely** (`osascript -e 'quit app "Docker"'`) — not
     just `compose stop` — to free the VM RAM. Volumes persist on disk, so the
     seed survives a Docker quit/restart (only `down -v` wipes it).
  3. `--only=narrate,render,publish` every demo from cache.
- **Render at concurrency 1 for the long/animated demos.** Set
  `HARNESS_RENDER_CONCURRENCY=1`. Higher values multiply parallel video-proxy
  seeks and crash the compositor. `render.ts` retries once and caps concurrency
  (default 4); 1 is the reliable floor for 70 s+ screencasts.
- The screencast is re-encoded to dense-keyframe VP9 at capture time
  (`reencodeDenseKeyframes` in `record-desktop.ts`) so seeks decode a short GOP.
- **`--only=capture` skips narration** — always include `narrate` in the render
  pass (`--only=narrate,render,publish`) or the video ships silent with fallback
  timing.

## Auth / seeding for bowrain captures

- Device-flow JWTs are short-lived. Re-mint before a capture session; a stale
  token silently redirects the SPA to Keycloak and you capture a login page.
  Verify by extracting a frame (`ffmpeg -ss 3 -i screencast-dark.webm -vframes 1`).
- The session cookie (`bowrain_session`) is scoped to **`path: /api/`** — match
  that in any Playwright auth helper, not `/`.
- Workspace routes are AD-011 bare-slug (`/:ws/...`): invites `/:ws/invites`,
  TM `/:ws/translation-memory`, terms `/:ws/terms`. The old `/workspaces/:ws/...`
  forms 404. `harness/scripts/seed-collaboration.mjs` seeds project + file + TM +
  terms + a second user and prints all the env the recorder needs.
- The docs site's Playwright (`bowrain/web/docs`) uses its own browser cache;
  run `vpx playwright install chromium` there once (or symlink the matching
  `chromium_headless_shell-<rev>` if a download stalls).

## Bowrain screenshots

`bowrain/web/docs/scenes/bowrain-web-screenshots/01-shots.spec.ts` captures the
web-app gallery (login + dashboard + workspace-rail + project-view + tm/term/
settings, light + dark) into `static/img/web-app/{theme}/`. Run from
`bowrain/web/docs` with `BOWRAIN_SESSION_TOKEN` + `BOWRAIN_WORKSPACE_SLUG` set
to a seeded workspace. It waits for the workspace to render (no blank frames).
</content>
