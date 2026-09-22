# Documentation release channels

Both documentation sites ship on two channels. Production describes the latest
GA release; a second channel describes what main contains.

| Site | Stable | Next | Host |
|---|---|---|---|
| kapi docs (`web/`) | `https://neokapi.github.io/` | `https://neokapi.github.io/next/` | the `neokapi.github.io` org Pages repository |
| bowrain docs (`bowrain/web/docs/`) | `https://bowrain.cloud/docs/` | `https://bowrain.cloud/docs/next/` | one S3 origin behind CloudFront |

The stable channel is built from the **`docs/stable`** branch. A GA release moves
that branch to its tag; a person cherry-picks a docs fix onto it between
releases. The next channel is built from every push to `main` and carries a
banner naming the release in progress, so a reader knows which version the page
describes before following it.

One branch serves both sites. The two products version together, their GA tags
have so far been cut on the same commit, and both docs builds read the same
`packages/`, the same pseudo-locale action and the same recipe. Two branches
would carry two copies of that shared tree and let the two stable sites drift
against it. Split the branch in two (`docs/stable-kapi`, `docs/stable-bowrain`)
on the day a GA tag of one product lands on a commit that is not a GA of the
other.

The bowrain **landing** at `https://bowrain.cloud/` keeps deploying from `main`.
It is the product's front page rather than release documentation.

## What builds what

| Ref | Workflow | Channel | Where it lands |
|---|---|---|---|
| `main` | `docs-kapi.yml` | next | artifact `kapi-site-next` → `pages-deploy.yml` → `/next/` |
| `docs/stable` | `docs-kapi.yml` | stable | artifact `kapi-site` → `pages-deploy.yml` → the repository root |
| a pull request | `docs-kapi.yml` | next | artifact `kapi-site` → the preview slot `/web/prs/<N>/neokapi/docs/` |
| `main` | `deploy-landing.yml` | next | landing at `s3://$LANDING_BUCKET/`, docs at `…/docs/next/` |
| `docs/stable` | `deploy-landing.yml` | stable | docs at `s3://$LANDING_BUCKET/docs/` |

Each workflow resolves the channel from the ref it builds and takes a
`workflow_dispatch` input named `ref`. Dispatching from `main` with
`ref: docs/stable` checks out that branch and publishes production, so the
workflow definitions live on `main` alone and a `docs/stable` cut from an older
tag still deploys with the current steps.

Two safeguards keep the channels apart:

- **kapi.** `pages-deploy.yml` routes by artifact name. `kapi-site` is slotted at
  the repository root and `kapi-site-next` under `/next/`, and the root clean
  keeps `next` along with `web` and `storybook`.
- **bowrain.** Each `aws s3 sync --delete` is scoped to its own prefix, so it
  prunes that prefix alone. The landing sync excludes `docs/*`, the stable docs
  sync excludes `next/*` and `next.html`, and the next docs sync targets
  `docs/next` outright.

### The two channel-root files on the apex

The apex CloudFront router (`modules/apex-site` in `bowrain-infra`) resolves a
clean URL under `/docs/` by appending `.html`, which suits a site built with
`trailingSlash: false`, and answers the directory roots it knows (`/docs/` and
`/docs/<locale>/`) from an explicit rule. `/docs/next/` and `/docs/next/qps/`
are roots it does not know, so it asks S3 for `/docs/next.html` and
`/docs/next/qps.html`. `deploy-landing.yml` writes those two files while
assembling the site tree, and the channel resolves with no change at the edge.
Every page below them already resolves, because appending `.html` is exactly
right for them.

Teaching the router about a channel segment would take a `docs_channels`
variable interpolated into the function body, a generalized root check, and a
`localeAt` that skips the channel segment before matching a locale. The two
files cost less and stay in this repository.

## Versions and download links

The banner and the switch name two versions: the one the channel describes and
the latest GA. CI resolves both from the published releases (`gh release list`,
family `v[0-9]*` for kapi and `bowrain-v[0-9]*` for bowrain) and passes them as
`DOCS_CHANNEL_VERSION` and `DOCS_STABLE_VERSION`, so the docs checkout stays
shallow. A local build with neither variable falls back to `git describe --tags`
in the site config, and a build that finds no version leaves the banner and the
switch off the page rather than naming a blank one.

