---
title: Dogfood sync — neokapi as bowrain.cloud customer #1
description: How to activate (and deactivate) the customer-#1 dogfood that syncs neokapi's own localization to Bowrain.
---

# Dogfood sync (customer #1)

neokapi localizes its own surfaces with kapi (the root `kapi.yaml` recipe).
The next step in the dogfood is to become the **first real workspace on
Bowrain**: push those surfaces to a bowrain-server, let the nb drafts land in
the governed review queue, and round-trip approved segments back into
`l10n/tm/*.klftm`. This exercises the paid product — the review loop, the
signup/claim funnel, connectors — on a real, continuously changing corpus
before any external user touches it (roadmap epic 017-F, decision D2).

This is shipped **inert**: an active `server:` block in the repo-root recipe
would make *every* in-repo kapi invocation bowrain-connected, which the
isolation contract (see [CLAUDE.md, "Dogfooding kapi"]) does not want by
default. So the recipe carries a commented `server:` template and
`.github/workflows/dogfood-sync.yml` is gated off until you arm it.

## Sequencing (decision D2)

Production `bowrain.cloud` does not exist until epic 002 (AWS prod) lands.
Dogfood against the local compose stack or `dev.bowrain.cloud` first, then flip
the recipe's `url:` to production the day epic 002's `cloud-e2e` passes — the
flip doubles as the production acceptance test.

## Activate

1. **Sign up through the public funnel** (dogfoods epic 007). Create the
   `neokapi` workspace, claim the project, and copy its compound project URL
   (`https://<server>/<workspace>/<project-id>`).
2. **Uncomment the `server:` block** at the foot of `kapi.yaml` and set
   `url:` to that compound URL. Commit it. From this point `kapi status` /
   `kapi push` operate against the server (this is the one in-repo invocation
   that legitimately binds the root recipe).
3. **Add the repo secret** `BOWRAIN_AUTH_TOKEN` — from `kapi auth token` after
   signing in locally, or a workspace CI token.
4. **Set the repo variable** `DOGFOOD_SYNC_ENABLED = true`. The nightly
   `Dogfood Sync` workflow (03:00 UTC, or manual `workflow_dispatch`) now runs
   `kapi up` + `kapi push` and writes a status summary. Use the dispatch
   `dry_run` input for a plan-only (`kapi up --explain`) pass first.
5. **Confirm plugin discovery** — the workflow builds and installs the
   `kapi-bowrain` plugin; verify `kapi plugins list` shows it in the run log
   (the one step that can't be tested until the server exists).

## Deactivate

Unset `DOGFOOD_SYNC_ENABLED` (or set it to anything but `true`). The workflow
skips at the job gate — no secret is read and no content moves. Re-comment the
`server:` block to return the recipe to a pure local project. Fully reversible.

## The loop, once live

`kapi push` sends each surface; `translate-ai` nb output lands as drafts in the
governed review queue (epic 006); approved segments come back via `kapi pull` →
`kapi extract/merge` → a refreshed `l10n/tm/*.klftm` commit. The nightly job
keeps the server current; human review closes the loop.

[CLAUDE.md, "Dogfooding kapi"]: the in-repo isolation contract.
