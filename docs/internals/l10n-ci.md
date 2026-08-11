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
[i18n-toil.md](i18n-toil.md) is a different document — the rubric kapi grades
*other* frameworks with.

## Four stages, and only two are ours

The recipe declares every surface as a content collection with a `target:`
template. Bringing all of them up to date is four stages:

| Stage | Make target | Whose job |
| --- | --- | --- |
| 1. Extract | `l10n-extract` | ours — the React extractors and two `go:generate`s |
| 2. Seed | `l10n-seed` | kapi — one command |
| 3. Translate | `l10n-translate` | kapi — one command |
| 4. Compile | `l10n-compile` | ours — catalogs into what each runtime loads |

`make l10n` runs all four; each stage depends on the one before it, so it is one
ordered walk even under `make -j`.

Stages 1 and 4 exist because
[AD-038](../../web/docs/contribute/architecture/038-execution-trust.md) refuses a
recipe that names a subprocess — "a recipe is trusted" is exactly the assumption
execution trust exists to disprove. So the extractors and the catalog compilers
stay outside the recipe, and stay make's job. That is the whole justification;
if AD-038 is ever revisited, both stages become recipe declarations and this
document loses half its content.

Stage 3 is one `kapi run tm-recycle --target-lang <lang>` with **no `-i`**. With
no input flag the flow-run path resolves the project's content patterns and
writes each item's output from its own `target:` template, so every collection
is covered by construction. Adding a collection to the recipe translates it with
no Makefile change at all. This replaced eleven per-surface loops that did the
same thing by hand, and a demo allowlist that asked a human to remember to
extend it.

`tm-recycle` is exact-match content-memory leverage and nothing else: no AI, no
provider credentials, no network. Its output therefore contains only reviewed
wording, and the pass is a deterministic function of source plus committed
context — which is what makes a byte gate over it legitimate.

Stage 4 compiles a catalog into whatever its runtime loads. The SPAs, the
landing page and the transactional emails take compiled dictionaries and
rendered HTML from `@neokapi/i18n-react`. The Go binaries take gettext MO:
`make i18n-catalogs` compiles each committed `core/i18n/catalogs/<lang>.json`
and `host/i18n/catalogs/<lang>.json` into the sibling `<lang>.mo` that
`//go:embed catalogs/*.mo` picks up. Version control carries the JSON — text a
reviewer reads in a diff — and never the MO. That is why the compiler is pure
Go in the framework module rather than a kapi invocation: it links neither
package it writes for, so there is no bootstrap cycle, and every target that
builds, tests, vets or lints those modules can declare it as a prerequisite.
A build that skips it fails on the embed pattern rather than silently shipping
English.

The `qps` pseudo-locale is a separate, isolated pass (`l10n-pseudo`). It is a
runtime-correctness probe — does the UI survive expanded, marked-up text? — not
project content, so it is not a target language in the recipe and does not bind
the project. It writes `core/i18n/catalogs/qps.json` in the same shape as any
other locale's catalog, and stage 4 compiles it like any other.

## One gate, and what it is allowed to assert

`make l10n-verify` regenerates everything and diffs the committed derived set,
which `make l10n-derived-paths` prints so the Makefile and CI cannot disagree
about it. It also fails on *untracked* output, because `git diff` alone cannot
see a catalog for a locale or a surface that did not exist before. The two Go
catalog directories are in that set for their `<lang>.json`; the `<lang>.mo`
beside them is gitignored, and the untracked check honours `.gitignore`, so
build output does not read as drift.

The gate asserts **generated-vs-source consistency** and nothing else. It says
nothing about how much of a target language is translated, and must never be
made to: an English change that leaves nb behind is pending work, not a build
break. Coverage is reported to the job summary and cannot fail anything, and so
is the mirror-image number — the seed entries that produced nothing, which
`make l10n-orphans-report` lists and
[`.kapi/README.md`](../../.kapi/README.md) explains.

Five separate gates used to cover subsets of this, and the subsets did not add
up to the whole — a surface could go stale in the gap between them.

## In CI

Two workflows.

**`.github/workflows/l10n.yml`** — the derived-artifact gate. Path-gated on the
recipe, the committed context (`.kapi/terms.json`, `.kapi/voice.yaml`,
`.kapi/memory/`, `.kapi/profiles/` — never the derived `.kapi/work/` or the unit
state under `.kapi/state/`), the sources the extractors read, the Go sources
behind the two generated inventories, and the extractor/compiler packages. It
runs `scripts/l10n-autofix.sh`, which runs `make l10n` and then:

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
under the gate is a deterministic function of committed inputs, so asking a
human to run a command and push its output is asking them to be a build step.
The moment an artifact under this gate stops being deterministic, the auto-fix
becomes wrong and the gate must go strict.