The stable site's installation page is spliced at build time:
`docs-kapi.yml` runs `scripts/update-website-downloads.sh <tag>` against the
release being deployed and commits nothing. The rolling auto-PR that
`release-docs.yml` opens still targets `main`, which keeps the next channel's
committed page current.

Build time is what the production page reads for a reason. The auto-PR route
depends on someone merging it, and when Actions lost permission to open pull
requests the branch `bot/release-downloads-kapi` carried a current page while
the published install page advertised a release eleven candidates old. A splice
that reads the release cannot drift from it.

The script refuses when a platform has no asset on the release yet, which is the
state of every Windows row until the binaries are signed out of band the morning
after a release (`scripts/publish-windows-signed.sh`). The page then keeps the
links it carries and the step records a warning. Re-run the deploy after signing
to pick the rest up.

## A GA release moves the branch

`release-docs.yml`'s `stable-branch` job runs on a GA tag of either product
(`v1.2.0`, `bowrain-v1.2.0`) and does two things:

1. Fast-forwards `docs/stable` to the tag's commit, with `contents: write` and
   `GITHUB_TOKEN`. The `docs/*` refs carry no ruleset, so no deploy key is
   involved. The push is refused when the branch tip is not an ancestor of the
   tag, which means someone put work on the branch that the tag would discard.
2. Dispatches `docs-kapi.yml` and `deploy-landing.yml` from the default branch
   with `ref: docs/stable`. `workflow_dispatch` is one of the two events
   `GITHUB_TOKEN` may raise that still start a run.

A prerelease leaves the branch alone. Release candidates are what `/next/`
already serves.

The job runs on the release event and on the `workflow_dispatch` the Windows
signing step already fires (`scripts/publish-windows-signed.sh` runs
`gh workflow run release-docs.yml -f tag=<tag>` once the signed assets are on
the release). Two independent triggers reach it, and the second one also
refreshes the download links now that the Windows rows resolve. Both are
idempotent: a branch already at the tag exits early.

## Backport a docs fix to the stable site

Land the fix on `main` first, so the next channel and the following release
carry it, then cherry-pick it onto the branch:

```bash
git fetch origin
git checkout -B docs/stable origin/docs/stable
git cherry-pick <sha>          # the commit as it landed on main
git push origin docs/stable
```

The push starts `docs-kapi.yml` and `deploy-landing.yml` on that branch when the
files it touches match their path filters, which a `web/**` or
`bowrain/web/docs/**` change does. Redeploy by hand otherwise.

Keep the cherry-pick to documentation. The branch is a build input for two
static sites; anything else on it ships nowhere and makes the next
fast-forward fail.

## Redeploy production by hand

```bash
gh workflow run docs-kapi.yml      --ref main -f ref=docs/stable
gh workflow run deploy-landing.yml --ref main -f ref=docs/stable
```

Both read `docs/stable` as it stands. To publish a different release, move the
branch first:

```bash
git push origin "$(git rev-list -n1 v1.2.0):refs/heads/docs/stable"
```

Rebuilding the next channel is a dispatch with no input:

```bash
gh workflow run docs-kapi.yml      --ref main
gh workflow run deploy-landing.yml --ref main
```

## Building a channel locally

```bash
# kapi docs, next channel (the default)
make docs-build-prod NEOKAPI_DOCS_BASE=/next/
# kapi docs, stable channel
cd web && DOCS_BASE_URL=/ DOCS_CHANNEL=stable vpx docusaurus build --locale en

# bowrain docs, next channel
make bowrain-docs-build-prod BOWRAIN_DOCS_BASE=/docs/next/
# bowrain docs, stable channel
make bowrain-docs-build-prod BOWRAIN_DOCS_BASE=/docs/ DOCS_CHANNEL=stable
```

`DOCS_CHANNEL` defaults to `next`, so a local build and a PR preview both carry
the banner and a reviewer sees it without arranging anything.
