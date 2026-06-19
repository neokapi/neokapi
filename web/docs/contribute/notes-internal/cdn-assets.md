---
title: CDN asset offloading (Cloudflare R2)
description: How the large, immutable docs assets are served from an external CDN to keep the GitHub Pages deploy small and fast.
---

# CDN asset offloading (Cloudflare R2)

The documentation sites deploy to GitHub Pages by pushing the built static
output to `neokapi.github.io`. A few asset families are large, immutable, and
fetched at runtime rather than needed to render a page:

| Family | Approx. size | Where it's used |
|---|---|---|
| Playground WASM (`kapi-cli.wasm` + `.gz`, `kapi.wasm`, `pdfium.wasm`, `wasm_exec.js`) | ~125 MB | The Lab / KLF playground, PDF Lab |
| Vision ONNX models (PP-OCRv5 + PP-DocLayoutV3) | ~155 MB | The Vision Lab |
| Walkthrough videos (`.webm` light/dark + `.jpg` posters) | ~85 MB kapi / ~55 MB bowrain | `ThemedVideo` embeds |

Bundling these into the Pages artifact makes every deploy slow and forced an
awkward workaround for the ~132 MB layout model (split into <100 MB parts to fit
the GitHub Pages per-file limit). Offloading them to **Cloudflare R2** — free
tier (10 GB storage, free egress), served behind a custom domain with CDN
caching and CORS — removes the bulk from the Pages artifact and lets the models
ship whole.

## Opt-in by design

Everything here is **inert until configured**. The site reads the CDN origin
from a build-time env var, `DOCS_CDN_URL`, surfaced to the frontend as the
`cdnBaseUrl` Docusaurus customField. When it is empty — the default, and the
local-dev case — every asset resolves same-origin exactly as before. Nothing
changes until the `DOCS_CDN_URL` repo variable and R2 secrets are set.

The frontend routing lives in one shared helper, `@neokapi/docs-shared`'s
`cdn.ts` (`readCdnConfig` / `cdnEnabled` / `cdnHref`), consumed by:

- `packages/docs-shared/src/ThemedVideo.tsx` — video + poster sources
- `web/src/components/KapiPlayground/config.ts` — `wasmUrl` / `wasmExecUrl`
- `web/src/pages/lab/vision.tsx` — the Vision Lab `modelBase`

## Bucket layout

One bucket backs both sites; objects are scoped per-site to avoid collisions.
The WASM is versioned by commit sha so it can be cached immutably without a new
deploy serving a stale binary.

```
<bucket>/
  kapi/
    wasm/<git-sha>/{kapi-cli.wasm, kapi-cli.wasm.gz, kapi.wasm, pdfium.wasm, wasm_exec.js}
    models/vision/{ppocrv5_det.onnx, ppocrv5_rec.onnx, ppocrv5_dict.txt, ppdoclayoutv3.onnx}
    video/...            # .webm + .jpg posters, mirroring web/static/video/
  bowrain/
    video/...            # mirroring bowrain/web/docs/static/video/
```

Served URLs: `${DOCS_CDN_URL}/kapi/wasm/<sha>/kapi-cli.wasm`, etc.

## Cloudflare setup (one-time)

1. Put the domain (`bowrain.cloud`) on a Cloudflare zone (free plan is fine).
2. Create an R2 bucket and bind a **custom domain** (e.g. `cdn.bowrain.cloud`) to
   it — required for CDN caching, CORS, and custom headers; the `r2.dev` URL is
   rate-limited and dev-only. Disable the `r2.dev` public URL.
3. Add a cache rule on the custom domain: **Cache Everything**, honoring the
   origin `Cache-Control` (the publish script sets `immutable` on wasm/models and
   1-day on videos).
4. Apply the CORS policy (so browser `fetch()` of models/wasm works cross-origin):
   ```bash
   aws s3api put-bucket-cors --bucket "$R2_BUCKET" \
     --cors-configuration file://scripts/r2-cors.json \
     --endpoint-url "$R2_ENDPOINT"
   ```
5. **Lifecycle**: because each docs build writes wasm under a fresh `<git-sha>/`
   prefix, add an R2 lifecycle rule to expire objects under `kapi/wasm/` after
   ~30 days. The live site always references a recent sha (it redeploys on every
   push to `main`), so expiring old prefixes only affects long-stale deploys.

## Credentials

Create an R2 **S3-compatible API token** scoped to the bucket. Both the publish
script and CI read it from standard env vars:

| Env var | Value |
|---|---|
| `R2_BUCKET` | bucket name |
| `R2_ENDPOINT` | `https://<account-id>.r2.cloudflarestorage.com` |
| `AWS_ACCESS_KEY_ID` | R2 access key id |
| `AWS_SECRET_ACCESS_KEY` | R2 secret access key |

In GitHub: set `DOCS_CDN_URL`, `R2_BUCKET`, `R2_ENDPOINT` as **repository
variables**, and `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY` as **repository
secrets**.

## Publishing

WASM is rebuilt on every docs build, so **CI publishes it** automatically (the
`docs-kapi.yml` build job syncs to `kapi/wasm/<sha>/` and drops it from the
artifact when `DOCS_CDN_URL` is set). The other families are published
out-of-band, mirroring the old `docs-assets` release flow: the vision models are
pre-trained artifacts pinned in the `vision-models-v1` GitHub release (the
publish target just re-uploads them to R2 — rerun only when that release
changes), and the videos are produced on the desktop by the harness:

### Seeding R2 with what exists today

The videos and models already live in GitHub release artifacts, so no
desktop-produced source is needed to seed R2 — pull from the releases and sync,
from any machine with `gh` + the `aws` CLI + the `R2_*` env vars:

```bash
make seed-cdn   # fetch kapi+bowrain videos/images (docs-assets, bowrain-docs-assets)
                # + models (vision-models-v1) from the releases, then sync each to R2
```

**Order matters:** run `seed-cdn` (or the individual targets below) **before**
setting the `DOCS_CDN_URL` repo variable. Once the variable is set, CI stops
staging videos/models into the artifact, so the deployed site expects them on the
CDN — seed first or the live site 404s them. (WASM is the exception: CI builds
and publishes it in the same run that flips to the CDN, so it never needs
pre-seeding.)

### Individual targets

```bash
# one-time / when assets change (needs the R2_* env vars above + aws CLI):
make publish-cdn-vision-models     # whole ONNX models → kapi/models/vision/
make publish-cdn-videos            # web/static/video → kapi/video/  (run fetch-docs-assets first)
make publish-cdn-bowrain-videos    # bowrain videos   → bowrain/video/ (run fetch-bowrain-docs-assets first)
make publish-cdn-wasm              # optional manual wasm push (CI does this in deploy)
```

All of these call `scripts/publish-cdn-assets.sh <family>`, which sets the right
`Content-Type` and `Cache-Control` per family. The pre-gzipped `kapi-cli.wasm.gz`
is uploaded as an opaque blob with **no** `Content-Encoding` — the runtime
self-inflates it via `DecompressionStream`, so a `Content-Encoding: gzip` header
would make the browser double-inflate and fall back to the 71 MB raw binary.

## CI behavior

`docs-kapi.yml` / `docs-bowrain.yml` compute a job-level `CDN_URL` =
`DOCS_CDN_URL` **on pushes only** (PR previews always stay same-origin, to avoid
R2 churn and secret dependence). When `CDN_URL` is set, the video- and
model-staging steps are skipped, the wasm is synced to R2 and removed from the
artifact, and the site is built with `DOCS_CDN_URL` + `DOCS_CDN_VERSION`
(= commit sha) so it points at the CDN.