This is the only job in the repository that holds `contents: write` for
multilingual reasons. It used to be four, including the frontend and desktop
jobs — which run `vp install` and a full test suite on pull-request content, and
held repository write only because a catalog check was bolted onto them.

**`.github/workflows/dogfood-sync.yml`** — the convergence loop against
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
```

`setup-kapi` installs the **released** CLI and the bowrain plugin from the
registry rather than building either from the checkout, so the nightly is a test
of the product rather than of the working tree. `kapi-action` runs the
convergence verb and commits what came back. The one non-standard step is
`make l10n-extract`, and it is there because five collections declare catalogs
the extractors produce from React source — build artifacts, gitignored by
design, so a run from a clean checkout would carry nothing for them. Source
catalogs only: the target side is the loop's job, and target drift must never
gate a push.

This is the one in-repo kapi invocation that binds the root recipe. Everything
else must isolate itself per the contract in CLAUDE.md.

Between `kapi up` and the delivery step sits `scripts/check-sync-backed.sh`. The
artifacts the loop owns are rebuilt from `.kapi/` on the next regeneration, so a
run that produces them without a matching `.kapi/` change delivers wording with
nothing behind it — content the following regeneration overwrites from a context
that never learned it, with every run in the sequence green. The gate refuses
that run, naming every file, and the nightly goes red. It also refuses anything
the run changed outside `.kapi/` and the derived set, backed or not: `git add -A`
would otherwise carry a source edit into main with no review and no CI. Its
`--self-test` runs in the repo-guards job, because nothing on a pull request
exercises the gate itself.

### What CI needs configured

Nothing is created by this repository; both already exist and are referenced by
name.

| Kind | Name | Used by | Purpose |
| --- | --- | --- | --- |
| Secret | `BOWRAIN_AUTH_TOKEN` | `dogfood-sync.yml` | a `bwt_` workspace API token; `setup-kapi` exports it as the CLI's auth |
| Variable | `DOGFOOD_SYNC_ENABLED` | `dogfood-sync.yml` | must equal `true` or every job skips — no secret is read and no content moves |

`l10n.yml` needs neither: it uses the built-in `GITHUB_TOKEN` and never talks to
a server. Unsetting `DOGFOOD_SYNC_ENABLED` disarms the dogfood completely and
reversibly.

## What was removed, and why

| Removed | Why |
| --- | --- |
| ~30 per-surface `l10n-*` / `*-translations` / `*-l10n-verify` targets | each was a hand-rolled subset of what the recipe already declares |
| `L10N_DEMO_DIRS` allowlist | a note asking a human to remember something; dropping sidecars byte-identical to their source is two lines and self-maintaining |
| four `scripts/l10n-autofix.sh` call sites | one workflow over the whole graph, so the subsets cannot fail to add up |
| `emails-landing-l10n` job (ci.yml) | the same gate, on two of the surfaces |
| `l10n-drift` job (reference-data-drift.yml) | the same gate, on three others |
| `docs-l10n-drift` job | materialized both docs sites twice per run to assert kapi's recycle is deterministic — a claim about kapi that belongs in kapi's tests, not a repo gate paying for two full passes |
| `l10n/**` path filters | named a directory that no longer exists; the context is committed flat by domain under `.kapi/` |
| `l10n_surfaces` change filter (ci.yml) | had no consumer left |

## What is left, and why each one survives

| Piece | One-line justification |
| --- | --- |
| `l10n-extract` (and the six extractors under it) | AD-038: a recipe may not name a subprocess |
| `l10n-compile` | same; no runtime loads a catalog directly — the SPAs load compiled dictionaries, the emails rendered HTML, the Go binaries embedded MO |
| `l10n-seed` | wipe-and-reseed proves the committed context is the truth and the one project store (`.kapi/work/store.db`) is only its projection |
| `l10n-pseudo` | `qps` is a correctness probe, not project content, so it is not a target language |
| `i18n-catalogs` | the Go binaries' embedded MO is build output, and `//go:embed` resolves at compile time, so it is a build prerequisite rather than a stage anyone runs by hand |
| `l10n-verify` / `l10n-derived-paths` | a generated-vs-source gate over committed artifacts, and its single source of truth for what that set is |
| `l10n-review-export` | emits the lossy interchange views (TMX/CSV) a human reviewer asks for; the native seeds stay the truth |
| `l10n-orphans` / `l10n-orphans-report` | the seeds are human-owned and keep entries their source string has outlived; content memory matches on text, so a kept entry is wording any surface can pick up again, and the only safe version of keeping it is seeing it |
| `scripts/l10n-autofix.sh` | the deterministic-regeneration commit; nothing standard commits the output of a stage that is not kapi's |
| `make l10n-extract` in the dogfood workflow | the loop cannot carry collections whose source catalogs do not exist yet |

Anything not in that table should not exist. If a new surface needs a new make
target, ask first whether the recipe can declare it instead — it usually can,
and then the answer is a collection, not a target.
