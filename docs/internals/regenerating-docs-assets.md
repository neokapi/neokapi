# Regenerating documentation assets (videos, screenshots, scenes)

This is the maintainer runbook for re-recording and republishing every media
asset embedded on the three public surfaces:

| Surface | Path | Asset home |
|---|---|---|
| kapi docs + landing | `web` (baseUrl `/`) | the S3 + CloudFront CDN (`kapi/{video,img,models}/`) |
| bowrain landing | `bowrain/web/landing` | committed in `bowrain/web/landing/public/` |
| bowrain docs | `bowrain/web/docs` (baseUrl `/docs/`) | the S3 + CloudFront CDN (`bowrain/{video,img}/`) |

**Landing pages** carry their own committed images; nothing to regenerate
unless a screenshot in `public/` is replaced. Everything below is about the two
Docusaurus docs sites, whose video/image assets are **gitignored** and published
to the CDN bucket (`$CDN_BUCKET`, served at `$DOCS_CDN_URL`) by
`scripts/publish-cdn-assets.sh`; the sites reference them by URL via
`ThemedVideo` / `ThemedImage` / the Vision Lab (CI never records, never
stages; see `web/docs/contribute/implementation/repo/cdn-assets.md`).

## 0. One-time setup

```bash
# Shared Gemini key for narration (all worktrees read this; honours $XDG_CONFIG_HOME)
mkdir -p ~/.config/neokapi
printf 'GEMINI_API_KEY=...\n' > ~/.config/neokapi/harness.env && chmod 600 ~/.config/neokapi/harness.env

brew install vhs ffmpeg            # vhs records the walkthrough tapes
make build build-bowrain-plugin    # kapi + the kapi-bowrain plugin (fts5 + icu4c)
make harness-deps                  # harness node deps + Playwright chromium
cd harness && vpx tsx src/cli/run.ts --list   # sanity: list demos
```

The harness auto-detects the enclosing checkout as the repo. Narration
pace/voice live in committed constants (`harness/src/narrate/synth.ts`), never
in env.

## Asset inventory: what produces what

| Asset family | Embedded on | Produced by | Lands in |
|---|---|---|---|
| kapi walkthrough scenes (`web/walkthroughs/<id>.scene.yaml`) | `web/docs/walkthroughs/<id>.mdx` | `scripts/walkthrough-gen/gen.ts` emits the VHS tape and the playground embed from the scene spec; `vhs` records the tape in `web/scenes/<id>/` | `web/static/video/kapi/` |
| kapi Claude explainers (`claude-app-i18n`, `claude-translate-document`) | `kapi/get-started/use-with-claude.mdx` | harness demos `02`,`03` (live Claude) | `web/static/video/kapi/` |
| kapi shell explainers (`kapi-checks-guardrail`, `toolbox-explainer`) | checks / toolbox pages | harness demos `05`,`09` (scripted shell) | `web/static/video/kapi/` |
| kapi monolingual journey (`monolingual-governance`) | `kapi/recipes/keep-source-on-brand.mdx` | harness demo `s0-northsea-governance` (scripted shell, seeded from `samples/northsea`) | `web/static/video/kapi/` |
| kapi multilingual ship states (`multilingual-ship-states`) | the multilingual/ship-state page | harness demo `s1-compass-multilingual` (scripted shell, seeded from `samples/compass`) | `web/static/video/kapi/` |
| kapi docs-site convergence (`docs-site-convergence`) | the CI / convergence page | harness demo `s2-tidewatch-docs` (scripted shell, seeded from `samples/tidewatch-docs`) | `web/static/video/kapi/` |
| Kapi Desktop tour (`kapi-desktop-*`) | `kapi/desktop/overview.mdx` | harness desktop demos `kapi-desktop-{projects,content,flows,config,explorer}` | `web/static/video/kapi/` |
| bowrain CLI videos (`/video/bowrain-cli/bowrain-cli-*`) | bowrain walkthroughs | harness demos `bowrain-cli-getting-started`, `bowrain-cli-auth-and-workspaces` (need a server) | `bowrain/web/docs/static/video/bowrain-cli/` |
| bowrain web framed videos (`/video/bowrain-web/bowrain-web-*`) | `server/web-overview.mdx` | harness demos `bowrain-web-{editor,governance,review,correction-loop,collaboration}` (need a server) | `bowrain/web/docs/static/video/bowrain-web/` |
| bowrain desktop framed videos (`/video/bowrain-desktop/bowrain-desktop-*`) | `server/desktop-app.mdx` | harness demos `bowrain-desktop-{dashboard,automations}` | `bowrain/web/docs/static/video/bowrain-desktop/` |
| bowrain walkthrough scenes (`bowrain/web/docs/scenes/<id>/0N-*.webm`) | bowrain walkthroughs | committed recordings; no recorder is checked in, so a change means re-recording by hand and committing the file | `bowrain/web/docs/scenes/<id>/` |
| bowrain web screenshots (`/img/web-app/{light,dark}/*.png`) | `server/web-overview.mdx`, `walkthroughs/bowrain-automation.mdx` | no producer is checked in: capture by hand from a seeded stack (`harness/scripts/theme-shots.mjs` shows the pattern, but it writes palette candidates to `harness/theme-shots/`, not the gallery) | `bowrain/web/docs/static/img/web-app/` |
| bowrain desktop screenshots (`/img/bowrain/{light,dark}/*.png`) | `server/desktop-app.mdx` | same: no producer is checked in | `bowrain/web/docs/static/img/bowrain/` |

