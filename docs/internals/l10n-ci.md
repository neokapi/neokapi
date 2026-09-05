# The dogfood loop in CI

How this repository keeps its own multilingual surfaces current, and why almost
none of it is bespoke.

The repo dogfoods kapi, so this machinery is also a claim: whatever a customer
would have to build here, the product is missing. Every step below is either
something a customer would write the same way, or has a one-line reason it
cannot be.

Related: [CLAUDE.md](../../CLAUDE.md) for the isolation contract and the
target-drift rule; the recipe itself (`kapi.yaml`) for what each collection is;
[`.kapi/README.md`](../../.kapi/README.md) for the committed context itself.
[i18n-toil.md](i18n-toil.md) is a different document: the rubric kapi grades
*other* frameworks with.

## One verb, between two build stages

The recipe declares every surface that converges as a content collection with a
`target:` template. Bringing all of them up to date is three stages, and only two
are make's business:

| Stage | Make target | Whose job |
| --- | --- | --- |
| 1. Extract | `l10n-extract` | ours: the React extractors and two `go:generate`s |
| 2. Converge | `l10n-converge` | kapi: one `kapi up` |
| 3. Compile | `l10n-compile` | ours: catalogs into what each runtime loads |

`make l10n` runs all three as recursive submake calls, so the order holds under
`make -j`, with the `qps` probe (`l10n-pseudo`) between converge and compile and
the collapse guard (`l10n-collapse-check`) after them.

Stages 1 and 3 exist because
[E-06](../../web/docs/contribute/architecture/engine/e-06-execution-trust.md) refuses a
recipe that names a subprocess: "a recipe is trusted" is exactly the assumption
execution trust exists to disprove. So the extractors and the catalog compilers
stay outside the recipe, and stay make's job. That is the whole justification; if
E-06 is ever revisited, both stages become recipe declarations and this
document loses half its content.

Stage 2 is one `kapi up`, and it is the same verb the nightly runs and the same
verb the product tells a customer to run. It seeds itself: the terms bundle
bound by `defaults.terms_source` and every content-memory bundle under
`.kapi/memory/` are compiled into the project store on the way in, keyed by each
file's content digest, so an unchanged bundle costs a read and a `git pull` of
an edited one recompiles exactly itself. Then it re-extracts the block store
from the working tree, runs the recipe's flow over every collection and locale,
and materializes the targets (`defaults.materialize: on-converge`).

Nothing wipes the store. The store is the union of what git carries and what a
venue pull brought home, so a wipe deletes precisely the half git does not
hold: every approval a reviewer made on the server, gone from a build that
reports success.

One collection names no `target:` at all. `neokapi-docs-reference` binds the
authored dossiers under `scripts/gen-refs/nativedocs/`, which
`make generate-reference-docs` compiles into the reference dataset and
`make generate-reference-pages` renders as the Format and Tool Reference, pages
that carry a DO-NOT-EDIT banner and hold no prose of their own. Without a target
the loop passes over it, and what the collection buys is the other half of a
recipe: `kapi check` reads the project's declared content, so declaring the
dossiers puts the register of the largest generated body of prose in this
repository under the same bar as the prose an author writes by hand, at the one
place a finding against it can be acted on.

That bar is enforced. `make check-reference-prose` runs the check over the
collection and fails on any critical, major or minor finding; the
`reference-data-drift.yml` workflow runs it beside the dataset and page gates, so a
dossier lands fresh and in register in the same change.

The recipe binds `flow: tm-recycle`: exact-match content-memory leverage and
nothing else: no AI, no provider credentials, no network. A checkout with no
credentials therefore converges from the committed context alone. AI convergence
happens at the server venue, on the org's keys; a deliberate local AI pass is
`kapi run translate-ai`. `make l10n` pins the local venue by discovering no
plugins (`KAPI_PLUGINS_DIR_ONLY=1`), so an install that carries kapi-bowrain
does not silently turn a developer's regeneration into a server run.

Stage 3 compiles a catalog into whatever its runtime loads. The SPAs, the
landing page and the transactional emails take compiled dictionaries and
rendered HTML from `@neokapi/i18n-react`. The Go binaries take gettext MO: `make
i18n-catalogs` compiles each committed `core/i18n/catalogs/<lang>.json` and
`host/i18n/catalogs/<lang>.json` into the sibling `<lang>.mo` that `//go:embed
catalogs/*.mo` picks up. Version control carries the JSON, text a reviewer
reads in a diff, and never the MO. That is why the compiler is pure Go in the
framework module rather than a kapi invocation: it links neither package it
writes for, so there is no bootstrap cycle, and every target that builds, tests,
vets or lints those modules can declare it as a prerequisite. A build that skips
it fails on the embed pattern rather than silently shipping English.

