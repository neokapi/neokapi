# neokapi PR previews (AWS S3 + CloudFront)

PR previews for the docs sites, the bowrain landing, and the Storybooks are
served from an **AWS S3 bucket behind a CloudFront distribution** at
`preview.bowrain.cloud`, instead of being committed into the `neokapi.github.io`
org Pages repo.

## Why

Previews used to be pushed into the Pages repo under `web/prs/<N>/` and
`storybook/prs/<N>/`. Each preview carries the full wasm (~73 MB) + vision model
(~132 MB) + video payload, so the repo grew to multiple GB — and pruning on PR
close only deleted files from the tip, leaving every binary in git history
forever.

S3 fixes all of it:

- **No per-file size limit** — GitHub Pages / Cloudflare Workers Static Assets
  cap at 25 MiB/file, which one Storybook `ort-wasm` file already exceeds. S3
  objects have no such cap.
- **Delete actually frees storage** — prune on PR close is an `aws s3 rm`.
- **Zero build changes** — previews are served from the host root, so the baked
  absolute base URLs (`/web/prs/<N>/neokapi/docs/`, `/storybook/prs/<N>/kapi/`)
  resolve exactly as they did on GitHub Pages.

> **History:** previews briefly lived in a Cloudflare R2 bucket fronted by a
> Worker (`deploy/preview-worker`, now removed). That host broke when the
> `bowrain.cloud` zone moved off Cloudflare to Route53 — Cloudflare stopped being
> authoritative for `preview.bowrain.cloud`, so the Worker's custom-domain DNS +
> TLS vanished. Previews now ride the same S3+CloudFront stack as the CDN
> (`cdn.bowrain.cloud`), the SPAs, and the apex landing — nothing about the
> platform depends on Cloudflare anymore.

A bare S3 origin can't serve a static *site* (it returns the object at the exact
key or 404s — no `index.html` resolution for a `trailingSlash: false` Docusaurus
build). A **CloudFront Function** (viewer-request) does the clean-URL resolution:
a trailing-slash path → `…/index.html`, an extensionless path → `….html`, an
extension-bearing path → served as-is. See `modules/preview-site/main.tf` in
`bowrain-infra`.

## How it fits together

```
docs-kapi / docs-bowrain / storybook-preview / web-landing   (build, on PR)
        │  upload artifacts
        ▼
pages-deploy.yml  ── is_pr? ──┬── prod → git commit+push → neokapi.github.io
                              └── PR   → assume PREVIEW_DEPLOY_ROLE_ARN (OIDC)
                                          → scripts/publish-pr-preview.sh
                                              aws s3 sync → s3://<PREVIEW_BUCKET>/{web,storybook}/prs/<N>/
                                              aws cloudfront create-invalidation /…/prs/<N>/*
                                                                    │
                                          CloudFront + Function  ◄──┘  serves at $DOCS_PREVIEW_URL
prune-pr-preview.yml (on PR close) → assume PREVIEW_DEPLOY_ROLE_ARN
                                     → scripts/prune-pr-preview.sh → aws s3 rm …/prs/<N>/
```

## Infrastructure (bowrain-infra)

- **`modules/preview-site`** — the private S3 origin bucket (OAC-gated),
  CloudFront distribution for `preview.<domain>`, ACM cert (us-east-1), Route53
  alias, and the clean-URL CloudFront Function. Instantiated in
  `live/prod/eu-north-1/50-edge` (gated by `preview_dns_cutover`, set `true`).
- **`modules/deploy-iam`** — a dedicated low-privilege `preview` OIDC role,
  trusted for `ref:refs/heads/main` (the publish, a `workflow_run` job) and
  `pull_request` (the prune), scoped to **only** the preview bucket's objects and
  the preview distribution's invalidations. It cannot touch anything else. Wired
  in `live/prod/eu-north-1/60-ops`.

After `terraform apply`, read the outputs from the `50-edge` / `60-ops` layers:

```bash
cd bowrain-infra/live/prod/eu-north-1/50-edge && terraform output preview_bucket_name preview_distribution_id
cd ../60-ops && terraform output preview_deploy_role_arn
```

## Repo variables (neokapi/neokapi)

No secrets — auth is GitHub OIDC. Set these repo **variables** to the Terraform
outputs above:

```bash
gh variable set PREVIEW_BUCKET          --body "<preview_bucket_name>"      --repo neokapi/neokapi
gh variable set PREVIEW_DISTRIBUTION_ID --body "<preview_distribution_id>"  --repo neokapi/neokapi
gh variable set PREVIEW_DEPLOY_ROLE_ARN --body "<preview_deploy_role_arn>"  --repo neokapi/neokapi
# AWS_REGION and DOCS_PREVIEW_URL already exist (eu-north-1 / https://preview.bowrain.cloud).
```

The old `R2_BUCKET`, `R2_ENDPOINT`, `R2_ACCESS_KEY_ID`, `R2_SECRET_ACCESS_KEY`
are no longer read by the preview workflows and can be removed once no other
workflow uses them.

## Notes

- Content types come from the S3 objects. `aws s3 sync` guesses every text/image
  type correctly; `scripts/publish-pr-preview.sh` fixes only WebAssembly
  (`.wasm` / `.wasm.gz` → `application/wasm`) with a server-side self-copy. The
  pre-gzipped `*.wasm.gz` is served as an opaque `application/wasm` blob with
  **no** `Content-Encoding` — the runtime self-inflates via `DecompressionStream`
  (same contract as `publish-cdn-assets.sh` / GitHub Pages).
- Each publish/prune invalidates only the PR's paths, so a re-pushed preview is
  served fresh immediately.
- Fork PRs get no preview — a fork workflow can't assume the OIDC role. Both
  workflows also gate on same-repo PRs. Unchanged from the R2 era.
- Stale previews whose PR never fired the prune are swept by a 30-day S3
  lifecycle rule.
