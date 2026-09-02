---
sidebar_position: 1
title: CDN asset offloading (S3 + CloudFront)
description: How the large, immutable docs assets are served from the S3 + CloudFront CDN to keep the GitHub Pages deploy small and fast.
---

# CDN asset offloading (S3 + CloudFront)

The documentation sites deploy to GitHub Pages by pushing the built static
output to `neokapi.github.io`. A few asset families are large, immutable, and
fetched at runtime rather than needed to render a page:

| Family | Approx. size | Where it's used |
|---|---|---|
| Playground WASM (`kapi-cli.wasm` + `.gz`, `kapi.wasm`, `pdfium.wasm`, `wasm_exec.js`) | ~125 MB | The Lab / KBF playground, PDF Lab |
| Vision ONNX models (PP-OCRv5 + PP-DocLayoutV3) | ~155 MB | The Vision Lab |
| Walkthrough videos (`.webm` light/dark + `.jpg` posters) | ~85 MB kapi / ~55 MB bowrain | `ThemedVideo` embeds |

Bundling these into the Pages artifact makes every deploy slow and forced an
awkward workaround for the ~132 MB layout model (split into sub-100 MB parts to
fit the GitHub Pages per-file limit). Offloading them to an **S3 origin fronted
by CloudFront** (`cdn.<domain>`) removes the bulk from the Pages artifact and
lets the models ship whole.

The CDN bucket + distribution are provisioned by bowrain-infra (`modules/cdn`,
instantiated in the `50-edge` layer); CORS and the immutable cache behavior live
there, not in this repo.

## Opt-in by design

Everything here is **inert until configured**. The site reads the CDN origin
from a build-time env var, `DOCS_CDN_URL`, surfaced to the frontend as the
`cdnBaseUrl` Docusaurus customField. When it is empty (the default, and the
local-dev case) every asset resolves same-origin exactly as before. Nothing
changes until the `DOCS_CDN_URL` repo variable is set.

The frontend routing lives in one shared helper, `@neokapi/docs-shared`'s
`cdn.ts` (`readCdnConfig` / `cdnEnabled` / `cdnHref`), consumed by:

- `packages/docs-shared/src/ThemedVideo.tsx`: video + poster sources
- `web/src/components/KapiPlayground/config.ts`: `wasmUrl` / `wasmExecUrl`
- `web/src/pages/lab/vision.tsx`: the Vision Lab `modelBase`

## Bucket layout

One bucket backs both sites; objects are scoped per-site to avoid collisions.
The WASM is versioned by commit sha so it can be cached immutably without a new
deploy serving a stale binary.

```
<bucket>/
  kapi/
    wasm/<git-sha>/{kapi-cli.wasm, kapi-cli.wasm.gz, kapi.wasm, pdfium.wasm, wasm_exec.js}
    models/vision/<version>/{ppocrv5_det.onnx, ppocrv5_rec.onnx, ppocrv5_dict.txt, ppdoclayoutv3.onnx}
    icu/<icu-version>/icu_capi.wasm    # ICU4X (Segmentation Lab), served application/wasm
    img/...              # screenshots referenced by ThemedImage
    video/...            # .webm + .jpg posters, mirroring web/static/video/
  bowrain/
    img/...              # mirroring bowrain/web/docs/static/img/
    video/...            # mirroring bowrain/web/docs/static/video/
```

Served URLs: `${DOCS_CDN_URL}/kapi/wasm/<sha>/kapi-cli.wasm`, etc.

## Credentials

The CDN origin is a private S3 bucket; writes use the AWS credential chain, with
**no static access keys**:

- **CI** (`docs-kapi.yml` on push to main): the GitHub Actions **OIDC deploy
  role** (`AWS_DEPLOY_ROLE_ARN`), whose trust is pinned to `main` / the protected
  `prod` environment. bowrain-infra's `deploy-iam` module scopes it to
  `s3:Put/Get/Delete/ListBucket` on the CDN bucket plus
  `cloudfront:CreateInvalidation` on its distribution. PRs cannot assume it.
