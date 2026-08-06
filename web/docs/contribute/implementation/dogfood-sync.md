---
title: Dogfood sync — neokapi as bowrain.cloud customer #1
description: How the nightly convergence of neokapi's own multilingual content against Bowrain is wired, armed, and disarmed.
---

# Dogfood sync (customer #1)

neokapi translates its own surfaces with kapi, through the root `kapi.yaml`
recipe. Customer #1 is the next step: neokapi as the first real workspace on
Bowrain, with nb drafts landing in the governed review queue and approved
wording round-tripping back into the committed content-memory seeds. It
exercises the paid product — the review loop, the signup and claim funnel, the
connectors — on a real, continuously changing corpus before any external user
touches it (roadmap epic 017-F, decision D2).

## The workflow is deliberately unremarkable

`.github/workflows/dogfood-sync.yml` is the workflow the product tells everyone
else to write, run against the repository that builds the product:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: bowrain
    auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
- run: make l10n-extract
- uses: neokapi/kapi-action@v1
  with:
    command: up
```

`setup-kapi` installs the **released** CLI and the bowrain plugin from the
registry rather than building either from the checkout, so a green nightly is
evidence about the shipped product and not about the working tree.
`kapi-action` runs the convergence verb and commits what came back.

The one step a customer would not write is `make l10n-extract`. Five of the
recipe's collections declare catalogs that the neokapi-i18n extractor produces
from React source; they are build artifacts, gitignored by design, so a run from
a clean checkout would carry nothing for them. It extracts the **source** side
only — the target side is the loop's job, and target-language drift must never
gate a push. See [the dogfood loop in CI][l10n-ci] for why that step cannot move
into the recipe.

This is the one in-repo kapi invocation that binds the root recipe. Every other
one isolates itself per the contract in CLAUDE.md, so tests, make targets and
the video recorders are unaffected by the connection.

## Arming and disarming

The workflow is inert until two settings exist, and neither lives in this
repository's source:

1. **Repo secret `BOWRAIN_AUTH_TOKEN`** — a `bwt_` workspace API token, minted
   with `kapi auth token create` after signing in. `setup-kapi` exports it as
   the CLI's auth for the run.
2. **Repo variable `DOGFOOD_SYNC_ENABLED = true`** — the single gate. Every job
   is conditioned on it, so until it is set the scheduled run and any manual
   dispatch evaluate the gate and skip: no secret is read and no content moves.

The recipe's `server:` block names the compound project URL
(`https://<server>/<workspace>/<project-id>`) and carries `converge: manual`, so
a push moves content but never auto-starts a server run.

To disarm: unset `DOGFOOD_SYNC_ENABLED` (or set it to anything but `true`).
Re-comment the `server:` block to return the recipe to a pure local project.
Fully reversible.

## Why convergence is started deliberately

The first on-push run tried to AI-draft the entire untranslated corpus against
an empty server content memory — no leverage, everything AI — and stalled on the
workspace credit allowance. `converge: manual` is the consequence: transport
stays pure, and the loop is started deliberately, scoped, once the server
content memory is seeded or a bring-your-own AI key is configured (which burns
no credits).

The nightly therefore runs `kapi up` against a recipe that will not auto-fan-out
across an unseeded corpus. Use the workflow's `plan` dispatch input for a dry
run first: it reports pending work, content-memory leverage and a token estimate
without writing anything or calling a provider.

## The loop, once live

`kapi up` pushes each surface, converges on the server against the workspace's
shared content memory and terms, and pulls the produced targets back; nb drafts
land as review-queue work (epic 006), and approved wording returns to the
committed seeds through an ordinary commit. The nightly keeps the server
current; human review closes the loop.

[l10n-ci]: https://github.com/neokapi/neokapi/blob/main/docs/internals/l10n-ci.md