## 1. kapi docs

```bash
# walkthrough tapes (cheap, no AI/stack): regenerate from the scene specs, then record
node --experimental-strip-types scripts/walkthrough-gen/gen.ts --all
(cd web/scenes/<id> && vhs 01-*.tape)

# desktop tour, one at a time (each self-manages an isolated real stack via wbridge)
cd harness
for id in projects content flows config explorer; do
  vpx tsx src/cli/run.ts kapi-desktop-$id --force --theme=both
done

# Claude + shell explainers (02/03 are live, billed Claude sessions; the rest are scripted)
vpx tsx src/cli/run.ts 02-nextjs-zero-to-i18n --force --theme=both
vpx tsx src/cli/run.ts 03-translate-docx     --force --theme=both
vpx tsx src/cli/run.ts 05-ai-checks-guardrail --force --theme=both
vpx tsx src/cli/run.ts 09-toolbox-find-replace --force --theme=both
vpx tsx src/cli/run.ts s0-northsea-governance --force --theme=both
vpx tsx src/cli/run.ts s1-compass-multilingual --force --theme=both
vpx tsx src/cli/run.ts s2-tidewatch-docs --force --theme=both
cd ..

# publish to the CDN (videos + images), then make it live
make publish-cdn-videos publish-cdn-images
gh workflow run docs-kapi.yml --ref main   # pages-deploy.yml deploys on its success
```

The harness publish stage writes `<publishAs>-{light,dark}.webm` + `.jpg`
posters straight into `web/static/video/kapi/`.

## 2. bowrain docs (needs a running stack)

```bash
# Bring up the full local stack (server + worker + deps, keyless `demo` provider)
make -C bowrain stack-up-web        # serves SPA + API at http://localhost:8080
export BOWRAIN_BACKEND_URL=http://localhost:8080

# Seed the BowMart workspace + content and mint record tokens into harness/.env
# (device flow: the JWT is planted as the bowrain_session cookie by the recorder)
make harness-seed
```

`make harness-videos-staged` runs the whole bowrain pass unattended: stack up,
seed, record every screencast, stack down, then narrate and package offline.
The steps below are the same pass by hand.

### 2a. bowrain framed videos (harness)

```bash
cd harness
# collaboration is two genuine users; seed both first:
node scripts/seed-collaboration.mjs > /tmp/collab.json   # prints both tokens + project/item/locale
# export the env it printed (BOWRAIN_SESSION_TOKEN, BOWRAIN_PEER_TOKEN, …), then:
vpx tsx src/cli/run.ts bowrain-web-collaboration --force --theme=both \
  --docs-dir=../bowrain/web/docs/static/video/bowrain-web

for id in editor governance review correction-loop; do
  vpx tsx src/cli/run.ts bowrain-web-$id --force --theme=both \
    --docs-dir=../bowrain/web/docs/static/video/bowrain-web
done
for id in dashboard automations; do
  vpx tsx src/cli/run.ts bowrain-desktop-$id --force --theme=both \
    --docs-dir=../bowrain/web/docs/static/video/bowrain-desktop
done
for id in getting-started auth-and-workspaces; do
  vpx tsx src/cli/run.ts bowrain-cli-$id --force --theme=both \
    --docs-dir=../bowrain/web/docs/static/video/bowrain-cli
done
cd ..
```

### 2b. bowrain walkthrough scenes and screenshots