Each locale is compiled where its catalogs are on disk and skipped where they
are not, because two walks share this stage: `make l10n` arrives with the loop's
target catalogs, and `make l10n-build` arrives with only the `qps` probe.

The reference dataset is compiled here too. The docs site's `/tools`,
`/formats`, `/commands` and `/models` pages print names, descriptions, command
help and model notes from `packages/reference-data/data/*.json`, which
`make generate-reference-docs` writes in English. The same run, and this stage
with `gen-refs -locales-only`, derive one variant per locale that has a Go
catalog (`packages/reference-data/data/<lang>/`) by walking the catalogs beside
`core/i18n/builtins/metadata.json` and `host/i18n/commands.json`, and by
overlaying the translated dossiers the loop writes to
`scripts/gen-refs/nativedocs/<lang>/`, the way the English dossiers are overlaid
onto the English dataset. A string the catalog lacks keeps its English and is
counted in a warning. The site's `reference-locale` plugin swaps the variant in
for the locale it is building. The `qps` variant and the `qps` dossiers are
build-derived and byte-gated; a target locale's are loop output that follows
its catalog, and `make check-reference-docs` holds both to the committed
English dataset, catalogs and dossiers.

The `qps` pseudo-locale is that probe (`l10n-pseudo`). It answers a
runtime-correctness question (does the UI survive expanded, marked-up text?)
rather than carrying project content, so it is not a target language in the
recipe. The pass runs under `$(KAPI_ISO_ENV)`; the landing page and its shell
head bind the recipe with an explicit `-p` only so that `defaults.locales.qps`
can turn the markers off for the published pseudo builds. It expands the
extracted source catalogs mechanically, which is why its output is byte-gated
where the loop's is not.

The byte gate holds only if the two gitignored trees between the stages are a
function of the current source, and each stage keeps its own tree so.
`neokapi-i18n extract` removes a catalog under `i18n/` that it would have
written for the document it records and did not, which is the catalog of a
source that has been deleted, moved or excluded. `l10n-pseudo` clears each
`i18n-qps/` tree before kapi writes it, because kapi writes one target per
catalog it reads and knows nothing of a file already there. Without either, a
catalog for a deleted component stays in both trees, reaches the runtime
dictionary through `l10n-compile`, and a checkout that has run the walk for
weeks regenerates different bytes from a clean one: the author's own run and
the gate's disagree, and the auto-fix commits a correction the author could
not have produced.

The two Docusaurus sites are pseudo-translated too, for a local preview:
`scripts/pseudo-docs-i18n.sh` writes each site's `i18n/qps/` tree, which is
gitignored, and the docs workflows regenerate it with a released kapi
(`.github/actions/pseudo-locale`) immediately before each site build. Nothing of
it is committed, so nothing of it is byte-gated.

## Two tiers, and what may be asserted about each

Every committed artifact this pipeline owns is a function of one of two things,
and that is what decides how it is checked.

**Build-derived** is a function of committed source alone: the two generated
inventories (`core/i18n/builtins/metadata.json`, `host/i18n/commands.json`), the
English email renders, and the whole `qps` tier, including the `qps` probe of
each Go-surface catalog and the reference dataset's `qps` variant. Nothing here passes through the
project store, so regenerating it must reproduce it byte for byte. `make
l10n-verify` runs `make l10n-build` (extraction, the probe, and the compilations
that read neither the store nor a target locale) and diffs the set that `make
l10n-derived-paths` prints, so the Makefile and CI cannot disagree about it. It
also fails on *untracked* output, because `git diff` alone cannot see an artifact
for a surface that did not exist before.

**Loop-owned** is the target-language tier `kapi up` writes out of the project
store: the Go-surface catalogs, the runtime dictionaries, the subject catalogs,
the rendered per-locale email templates, and the demo narration sidecars. A byte
gate over this tier would require a checkout with no server to reproduce wording
a reviewer approved on one, and would overwrite the wording the moment it could
not, green every time and silent. So there is no byte gate here.

