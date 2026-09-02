# CLAUDE.md

neokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/)
in Go — a content and language intelligence framework: format-aware document
parsing into one content model, channel-based concurrent processing flows,
faithful write-back, and pluggable tools that edit, check and translate the
content inside.

This file covers what you can't learn by reading the tree. Architecture,
interfaces, and how-to guides live in the docs — see [Where things are
documented](#where-things-are-documented).

## Modules and the dependency direction

A multi-module Go monorepo coordinated by `go.work`. The dependency arrows are
one-way and enforced; violating one breaks `make audit-modules`.

| Module | Path | May depend on |
| --- | --- | --- |
| Framework | `.` (`core/`, `memory/`, `terms/`, `providers/`) | — |
| Host — cobra-free runtime + services | `host/` | framework |
| CLI — thin Cobra shell over host | `cli/` | framework, host |
| Kapi — the `kapi` binary | `kapi/` | framework, host, cli |
| Kapi Desktop — Wails v3 app | `apps/kapi-desktop/` | framework, host |
| Bowrain Core | `bowrain/core/` | framework |
| Bowrain Plugin — `kapi-bowrain` binary | `bowrain/plugin/` | framework, host, cli, bowrain/core |
| Bowrain — server/web/connectors | `bowrain/` | framework, host, bowrain/core |
| SaT segmenter plugin | `plugins/sat/` | framework |

Non-obvious constraints:

- **The framework never imports bowrain.** No Wails, Echo, Cobra, or OIDC below
  the root module. Bowrain attaches through the extension mechanism and the
  plugin registries, never through a `core/` → bowrain import.
- **Host is cobra-free.** Command threading is the `host.Command` interface
  (context + `*pflag.FlagSet` + IO), which `*cobra.Command` satisfies natively;
  embedded runs use `host.EnvCommand`.
- **kapi-desktop links neither cobra nor the cli module** — asserted by
  `make audit-modules` via `go list -deps ./backend/...`.
- **License boundary.** Framework, host, cli, kapi and kapi-desktop are
  Apache-2.0; `bowrain/` is AGPL-3.0, **except `bowrain/plugin/`, which is
  Apache-2.0** — the `kapi-bowrain` binary is a client of the server, not a part
  of it, and links nothing else under `bowrain/`. The line is drawn by
  **directory containment under a `LICENSE` file** and by nothing else — there
  are no SPDX headers, so moving a file between subtrees *is* relicensing it.
  `make audit-modules` and the `module-boundaries` CI job assert both
  consequences: **no Apache module reaches a package under `bowrain/`, with no
  exception**, and the plugin binary's closure stays inside `bowrain/plugin/`.
  Do not add an import exception. A type both sides need belongs below the line
  (`core/venue`, `host/venue/…`), never on an allowlist above it.

  The same two run `scripts/check-ts-license-boundary.sh`, which asserts the
  TypeScript half: no file outside `bowrain/` may reach an AGPL one by relative
  path, by the name of a package whose `package.json` sits under `bowrain/`
  (`@neokapi/ui` is `bowrain/packages/ui`), by a tsconfig `paths` alias, or by a
  `package.json` dependency. Go import closures say nothing about a React
  component, and the pnpm workspace links every internal dependency from source,
  so such an import compiles and bundles into the Apache desktop app with
  nothing to notice it. Shared frontend code belongs in `packages/`, where
  `@neokapi/ui-primitives` already sits. The check has no allowlist either.
- **`kapi` links no plugin's code.** Plugins (bowrain, okapi-bridge, sat, …) are
  discovered at runtime via the unified manifest model and dispatched as
  subprocesses. `bowrain/plugin/*` is blank-imported into `kapi-bowrain`, not
  into `kapi`. The mechanism is the guarantee: what a plugin depends on and what
  licence it carries stay in its own binary, whoever wrote it. See
  [Note: Plugin model](web/docs/contribute/implementation/engine/plugin-model.md).

## Build

`make help` is the catalog — it is self-documenting and current, so read it
rather than guessing target names. `make pre-push` runs the checks relevant to
your changes and mirrors CI.

Prefer `make` targets over raw `go build` / `go test`: the Makefile handles
prerequisites (e.g. `make proto` before builds that need generated gRPC code)
and puts binaries in `bin/`. Drop to `go test ./core/flow/ -run TestX -v` only
when targeting one package or test.

Frontend work runs off a single root pnpm workspace (`pnpm-workspace.yaml`) —
`vp install` at the repo root, never per-directory installs. Module isolation is
checked with `GOWORK=off` builds per module plus `make audit-modules`.

**`go` does not union repeated `-tags` — the last occurrence wins.** So
`$(GOTEST) -tags parity` silently drops the `fts5` that `$(GOTAGS)` baked in,
and `go build -tags fts5 -tags server` builds without FTS5. Spell every tag in
one comma-separated flag (`-tags "fts5,parity"`), and reach for `$(GO)` rather
than `$(GOTEST)`/`$(GOBUILD)` when a target needs its own tag set, so no macro
can re-introduce the shadowing.

Nothing fails when this goes wrong: `fts5` is a runtime SQL capability, not a
compile gate, so the build succeeds and only a query that reaches FTS5 notices —
`no such function: fts5`, arbitrarily far from the flag. It cost a headless
binary that died on its first memory or terms query, and a parity suite running
in a configuration no other target used, on the same day. Check with:

```bash
go list -tags "<your tags>" -f '{{.CgoCFLAGS}}' github.com/mattn/go-sqlite3 | grep FTS5
```

### A piped command hides its exit code

A pipeline reports only its *last* stage, so `golangci-lint ./... | tail` exits
0 when the linter found problems. A backgrounded `cmd > log; echo EXIT=$?` has
the same shape: the status belongs to `echo`. Both report a failed check as a
passing one, and the only evidence is in the log nobody read.

Start any command whose exit code you will act on with `set -o pipefail`, or
redirect to a file and read `$?` before filtering:

```bash
set -o pipefail; golangci-lint run ./core/... | tail -20   # exits non-zero
golangci-lint run ./core/... > /tmp/lint.out 2>&1; echo $?  # or capture first
```

## Dogfooding kapi: the in-repo isolation contract

This repo dogfoods kapi. A `kapi.yaml` recipe at the repo root is driven by the
**system-installed** kapi + plugins (real `kapi-bowrain`, real keychain auth,
real server), and it is auto-discovered by a git-style **upward walk** from any
cwd inside the tree (`core/project.ResolveLayout` → `host.ResolveProjectPath`,
re-exported as `cli.ResolveProjectPath`).

**Every in-repo kapi invocation that is not the dogfood workflow must isolate
itself**, or it will silently bind to — and act on — the dogfood project. Set on
the kapi process environment:

- `KAPI_NO_PROJECT=1` — opt out of discovery (an explicit `-p` still wins).
  `KAPI_PROJECT=""` does **not** disable discovery; only a non-empty
  `KAPI_NO_PROJECT` does.
- `KAPI_CONFIG_DIR`, `XDG_DATA_HOME`, `XDG_CACHE_HOME` → throwaway dirs, so kapi
  can't read the developer's `~/.config/kapi`, plugins, or caches.
- `KAPI_PLUGINS_DIR_ONLY=1` — discover plugins only from `$KAPI_PLUGINS_DIR`
  (empty → none). `XDG_DATA_HOME` alone isolates the *user* plugin root only;
  without this, an in-repo kapi still picks up Homebrew-installed plugins.

Already wired: the Makefile's shared `$(KAPI_ISO_ENV)` prefix, `kapi/e2e`'s
`isoEnv` in `TestMain`, and `harness/` (sandboxes in `os.tmpdir()` via
`kapiIsolationEnv()`). Follow the contract when adding a new invocation.

## Never commit an absolute home path

`/Users/<name>/src/...` resolves on exactly one laptop. Locations outside this
repo are named by environment variable with a repo-relative default, so a fresh
clone in the conventional layout needs no environment at all:

| Variable | Default | What it names |
| --- | --- | --- |
| `NEOKAPI_WORKSPACE_DIR` | parent of this repo | The multi-repo workspace — this repo plus `okapi-bridge/`, `registry/`, … |
| `NEOKAPI_CHECKOUTS_DIR` | parent of the workspace | Unrelated reference checkouts |
| `NEOKAPI_OKAPI_DIR` | `$NEOKAPI_CHECKOUTS_DIR/okapi/Okapi` | Upstream Okapi Framework (Java) clone |
| `NEOKAPI_DOCLANG_DIR` | `$NEOKAPI_CHECKOUTS_DIR/doclang-project/doclang` | DocLang specification checkout |

The root Makefile exports all four (`make workspace-paths` prints them); shell
scripts source `scripts/lib/workspace.sh`. Both are worktree-aware — the
workspace is the parent of the **main** checkout via `git rev-parse
--git-common-dir`, because inside `.claude/worktrees/<name>/` the parent is not
a workspace. Prose names the variable, not a resolved path.

`scripts/check-abs-paths.sh` enforces this over every tracked file in `make
lint`, `make pre-push`, and the *Repo guards* CI job. Watch generated artefacts
too — a Go subtest named after an absolute fixture path once put 358 home paths
into a committed dataset. See
[Workspace Paths](docs/internals/workspace-paths.md).

## Never walk a run sequence by hand

A surface that shows content projects the block's `Run[]` into its own shape.
The lossy version of that loop is the one that is easy to write —

```ts
for (const r of runs) if (typeof r.text === "string") out += r.text;
```

— which reads as *concatenate the text* and behaves as *silently delete every
placeholder, every paired code, every plural*. It shipped three times: a review
pane whose source read "Your credits reset on ." beside a target showing the
variable; a document preview that drew a plural block as an empty line; a lab
that measured segmentation over text no reader saw. Nothing failed; the content
was simply gone.

So a projection is **declared**, never looped: a `RunSpec`
(`packages/kapi-format/src/run-projection.ts`) answers for every kind in
`RUN_KINDS`, and its type is a mapped type over that list — omit `plural` and it
does not compile. Each kind is rendered, `{ dropped: "why" }` (nothing, on
purpose, with the reason), or `{ unsupported: "why" }` (reported, and the spec's
required `fallback` drawn in its place). A kind added to the model breaks every
projection until each has said what it does with it.

`scripts/check-run-projection.sh` enforces it in `make lint`, `make pre-push`
and the *Repo guards* CI job; its allowlist is for files that map every kind
one-to-one (coded-text bridges, the KBF canonicaliser, anatomy views), and the
bar for joining it is that the file cannot lose content silently.

## Target-language drift must never block the build

A **source-language** change must never hard-fail a build (or kapi itself)
because **target-language** translations haven't caught up. That drift is the
normal, continuous toil kapi exists to absorb.

- The natural outcome of a source change is pending target-language work: the
  target locale falls back to source, or shows partial coverage. Not an error.
- At worst, gate a *pre-release* check on a translation-coverage bar
  ("nb ≥ X% before a release tag") — never the ordinary build or PR.
- Build policy: source/default locale stays **strict** (broken links throw);
  target locales **warn**.

Target-language artefacts are generated, not authored — `web/i18n/nb/` and
`apps/kapi-desktop/frontend/i18n-nb/` are build artefacts (gitignored,
regenerated by kapi). Never hand-edit them, never hand-translate a target
locale, and never gate a source change on one.

## Documentation assets

Walkthrough videos are documentation. **When UI- or CLI-surface code changes,
re-record the affected walkthrough videos** before committing.

Videos come from the **harness** (`harness/`): authored `demo.yaml` sequences
driven against real infrastructure, screencast, TTS-narrated, rendered with
Remotion into light + dark `.webm`. Two things are easy to get wrong:

- **Narration sidecars are loop output.** `demo.<lang>.yaml` files are written by
  `kapi up` on the dogfood recipe (`make l10n`) — never edited by hand, and never
  hand-translated inline. A wording fix is a decision: correct it where it is
  reviewed and let the next convergence materialize it, rather than editing the
  `.kapi/memory/` bundle the loop reads as an input. A sidecar identical to its
  source is dropped rather than committed, so a demo gets one exactly when its
  narration has been translated. See [the dogfood loop in
  CI](docs/internals/l10n-ci.md).
- **Assets are not in git and not in GitHub releases.** They live only on the
  S3 + CloudFront CDN (`$DOCS_CDN_URL`) and are referenced by URL via
  `ThemedVideo` / `ThemedImage`; docs CI references them rather than recording.
  See [CDN assets](web/docs/contribute/implementation/repo/cdn-assets.md).

Recordings and screenshots run against **real** neokapi infrastructure — real
Keycloak OIDC via `bowrain/compose.yaml`, the real `bowrain-server` binary, a
real PostgreSQL database (the server accepts no other). Never mock the auth
flow or the API. Third-party services outside this project (MT providers,
external LLM APIs) may be mocked.

Runbooks: [regenerating docs assets](docs/internals/regenerating-docs-assets.md),
[video revision](docs/internals/video-revision-runbook.md).

## Commits, PRs and issues

Titles are short, plain descriptions of intent. A reader scanning `git log` or
a PR list wants to know what the change does before they open it.

- Good: `Exclude ALB health probes from Sentry tracing`
- Bad: `The load balancer's health probe is answered, not measured`

No aphorisms, no inversions, no titles that only parse once you have read the
diff. Keep bodies short too — what changed, why, and anything a reviewer would
otherwise have to discover. Reasoning that belongs beside the code goes in a
comment, not the commit message. The same applies to issue titles and PR
descriptions.

## Writing user-facing prose

Follow [brand-communication.md](docs/internals/brand-communication.md): an
academic, restrained register — no marketing superlatives, no emoji. Three rules
that bite most often:

- **The vocabulary is fixed.** Content memory, not *translation memory*/*TM*.
  Terms, not *termbase*/*glossary*. Multilingual content or language, not
  *localization*/*l10n*; recast the sentence rather than substituting a word for
  *localize*. *Translate*, *locale*, *language* and *parity* are real words and
  stay. `grep -niE 'localiz|localis|l10n|termbase|glossar|translation memor'`
  before you commit prose — inflections are what a noun-only search misses.
  **The identifiers now match the concepts.** The retired spellings were swept
  out of recipe keys (`memory:`/`terms:`), state filenames (`memory.db`,
  `terms.db`), CLI flags (`--memory` for the content memory, `--termstore` for
  the terms store) and the package tree (`core/profile`), finishing what #1462
  and #1504 began. Do not reintroduce `tm`/`termbase` as an identifier — if you
  find one, it is a leftover.

  **Voice is the mechanism; brand is one use of it.** The context type is the
  **voice profile**: the recipe key is `defaults.voice`, the command family is
  `kapi voice`, the check gate is `--voice`, the tools are `voice-vocab-check` /
  `voice-check` / `voice-infer`, the annotation type is `voice`, the store
  packages are `voice/` and `bowrain/voice`, the permission is `manage_voice`,
  the tables are `voice_*`, the store inside a project is the shared pool
  (`core/projectdb`, beside the content memory, the terms store and the block
  cache) and a standalone one is `voice.db`, the scoring dimension is
  `compliance`, and the MCP tool is `score_voice_compliance`. *Brand voice* belongs in prose describing the common
  use case, never as the name of the mechanism. A `brand`-named identifier is a
  leftover.

  **Brand is a coordinate, not a subsystem.** `core/project.BrandAxis` sits
  beside `ProductAxis` and `ChannelAxis`; a recipe declares it under
  `defaults.coordinates` — inherited by every collection — or on a collection
  that sits elsewhere. Content sits AT a brand the way it sits at a product;
  what governs it there (a voice profile, terms, gates) is bound at the point
  rather than being part of it. See `strategy/context-axes.md`.

  **The terms-store selector is `--termstore`, not `--terms`.** `--tm` became
  `--memory` (#1520), but `--termbase` became `--termstore` (#1505) rather than
  `--terms`, because `--terms` was already taken — it is the boolean gate on
  `kapi exec dnt-check`. The asymmetry is deliberate,
  `cli/project_voice_termstore_test.go` guards it, and a vocabulary sweep that
  "corrects" `--termstore` to `--terms` is a regression. That is precisely how
  `kapi/e2e` drifted: #1462 rewrote the flag inside a suite no PR runs, and the
  nightly said so for a fortnight with nobody listening.

  **A store is never called `terms`.** The recipe key followed the flag:
  a profile binds one with `profiles.<n>.termstore`. `terms` names the
  *contents* — the concepts, and dnt-check's list of strings.

  **The constraint on wording is a term rule, at every scope.** One term, what
  to use instead, how hard it bites: `profile.TermRule`, carrying an optional
  `ConceptID` that ties it to the concept in the terms store and the graph. It
  is what the voice profile has always written under `vocabulary:`, and since
  #2170 it is also what term-check, translate and recycle take, under one key —
  `term_rules:`. Each tool projects the list for itself; `profile.TermRuleMap`
  is the single projection to the prompt's map, because that map feeds the
  context fingerprint the staleness gate recomputes.

  A rule's `severity` decides whether a violation fails or only reports —
  `minor`/`neutral` warn, everything else (including unset) fails, because rules
  resolved from a terms store carry no severity and must not be silently
  downgraded. A rule with an empty `Replacement` is skipped by the tools: in a
  voice profile a bare term is meaningful, but "say this instead" needs a this.

  Three words were tried before `term_rules:` and all three were taken —
  `terms:` by dnt-check, `concepts:` by `profiles.<n>.concept`, and
  `preferred_terms:` (shipped in #2169) by the voice profile's
  `vocabulary.preferred_terms`. Map the whole family before renaming any of it;
  see `strategy/terms-vocabulary-alignment.md`.

  The persisted discriminators followed too (#1522): `model.Origin.Kind`, the
  KPZ content type, the sync `content_type` and apply change-set kinds all now
  write `"memory"`. That was possible because the standing decision is to
  **prefer resetting data over writing a migration** until the dogfood setup is
  proven — see `bowrain-infra/docs/runbooks/data-reset.md`, which also records
  that the trade is only correct while we are the only customer.

  What genuinely stays: `TMX`/`TBX` (external file-format standards we do not
  own), the `l10n-*` make targets (developer-facing internals on no user
  surface), the `termbase` SQLite migration bookkeeping table (renaming it makes
  `Migrate` replay every migration — see `terms/sqlite.go`), and the analytics
  property `method: "tm"` (event values are a rename boundary: PostHog holds the
  history, so renaming splits a metric rather than moving it).
- **Never hardcode counts the code controls** (formats, tools, providers,
  filters). Name the category and link to the generated reference.
- **Diagrams are real React components, never ASCII art.** The themed light/dark
  SVG kit is in `packages/docs-shared/src/diagram/` (`PipelineDiagram`,
  `StreamDiagram`, `PhaseFlow`, `RoundTripDiagram`, `LanesDiagram`,
  `SwimlaneDiagram`, `ArchitectureDiagram`, `RedactionDiagram`,
  `AxisLadderDiagram`, `AxisFamiliesDiagram`, `CycleDiagram`,
  `GatedLoopDiagram`), each with a
  story under **Diagrams** in the Kapi Storybook. Code fences are for code only:
  CLI output, file trees, config snippets — not flows or relationships.

### The tells that make prose read as machine-written

An LLM writing English drifts toward a narrow band of constructions that rate
well sentence by sentence and read as slop in aggregate. They are catalogued
(Wikipedia's *Signs of AI writing*; the `avoid-ai-writing` skill), and this repo
has shipped most of them. Check for these before committing prose:

- **Em dashes.** The strongest single signal. Target zero; one per 1,000 words
  is the ceiling. Use a comma, a full stop, or two sentences.
- **Negation reveals.** "It does not write it back", "this is not X, it's Y".
  State the positive: "It reads these files and never writes them."
- **Abstract subjects doing human things.** "The collection names no target",
  "the recipe decides", "the check wants". Name the actor, or rewrite around the
  behaviour: "a collection with no `target:` is source-only".
- **Aphorism endings.** A paragraph that closes on a quotable turn ("naming no
  target is the whole statement") is building toward a phrase rather than
  finishing a thought. Cut the flourish.
- **Triads.** Three parallel items, over and over. Vary the grouping, or use two.
- **Significance labels.** "worth stating plainly", "the real question is", "this
  is the interesting part", "load-bearing". Delete the label; keep the claim.
- **Diff-anchored writing.** "This used to…", "now finished", a note describing
  work as open. Docs describe the current state; history lives in git. (The same
  rule applies to code comments: describe what the code does, not what it used to.)

Two more that apply specifically to end-user surfaces:

- **Implementation vocabulary.** A user reads what a thing does for them, not how
  it is built. tree-sitter, ONNX, cgo and the daemon model belong in
  `web/docs/contribute/`, never in a recipe guide. Check the generated surfaces
  too: a format's manifest `display_name` becomes the **title** of its reference
  page, so jargon there ships to users without passing any prose review.
- **Rejected-design rationale.** Why we did not build the other thing is
  architecture-note material. A user guide says what to do.

The register stays academic and restrained; none of this licenses marketing
voice. Prefer the plainest sentence that is still precise.

State each topic once and cross-link; verify every command, flag, import path,
and flow name against the code before publishing.

## Where things are documented

- **Architecture decisions** — `web/docs/contribute/architecture/`, in six
  series by concern, one directory each: **F** foundations (`foundations/`),
  **E** engine (`engine/`), **C** context (`context/`), **S** surfaces
  (`surfaces/`), **M** multilingual (`multilingual/`), **A** assurance
  (`assurance/`). An AD is named for its series and position — `C-03`,
  `e-01-processing-engine.md` — and the sidebar autogenerates from the tree, so
  adding one means adding a file with a `sidebar_position`.

  Each AD describes the *current* state of its subsystem and carries no
  decision history — that lives in git. When a subsystem evolves, **update the
  existing AD in place** rather than appending a new one. Create a new AD only
  for a genuinely new concern, and retire one by redirecting its slug in
  `web/docusaurus.config.ts`.

  The corpus names no platform: `scripts/check-docs-bowrain-clean.sh` sweeps it
  alongside the user-facing docs trees.
- **Implementation notes** — `web/docs/contribute/implementation/`. Tactical
  detail (SQL schemas, API routes, pseudocode) kept out of the ADs.
- **Contributor guides** — `web/docs/contribute/`: `formats.md`,
  `tool-authoring.md`, `flow-authoring.md`, `plugins.md`, `testing.md`,
  `interfaces.md`. These are published: they document how to extend neokapi.
- **Repo internals** — `docs/internals/`: format ops, testing strategy, the
  dogfood loop in CI ([l10n-ci.md](docs/internals/l10n-ci.md)), the i18n Toil
  Index, docs-asset runbooks, brand communication, workspace paths. Unpublished
  by design — contributor machine setup and working notes, not product
  documentation.
- **kapi agent skill** — `cli/skills/data/kapi/` (`SKILL.md` + `references/`),
  the shipped guidance for driving kapi itself.

Terminology maps from Okapi: Filter → DataFormat, Step → Tool, Pipeline → Flow,
PipelineDriver → Executor, Event → Part, TextUnit → Block, TextFragment → `[]Run`.

Testing convention: `stretchr/testify` (assert/require), table-driven, `*_test.go`
colocated with implementation, roundtrip validation for formats.
