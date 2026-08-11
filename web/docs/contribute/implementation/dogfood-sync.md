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
`kapi-action` runs the convergence verb and reports; delivering what came back
is the workflow's own business, below.

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

The recipe's `bowrain:` block names the compound project URL
(`https://<server>/<workspace>/<project-id>`) and carries `converge: manual`, so
a push moves content but never auto-starts a server run.

To disarm: unset `DOGFOOD_SYNC_ENABLED` (or set it to anything but `true`).
Re-comment the `bowrain:` block to return the recipe to a pure local project.
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
land as review-queue work (epic 006), and approved wording returns as decisions
the project commits under `.kapi/state/`. The nightly keeps the server current;
human review closes the loop.

## The return leg runs in the nightly

The pull stages every approved decision the server's ledger holds into the
project's working store. Staging is not publishing: `kapi commit` is the only
door from there into the committed record under `.kapi/state/` that git tracks,
and recording a decision stays a separate act from putting it in the record a
reviewer reads. A run of automated approvals therefore does not reach the
tracked record on its own.

So the nightly runs it, between `kapi up` and the backing gate:

```yaml
- name: Commit the decisions the run brought home
  run: kapi commit
```

Order is the whole point. The gate below asks whether the committed context
moved with the artifacts the run produced; the artifacts came from wording the
server approved, and this step is what puts that approval under `.kapi/`.
Running the gate first would refuse the run for a shard the next line was about
to write. Nothing staged is a no-op that exits 0.

The terminology half of the return leg does not run yet. Approved concept
decisions arrive from the server into the project's rebuildable terms store and
never reach the authored `.kapi/terms.json`, and no command projects them there
without replacing the whole file — which would delete authored concepts the
workspace has not adopted. It waits on a merging projection; see
[issue #1850](https://github.com/neokapi/neokapi/issues/1850).

## Delivery is gated on the committed context

Everything the loop writes — target catalogs, narration sidecars, runtime
dictionaries — is rebuilt from `.kapi/`, the committed context graph, every time
the surfaces regenerate. Wording that reaches the tree without a matching
`.kapi/` change therefore survives until the next regeneration and is then
overwritten from a context that never learned it. Nothing fails while that
happens: the run that produced the wording was green, and the run that erased it
was green too.

`scripts/check-sync-backed.sh` runs between `kapi up` and the delivery step and
refuses such a run, naming every file:

```
check-sync-backed: REFUSED — this run must not be committed

The run rewrote artifacts the loop owns while the committed context under
.kapi/ did not move. Regeneration rebuilds these files from .kapi/, so
committing them delivers wording that the next regeneration erases.

Refused — derived, nothing behind it:
  M  bowrain/mailer/subjects/nb.json
  A  harness/demos/08-mcp-tools/demo.nb.yaml
```

It classifies the whole working tree, using git for every path match: changes
under `.kapi/` are **backing** (a decision shard, terms, a memory seed, the
voice profile, a profile), changes matching `make l10n-derived-paths` are
**derived** — the same set `l10n-verify` regenerates — and anything else is
**foreign**. Derived changes need a backing change in the same run; a foreign
change is refused whether or not the context moved, because a convergence run
does not author source: a source edit appearing in one is a symptom, and the
gate is the layer that says so rather than letting it reach a diff nobody
expected it in.

Backing is asked of the run as a whole rather than file by file: no committed
mapping ties a seed or a shard to the artifact it feeds — seed filenames are
deliberately independent of collection names — so a per-file correspondence
would be invented rather than read.

A red nightly here is pending work, not a broken build. It is answered by
carrying the run's decisions home, never by editing a seed until the artifact
matches: seeds are read-only accelerants, wording is decided in the ledger, and
a seed edited to fit an artifact records a decision nobody made.

## Delivery is a pull request

What survives the gate goes up as a pull request, never as a push to `main`.
Approved wording is content someone decided, so a human reads the diff before it
ships — the same intent `kapi commit` carries one layer down, applied to the
repository rather than the project.

`scripts/auto-pr.sh` does it, on a fixed bot branch (`bot/dogfood-sync`)
force-pushed each run. One rolling pull request per stream: an unmerged one is
refreshed with a comment rather than joined by a nightly sibling, and a night
that brought nothing home pushes nothing, opens nothing, and exits 0 — quiet
nights leave no branches and no pull requests behind.

Only the paths the loop owns are staged — `.kapi/` and the derived set from
`make l10n-derived-paths`, the same two the gate classifies — so an unrelated
working-tree change cannot ride along even if the gate's refusal of foreign
changes were ever relaxed. Staging is by concrete path rather than pathspec,
because `git add` treats a pathspec matching nothing as fatal and the sidecar
glob matches nothing until a locale gets its first one.

The pull request is opened with `GITHUB_TOKEN`, which starts no workflow runs,
so its checks do not run on their own; the body says to close and reopen it (or
push any commit) to start them.

Both scripts run only in scheduled jobs, so `--self-test` drives them against
scratch repositories from the repo-guards job of `ci.yml` and from `make
pre-push`. A change that disarms the gate fails on the pull request that makes
it rather than in a nightly nobody is watching.

[l10n-ci]: https://github.com/neokapi/neokapi/blob/main/docs/internals/l10n-ci.md