What replaces it is not a weaker version of the same question. Reproducibility
cannot be asserted about this tier; **soundness** can, and it is a different
question with a different answer. A derived artifact must parse, must carry
exactly the interpolation placeholders its source carries, and must not have
translated a machine identifier the recipe never declared translatable. None of
those is coverage: a string the target does not carry falls back to its source,
which is the pending state the loop absorbs. These are strings the target does
carry and gets wrong, and the next convergence re-materializes them rather than
repairing them.

- `scripts/check-derived-content.mjs`: the content reader. Every artifact is
  measured against the document it derives from: the `qps` probe where a surface
  has one, its source document otherwise, and for a narration sidecar the
  `demo.yaml` master it overlays. `make l10n-content-check` reads the whole
  committed tier; `scripts/check-sync-backed.sh` reads what a run wrote, and
  refuses on a defect.
- `make l10n-report`: per-locale coverage and placeholder parity, posted to the
  `l10n` workflow's job summary. It reports and cannot fail: an English change
  that leaves nb behind is pending work, not a build break. Coverage and
  soundness are two readings of one relation, which is why both take their pairs
  from `make l10n-content-pairs`.
- `make l10n-collapse-check`: existence. A catalog that carried entries at HEAD,
  in a locale the committed content memory still holds pairs for, may not come
  back empty. Partial coverage passes. It runs inside `make l10n`, in the walk
  that produced the files, because that is the only place that can tell an empty
  catalog from an unchanged one.
- `scripts/check-sync-backed.sh`: the return-leg gate, below.

`make l10n-orphans-report` is the mirror image of coverage: the memory entries
that produced nothing, which [`.kapi/README.md`](../../.kapi/README.md) explains.
It reports and never gates either.

`make l10n-stale-report` is the mirror image of *that*, and the two together
cover both ways a source rewrite lands. A KBF catalog keys on the source text, so
rewording changes the key: the old target stops being emitted and the locale
falls back to English: pending work, reported by the orphan report. The
Go-surface catalogs are addressed by scope path, so rewording keeps the scope and
the previous translation stays attached to a sentence the source no longer
contains. Nothing falls back, and the locale ships a wrong translation that looks
exactly like a current one. The wording a translation was produced from survives
a clone in exactly one place, the target's own git history, which is what this
reads. It reports and never gates: re-translating an entry, or removing it so the
locale falls back until the next convergence, is a decision rather than a build
step.

Three make targets print the three path sets, one definition each, so nothing
re-derives a list:

| Target | Set | Read by |
| --- | --- | --- |
| `l10n-derived-paths` | build-derived | `l10n-verify`, `scripts/l10n-autofix.sh` |
| `l10n-loop-owned-paths` | loop-owned | a reviewer asking what may move with no source change behind it |
| `l10n-owned-paths` | both | `scripts/check-sync-backed.sh`, the nightly's delivery step |

The gate and the delivery step read the union because a convergence run may
legitimately have written either tier, and anything else it touched is foreign.
The byte gate reads only the first, because only the first is reproducible.

## In CI

Two workflows.

**`.github/workflows/l10n.yml`** is the build-derived gate. Path-gated on the
sources the extractors read, the Go sources behind the two generated
inventories, the committed catalogs (for the `qps` sibling beside them), and the
extractor/compiler packages. It runs `scripts/l10n-autofix.sh`, which runs `make
l10n-build` and then:

- no drift → pass;
- drift on a **same-repo pull request** → commit exactly the owned paths as
  `github-actions[bot]` and push to the PR head branch. Regeneration is
  deterministic, so the rerun on the bot commit finds nothing and the loop
  terminates after one commit. The push uses `GITHUB_TOKEN`, which does not
  re-trigger workflows, so the checks on the current run's SHA are
  authoritative;
