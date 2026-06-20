# neokapi PR previews (Cloudflare Worker + R2)

PR previews for the docs sites and Storybooks are served from **Cloudflare R2**
by this Worker, instead of being committed into the `neokapi.github.io` org
Pages repo.

## Why

Previews used to be pushed into the Pages repo under `web/prs/<N>/` and
`storybook/prs/<N>/`. Each preview carries the full wasm (~73 MB) + vision model
(~132 MB) + video payload, so the repo grew to **~2.4 GB working tree / ~0.6 GB
`.git`** — and pruning on PR close only deleted files from the tip, leaving
every binary in git history forever.

R2 fixes all of it:

- **No per-file size limit** — Cloudflare Pages / Workers Static Assets cap at
  25 MiB/file, which one Storybook `ort-wasm` file (25.02 MiB) already exceeds.
  R2 objects have no such cap.
- **Delete actually frees storage** — prune on PR close is an `aws s3 rm`.
- **Zero build changes** — previews are served from the host root, so the baked
  absolute base URLs (`/web/prs/<N>/neokapi/docs/`, `/storybook/prs/<N>/kapi/`)
  resolve exactly as they did on GitHub Pages.

R2 alone can't serve a static *site* (a bare custom domain returns the object at
the exact key or 404s — no `index.html` resolution for `trailingSlash: false`
Docusaurus). This Worker is the thin routing layer that does clean-URL
resolution. See `src/index.ts`.

## How it fits together

```
docs-kapi / docs-bowrain / storybook-preview / web-landing   (build, on PR)
        │  upload artifacts
        ▼
pages-deploy.yml  ── is_pr? ──┬── prod → git commit+push → neokapi.github.io
                              └── PR   → scripts/publish-pr-preview.sh
                                            aws s3 sync → s3://neokapi-docs-cdn/previews/{web,storybook}/prs/<N>/
                                                                    │
                                            this Worker (R2 binding) ◄┘  serves at $DOCS_PREVIEW_URL
prune-pr-preview.yml (on PR close) → scripts/prune-pr-preview.sh → aws s3 rm previews/.../prs/<N>/
```

## One-time setup

Repo vars/secrets `R2_BUCKET`, `R2_ENDPOINT`, `R2_ACCESS_KEY_ID`,
`R2_SECRET_ACCESS_KEY` already exist (used by `publish-cdn-assets.sh`). What's
new:

1. **Deploy the Worker** (account id is the one in `R2_ENDPOINT`:
   `31ae84ca17450245ed9799b73700677a`):

   ```bash
   cd deploy/preview-worker
   npm install
   export CLOUDFLARE_ACCOUNT_ID=31ae84ca17450245ed9799b73700677a
   export CLOUDFLARE_API_TOKEN=…        # see token scopes below
   npx wrangler deploy
   ```

   `wrangler.jsonc` declares the bucket binding (`neokapi-docs-cdn`) and a
   `custom_domain` route for `preview.bowrain.cloud` — wrangler provisions the
   DNS record + TLS cert on the `bowrain.cloud` zone automatically (no manual
   DNS). If you pick a different host, update the route there and
   `DOCS_PREVIEW_URL` below.

   API token scopes (create at dash → My Profile → API Tokens; the "Edit
   Cloudflare Workers" template covers most): Account · Workers Scripts:Edit,
   Workers R2 Storage:Edit; Zone (bowrain.cloud) · Workers Routes:Edit,
   DNS:Edit, SSL and Certificates:Edit. The DNS + SSL scopes are what let
   `custom_domain` provision the hostname; without them, deploy the Worker first
   and instead add the custom domain in the dashboard (Workers & Pages → the
   worker → Settings → Domains & Routes → Add → Custom domain).

2. **Repo variable** `DOCS_PREVIEW_URL` = `https://preview.bowrain.cloud`
   (the PR-comment links and the deploy step read it; it falls back to that
   default if unset).

   ```bash
   gh variable set DOCS_PREVIEW_URL --body "https://preview.bowrain.cloud" --repo neokapi/neokapi
   ```

That's it — no new secret. The deploy/prune workflows reuse the existing R2
credentials.

## Local development

```bash
npm install
npm run dev     # wrangler dev, against the real R2 bucket (read-only path)
npm run tail    # live logs from the deployed Worker
```

## Notes

- Content types are set authoritatively by the Worker from the file extension
  (`application/wasm` for `.wasm`, etc.), so they don't depend on what
  `aws s3 sync` guessed.
- Pre-gzipped wasm (`*.wasm.gz`) is served as an opaque blob with **no**
  `Content-Encoding`; the runtime self-inflates via `DecompressionStream`, the
  same contract as `publish-cdn-assets.sh` and GitHub Pages.
- Fork PRs get no preview (the deploy needs the R2 secret) — unchanged from the
  previous git-based flow, which also gated on same-repo PRs.