The walkthrough scenes under `bowrain/web/docs/scenes/<id>/` are committed
`.webm` files and the screenshot galleries under
`bowrain/web/docs/static/img/{web-app,bowrain}/{light,dark}/` are gitignored
CDN assets; neither has a recorder checked in. Re-record a scene or capture a
gallery by hand against the seeded stack (a Playwright script in the shape of
`harness/scripts/theme-shots.mjs`, which plants the session cookie and captures
the real app in light and dark), keep the file names the pages reference, and
commit the scene files.

### 2c. publish + deploy

```bash
make publish-cdn-bowrain-videos publish-cdn-bowrain-images
gh workflow run deploy-landing.yml --ref main   # Deploy Landing + Docs (bowrain.cloud)
```

`docs-bowrain.yml` builds pull-request previews only; production is
`deploy-landing.yml`.

## 3. Verify live

CloudFront edges can serve a stale object briefly after an overwrite.
**Verify by byte size, not HTTP 200**: compare the live `Content-Length` on the
CDN against the local file and re-publish until they match.

```bash
# example: compare one asset on the CDN
live=$(curl -sI "$DOCS_CDN_URL/kapi/video/kapi/kapi-desktop-projects-dark.webm" | awk '/content-length/{print $2}' | tr -d '\r')
local=$(stat -f%z web/static/video/kapi/kapi-desktop-projects-dark.webm)
echo "live=$live local=$local"
```

## Notes / footguns

- **One render at a time** keeps CPU sane; the harness already renders demos
  sequentially; don't fan them out.
- All recordings run against **real** backends (no mocks): real Keycloak/`demo`
  provider, real bowrain-server, a real PostgreSQL database.
- `ThemedVideo` (`@neokapi/docs-shared`) resolves `src` through `useBaseUrl`;
  always use root-absolute `/video/...` / `/img/...` paths in MDX, never bare.
- Every served `.webm` must be **bt709 / limited-range**; the harness publish
  step re-tags accordingly (Chrome's VP9 path is strict; Safari is lenient).
- bowrain videos carry the **Bowrain** brand lockup (logo + indigo wordmark);
  this is the `brand: bowrain` card brand in `harness/src/remotion/components/Cards.tsx`.

## Desktop/web render reliability (important)

The framed desktop/web videos embed a Playwright screencast that Remotion's
OffthreadVideo seeks into per beat. The rust compositor is fragile here:

- **Separate capture from render for bowrain.** Capture needs the Docker stack
  up (server + Playwright). Rendering needs *RAM*: Docker Desktop's VM holds
  ~15 GB even when containers are only stopped, which starves Remotion's
  compositor and causes intermittent `Could not extract frame from compositor:
  Request closed` / `delayRender timeout` failures. Workflow that works:
  1. Stack up, then `--only=capture` every bowrain demo (web + desktop).
  2. **Quit Docker Desktop entirely** (`osascript -e 'quit app "Docker"'`), not
     just `compose stop`, to free the VM RAM. Volumes persist on disk, so the
     seed survives a Docker quit/restart (only `down -v` wipes it).
  3. `--only=narrate,render,publish` every demo from cache.
- **Render at concurrency 1 for the long/animated demos.** Set
  `HARNESS_RENDER_CONCURRENCY=1`. Higher values multiply parallel video-proxy
  seeks and crash the compositor. `render.ts` retries once and caps concurrency
  (default 4); 1 is the reliable floor for 70 s+ screencasts.
- The screencast is re-encoded to dense-keyframe VP9 at capture time
  (`reencodeDenseKeyframes` in `record-desktop.ts`) so seeks decode a short GOP.
- **`--only=capture` skips narration**; always include `narrate` in the render
  pass (`--only=narrate,render,publish`) or the video ships silent with fallback
  timing.

## Auth / seeding for bowrain captures

- Device-flow JWTs are short-lived. Re-mint before a capture session; a stale
  token silently redirects the SPA to Keycloak and you capture a login page.
  Verify by extracting a frame (`ffmpeg -ss 3 -i screencast-dark.webm -vframes 1`).
- The session cookie (`bowrain_session`) is scoped to **`path: /api/`**; match
  that in any Playwright auth helper, not `/`.
- Workspace routes are bare-slug (`/<workspace>/...`): the content memory at
  `/<workspace>/memory`, terms at `/<workspace>/terms`.
  `harness/scripts/seed-collaboration.mjs` seeds project + file + content memory +
  terms + a second user and prints all the env the recorder needs.
