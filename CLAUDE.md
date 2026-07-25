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
| Kapi Desktop — Wails v3 app | `apps/kapi-desktop/` | framework, host, `bowrain/plugin/schema` |
| Bowrain Core | `bowrain/core/` | framework, `bowrain/plugin/schema` |
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
- **License boundary.** Framework, host, cli, kapi, kapi-desktop and
  `bowrain/plugin/schema` are Apache-2.0; the rest of `bowrain/` is AGPL-3.0.
  `bowrain/plugin/schema` has its own `go.mod` precisely so kapi-desktop can
  blank-import recipe vocabulary without taking on AGPL code. Keep it that way.
- **`kapi` contains zero vendor-plugin code.** Plugins (bowrain, okapi-bridge,
  sat, …) are discovered at runtime via the unified manifest model and
  dispatched as subprocesses. `bowrain/plugin/*` is blank-imported into
  `kapi-bowrain`, not into `kapi`. See
  [Note: Plugin model](web/docs/contribute/notes-internal/plugin-model.md).

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

## Dogfooding kapi: the in-repo isolation contract

This repo dogfoods kapi. A `kapi.yaml` recipe at the repo root is driven by the
**system-installed** kapi + plugins (real `kapi-bowrain`, real keychain auth,
real server), and it is auto-discovered by a git-style **upward walk** from any
cwd inside the tree (`core/project.ResolveLayout` → `cli.ResolveProjectPath`).

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
[Workspace Paths](web/docs/contribute/workspace-paths.md).

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

- **Narration sidecars are generated.** `demo.<lang>.yaml` files come from the
  dogfood recipe (`make l10n-demos`, drift-gated by `l10n-verify`). Fold fixes
  into `l10n/tm/demo-narration-<lang>.kmb` and regenerate — never edit a sidecar
  or author inline translations.
- **Assets are not in git and not in GitHub releases.** They live only on the
  S3 + CloudFront CDN (`$DOCS_CDN_URL`) and are referenced by URL via
  `ThemedVideo` / `ThemedImage`; docs CI references them rather than recording.
  See [CDN assets](web/docs/contribute/notes-internal/cdn-assets.md).

Recordings and screenshots run against **real** neokapi infrastructure — real
Keycloak OIDC via `compose.yaml`, the real `bowrain-server` binary, a real
SQLite database. Never mock the auth flow or the API. Third-party services
outside this project (MT providers, external LLM APIs) may be mocked.

Runbooks: [regenerating docs assets](docs/internals/regenerating-docs-assets.md),
[video revision](docs/internals/video-revision-runbook.md).

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
  Retained identifiers (recipe keys `tm:`/`termbase:`, `--termbase`, `tm.db`,
  `l10n-*` targets, `/translation-memory`, `TMX`/`TBX`) keep their spelling:
  describe the new concept and quote the old identifier verbatim.
- **Never hardcode counts the code controls** (formats, tools, providers,
  filters). Name the category and link to the generated reference.
- **Diagrams are real React components, never ASCII art.** The themed light/dark
  SVG kit is in `packages/docs-shared/src/diagram/` (`PipelineDiagram`,
  `StreamDiagram`, `PhaseFlow`, `RoundTripDiagram`, `LanesDiagram`,
  `SwimlaneDiagram`, `ArchitectureDiagram`, `RedactionDiagram`), each with a
  story under **Diagrams** in the Kapi Storybook. Code fences are for code only:
  CLI output, file trees, config snippets — not flows or relationships.

State each topic once and cross-link; verify every command, flag, import path,
and flow name against the code before publishing.

## Where things are documented

- **Architecture decisions** — `web/docs/contribute/architecture/`. Organized by
  concern, not chronologically; each AD describes the *current* state of its
  subsystem. When a subsystem evolves, **update the existing AD in place**
  rather than appending a new one. Create a new AD only for a genuinely new
  concern.
- **Implementation notes** — `web/docs/contribute/notes-internal/`. Tactical
  detail (SQL schemas, API routes, pseudocode) kept out of the ADs.
- **Contributor guides** — `web/docs/contribute/`: `formats.md`,
  `tool-authoring.md`, `flow-authoring.md`, `plugins.md`, `testing.md`,
  `interfaces.md`, `workspace-paths.md`.
- **Repo internals** — `docs/internals/`: format ops, testing strategy, i18n
  toil, docs-asset runbooks, brand communication.
- **kapi agent skill** — `cli/skills/data/kapi/` (`SKILL.md` + `references/`),
  the shipped guidance for driving kapi itself.

Terminology maps from Okapi: Filter → DataFormat, Step → Tool, Pipeline → Flow,
PipelineDriver → Executor, Event → Part, TextUnit → Block, TextFragment → `[]Run`.

Testing convention: `stretchr/testify` (assert/require), table-driven, `*_test.go`
colocated with implementation, roundtrip validation for formats.