- drift **anywhere else** (fork PR, push to main, a human's laptop) → fail, like
  `make l10n-verify`.

Auto-committing is the right shape here for one reason only: every artifact
under the gate is a deterministic function of committed source, so asking a human
to run a command and push its output is asking them to be a build step. That is
also why the walk it runs has no convergence in it. A job that regenerated the
target tier from the committed bundles and committed the result would overwrite
every approval the loop had brought home: the regeneration would be green, the
diff would look routine, and the wording would be gone.

The job's summary carries `make l10n-report`: coverage and placeholder parity
per locale, standing and never gating.

This and the dogfood sync below are the only jobs in the repository that hold
`contents: write` for multilingual reasons, and they write to different places:
this one to the pull-request branch under review, the sync only to its own bot
branch.

**`.github/workflows/dogfood-sync.yml`** is the convergence loop against
bowrain.cloud, nightly and on demand. It is deliberately unremarkable:

```yaml
- uses: neokapi/setup-kapi@v1
  with:
    plugins: bowrain
    auth-token: ${{ secrets.BOWRAIN_AUTH_TOKEN }}
- run: make l10n-extract
- uses: neokapi/kapi-action@v1
  with:
    command: up
- run: make l10n-compile
```

`setup-kapi` installs the **released** CLI and the bowrain plugin from the
registry rather than building either from the checkout, so the nightly is a test
of the product rather than of the working tree. Both are pinned to an explicit
prerelease: `latest` resolves stable tags only, so a release candidate is
reachable from a pin and from nowhere else, and the pin is what carries a new
CLI or plugin into the nightly. `kapi-action` runs the convergence verb and
reports; the workflow delivers.

The two non-standard steps are the pipeline's own build stages, run here for the
reason they exist at all: the recipe may not name a subprocess. `make
l10n-extract` comes first because the collections that declare
`i18n/**/*.kbf.json` catalogs take them from the extractors, which produce them
from React source: build artifacts, gitignored by design, so a run from a clean
checkout would carry nothing for them. Source catalogs only: the target side is the loop's job, and target drift must never gate a push.

### The extract root is declared, and moving it re-keys a surface

A catalog records the source file it came from, and that path is two things at
once: what a reviewer reads in bowrain, and the document's identity: it spells
`doc.path`, `doc.id` and every block id under it.

Left to the working directory it is only the first of those. A surface whose
`--src` reaches outside its own package records
`../../apps/bowrain/frontend/src/App.tsx`, which names a real file only to
someone who knows where the extract ran; bowrain, holding the catalog and not
the checkout, showed a reviewer a row reading
`apps/bowrain/frontend/src/App.kbf.json`, a path that looks like a repository
path and is not one. So the two surfaces whose roots cross packages declare
`--source-root` at the repository root (`KAPI_DESKTOP_SOURCE_ROOT`,
`BOWRAIN_APP_SOURCE_ROOT`), and every path they record means the same thing read
from anywhere. `ctrl`, `pulse`, `emails` and `landing` scan only their own
`src/`, where the working directory already is the root, so they declare nothing.

**Declared, never derived.** A root inferred from the common ancestor of
whatever globs matched would make every block id a function of which files
happened to exist: add a `--src` reaching one level further up and the whole
collection re-keys, silently. A root someone wrote down moves when they move it.

Moving one is therefore a data decision, not a flag change. Every block id
changes, and a push declares an item's key set rather than diffing ids, so the
old ids read as removed and the new ones as added: the item is rewritten rather
than migrated, and each block starts again with no target, no review state and
no history. The surface's stored blocks are reset with it
(`bowrain-infra/docs/runbooks/data-reset.md`). What does *not* move is the
message hash, taken over the text and its descriptor and never over the path, so
no translated catalog entry is orphaned and the loop refills from content
memory.

This is the one in-repo kapi invocation that binds the root recipe against the
server venue. Everything else must isolate itself per the contract in CLAUDE.md.

Three steps sit between `kapi up` and delivery, in this order.

`kapi commit` is the loop's return leg for unit decisions. The pull stages the
server's approved decisions in the project's working store, and `kapi commit` is
the only door from there into the committed record under `.kapi/state/` that git
tracks: recording a decision and publishing it stay separate acts, so `up` does
not do it. Nothing staged is a no-op that exits 0. The terminology return leg
needs no step: the concept pull merges approved term decisions into
`.kapi/terms.json` itself, upsert-only and byte-stable, so a night with no new
decisions writes nothing.

`make l10n-compile` is the pipeline's third stage, and it runs here rather than
only on a developer's machine because no runtime loads a catalog: the SPAs and
the landing page load compiled dictionaries, the transactional emails rendered
per-locale HTML, the Go binaries embedded MO. A night that delivered catalogs
alone would leave every surface compiled from them behind until someone ran
`make l10n` by hand. It reads what the loop materialized, so it runs after the
return leg and before the gate, which classifies the compiled dictionaries and
the per-locale renders as owned output alongside the catalogs they come from.

`scripts/check-sync-backed.sh` then reads what the run wrote, which is why it
runs last. It classifies the working tree (context, derived, foreign) and then
opens the derived artifacts: a run whose output does not parse, dropped a
placeholder, or translated a machine identifier is refused by name, and the
nightly goes red. It also refuses anything the run changed outside `.kapi/` and
the owned set: an indiscriminate delivery would otherwise carry a source edit
into main with no review and no CI.

Two things it does *not* do, each for a reason the other half of this document
gives.

It does not require the committed context to have moved. A night that converged
and approved nothing is the ordinary night for a repository whose source moves
daily and whose reviewers approve in batches, and requiring backing made the
nightly red on every one of them. What the context did is *reported* beside the
verdict, and reported by what it says rather than by whether its bytes moved,
because a projection that re-sorted an array decided nothing and was once
credited with backing forty-eight derived files.

It does still require the context to explain a **removal**. A rewrite carries
content to read; a deletion carries none, so there the committed context stays
the authority: a catalog or a sidecar that disappeared is an erasure until a
decision shard, a terms edit, a memory seed, the voice profile or a profile
explains it. A re-serialization is not that.

Delivery is `scripts/auto-pr.sh`: what survives the gate goes up as a pull
request on the rolling `bot/dogfood-sync` branch, never as a push to main, so a
human reads approved wording before it ships. A night that brought nothing home
opens nothing and leaves no branch behind. Both scripts carry a `--self-test`
that runs in the repo-guards job, because nothing on a pull request exercises
either of them.

### What CI needs configured

Nothing is created by this repository; both already exist and are referenced by
name.

| Kind | Name | Used by | Purpose |
| --- | --- | --- | --- |
| Secret | `BOWRAIN_AUTH_TOKEN` | `dogfood-sync.yml` | a `bwt_` workspace API token; `setup-kapi` exports it as the CLI's auth |
| Variable | `DOGFOOD_SYNC_ENABLED` | `dogfood-sync.yml` | must equal `true` or every job skips; no secret is read and no content moves |

`l10n.yml` needs neither: it uses the built-in `GITHUB_TOKEN` and never talks to
a server. Unsetting `DOGFOOD_SYNC_ENABLED` disarms the dogfood completely and
reversibly.

## What is left, and why each one survives

| Piece | One-line justification |
| --- | --- |
| `l10n-extract` (and the extractors under it) | E-06: a recipe may not name a subprocess |
| `l10n-compile` | same; no runtime loads a catalog directly: the SPAs load compiled dictionaries, the emails rendered HTML, the Go binaries embedded MO |
| `l10n-converge` | the loop itself, one `kapi up` over the whole recipe |
| `l10n-build` | the build tier with no loop in it: what the byte gate regenerates and what a pull request may safely have committed back |
| `l10n-pseudo` | `qps` is a correctness probe, not project content, so it is not a target language |
| `i18n-catalogs` | the Go binaries' embedded MO is build output, and `//go:embed` resolves at compile time, so it is a build prerequisite rather than a stage anyone runs by hand |
| `l10n-verify` / `l10n-derived-paths` | a generated-vs-source gate over the build tier, and its single source of truth for what that set is |
| `l10n-loop-owned-paths` / `l10n-owned-paths` | the target tier, and the union the erasure gate and the delivery step classify against |
| `l10n-report` | coverage and placeholder parity over the target tier, reported and never gating |
| `l10n-content-pairs` | what each derived artifact derives from, defined once for the report and the gate |
| `l10n-content-check` | the soundness question over the whole committed tier: the standing burndown of what is already in git |
| `l10n-collapse-check` | existence, not coverage: a catalog that carried entries may not come back empty, asserted in the walk that produced it |
| `l10n-review-export` | emits the lossy interchange views (TMX/CSV) a human reviewer asks for; read-only, and wording is still decided in the ledger |
| `l10n-orphans` / `l10n-orphans-report` | content memory matches on text, so an entry whose source string is gone is wording any surface can pick up again, and the only safe version of keeping it is seeing it |
| `l10n-stale-report` | a scope-addressed catalog keeps a translation attached to its scope when the sentence under it is rewritten, and git is the only record of what it was a translation of that survives a clone |
| `scripts/l10n-autofix.sh` | the deterministic-regeneration commit; nothing standard commits the output of a stage that is not kapi's |
| `make l10n-extract` in the dogfood workflow | the loop cannot carry collections whose source catalogs do not exist yet |
| `make l10n-compile` in the dogfood workflow | a night that delivered catalogs alone would ship every runtime compiled from the previous one |

Anything not in that table should not exist. If a new surface needs a new make
target, ask first whether the recipe can declare it instead; it usually can,
and then the answer is a collection, not a target.