- **Locally** (desktop publish of videos/models/images): an `aws sso login`
  profile with write access to the bucket.

The publish script reads only:

| Env var | Value |
|---|---|
| `CDN_BUCKET` | S3 CDN origin bucket (`bowrain-<env>-cdn-<region>-<acct>`) |
| `AWS_REGION` | bucket region (default `eu-north-1`) |
| AWS credentials | from the environment (OIDC role in CI, SSO profile locally) |

In GitHub: `DOCS_CDN_URL`, `CDN_BUCKET`, `AWS_DEPLOY_ROLE_ARN`, and `AWS_REGION`
are **repository variables**. No repository secrets are needed for the CDN.

## Publishing

WASM is rebuilt on every docs build, so **CI publishes it** automatically, but
only on **push to main**: the `docs-kapi.yml` build job assumes the OIDC role,
syncs `kapi/wasm/<sha>/` to the CDN, and drops it from the artifact. PRs cannot
assume the deploy role, so PR previews serve their own wasm **same-origin** (the
version is unset → `KapiPlayground/config.ts` falls back), while videos, models,
and images still resolve from the CDN by URL.

The other families are published out-of-band from the desktop, where the harness
renders the videos/screenshots and `make fetch-vision-models` stages the model
set (the vision models are pinned in the `vision-models-v1` GitHub release, so
the publish target just re-uploads them). Needs the `aws` CLI (an `aws sso login`
session) + `CDN_BUCKET`:

```bash
make publish-cdn-all   # videos + images + vision models, kapi & bowrain → CDN
```

**Order matters:** publish (or run the individual targets below) **before**
setting the `DOCS_CDN_URL` repo variable. Once the variable is set, CI builds
the sites pointing at the CDN (for push and same-repo PRs), so the deployed and
preview sites expect those assets on the CDN, so publish first or they 404. (WASM
is the exception: CI builds and publishes it, versioned by sha, in the same
push-to-main run.)

### Individual targets

```bash
# when assets change (needs CDN_BUCKET + an aws SSO session):
make publish-cdn-vision-models     # ONNX models → kapi/models/vision/<web/models.version>/
make publish-cdn-icu               # ICU4X seg wasm   → kapi/icu/<ver>/icu_capi.wasm
make publish-cdn-videos            # web/static/video → kapi/video/
make publish-cdn-bowrain-videos    # bowrain videos  → bowrain/video/
make publish-cdn-images            # web/static/img  → kapi/img/
make publish-cdn-bowrain-images    # bowrain images  → bowrain/img/
make publish-cdn-wasm              # optional manual wasm push (CI does this on push to main)
```

The vision model set is **versioned**: `kapi/models/vision/<version>/`, with the
version pinned in the committed `web/models.version`. To ship a new model set,
publish it under a new version and bump that file. A PR doing so previews the
new models automatically (the Vision Lab reads the version from the build).

All of these call `scripts/publish-cdn-assets.sh <family>`, which sets the right
`Content-Type` and `Cache-Control` per family. The pre-gzipped `kapi-cli.wasm.gz`
is uploaded as an opaque `application/wasm` blob with **no** `Content-Encoding`.
The runtime self-inflates it via `DecompressionStream`, so a
`Content-Encoding: gzip` header would make the browser double-inflate and fall
back to the ~76 MB raw binary.

## CI behavior

`docs-kapi.yml` / `docs-bowrain.yml` compute a job-level `CDN_URL` =
`DOCS_CDN_URL` for **push and same-repo PRs** (fork PRs stay same-origin). When
`CDN_URL` is set, the video/model/image assets resolve from the CDN by URL. WASM
is narrower: only **push to main** publishes it (via the OIDC role) and sets
`DOCS_CDN_VERSION` (= commit sha); on PRs `DOCS_CDN_VERSION` is empty, so the
playground serves wasm same-origin while the other CDN assets are still used.
