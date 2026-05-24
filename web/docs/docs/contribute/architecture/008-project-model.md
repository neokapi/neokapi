---
id: 008-project-model
sidebar_position: 8
title: "AD-008: Kapi Project Model"
---

# AD-008: Kapi Project Model

## Summary

A kapi project is a folder containing a `{name}.kapi` YAML recipe at its root
and a sibling `.kapi/` state directory. The recipe captures the user's
declarative intent — identity, content collections, flows, store selection,
plus an optional `server:` block when the project syncs with a bowrain server —
while `.kapi/` holds working state, with all regenerable caches under
`.kapi/cache/`. A `ProjectContext` resolves the recipe into a runtime
configuration, and a `BlockStore` interface with pluggable providers gives
tools random-access storage beyond the streaming pipeline.

## Context

Localization workflows need to persist more than an in-flight stream of parts:

- Translators add targets over time, per locale.
- Multiple tools (term lookup, TM leverage, QA) contribute independent
  annotation layers.
- Re-running a flow must not re-translate blocks whose source has not changed.
- Content collections group heterogeneous source files with different formats,
  writer outputs, and language targets.

The channel-based `Part → Tool → Part` model ([AD-004: Processing Engine](004-processing-engine.md))
is a forward-only transform. It does not cover random-access reads,
incremental work, or parallel tools writing independent annotation layers.

A declarative project file captures the user's intent (which plugins, which
collections, which flows). A local block store captures the working state.
The project folder is the unit users share, back up, and commit. No extra
archive format is required: sharing a project means sharing the folder.

## Decision

### Project layout

Three ownership zones at the project root:

```
my-app/
├── my-app.kapi             ← RECIPE (user edits, click-to-open)
├── .kapi/                  ← WORKING STATE (kapi maintains)
│   ├── manifest.yaml       ← bookkeeping: block counts, fingerprints, timestamps
│   ├── tm.db               ← project translation memory (AD-009) — authoritative
│   ├── termbase.db         ← project termbase — authoritative
│   ├── flows/              ← optional file-per-flow definitions (authored)
│   │   └── <flow>.yaml
│   └── cache/              ← all regenerable caches under one roof
│       ├── blocks.db       ← block store (SQLite, was `.kapi/cache.db`)
│       ├── sync-cache.json ← kapi push/pull state (only with server: block)
│       ├── extractions/    ← per-extract batch state (AD-017)
│       │   └── <batch-id>/
│       │       ├── manifest.yaml         ← source→output pairs, leverage, hashes
│       │       ├── skel-<src-hash>.bin   ← per-source skeleton for merge
│       │       └── suggestions.jsonl     ← sub-threshold TM matches
│       └── collections/    ← overlay layers per collection
│           └── ui/
│               ├── targets/{fr,de}.json
│               ├── annotations/{terms,tm-matches,qa}.json
│               └── skeletons/
├── src/                    ← authored sources (user-owned)
│   └── **/*.tsx
└── i18n/                   ← generated translations (format writer output)
    └── {de,fr}.json
```

Ownership:

- **`{name}.kapi`** — the user's. Hand-edited YAML. The click-to-open handle
  for kapi-desktop. Committed to git.
- **`.kapi/`** — kapi's. Authoritative state (`tm.db`, `termbase.db`,
  `manifest.yaml`) sits at the top level; all regenerable caches live under
  `.kapi/cache/` so users can blow them away without losing translation work.
  Gitignored by default; opt in to commit `.kapi/tm.db` / `.kapi/termbase.db`
  when cross-clone reproducibility matters.
- **`src/**`** — user-authored content. Referenced by the recipe; never moved
into `.kapi/`.
- **Writer outputs** (e.g. `i18n/{locale}.json`) — produced by format writers
  the recipe declares. The runtime consumes these; kapi does not.

The name pair mirrors git: `.gitignore` file plus `.git/` folder at the same
root.

### Recipe schema

The recipe is a YAML document parsed into `core/project.KapiProject`:

```yaml
# my-app.kapi
version: v1
id: my-app
name: My App Localization
sourceLocale: en-US
targetLocales: [fr-FR, de-DE, ja-JP]

content:
  - name: ui
    store:
      type: cache
      path: .kapi/cache/blocks.db
    items:
      - path: "src/**/*.{tsx,jsx}"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
      - path: "src/i18n/en/*.json"
        format: json
    writers:
      - format: json
        out: "i18n/{locale}.json"

plugins:
  - okapi@1.47.0

flows:
  translate:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check

  full-pipeline:
    steps:
      - tool: tm-leverage
        config:
          fuzzy_threshold: 75
      - tool: ai-translate
      - tool: qa-check

defaults:
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
```

Required fields: `version: v1`, `name`, and for each content item a non-empty
`path`. Every flow contains at least one step with a non-empty `tool` (unless
the step uses `parallel`, in which case the parallel branches carry the tools).

The recipe holds provider **names** only — API keys live in the OS keychain
(see [AD-013: Kapi CLI](013-kapi-cli.md)) or environment. Nothing in the
recipe is secret; it is safe to commit.

Discovery is git-style: kapi tools walk up from the current directory until
they find a `*.kapi` file. Multiple recipes at the same directory level
require an explicit `-p <path>` flag.

### Recipe extension mechanism

The framework recipe (`KapiProject`) carries an `Extras map[string]yaml.Node`
field with `yaml:",inline"` on `KapiProject`, `Defaults`, `ContentCollection`,
and `ContentItem`. Unknown top-level YAML keys are captured as raw nodes;
platform layers (e.g. bowrain) declare their own typed schema and decode
from `Extras` at load time. The framework knows nothing about platform-
specific extensions and round-trips them verbatim.

A platform package registers schemas at `init()`:

```go
coreproj.RegisterExtensionGroup("bowrain", []coreproj.Extension{
    {Name: "server", Scope: coreproj.ScopeProject, Decoder: serverDecoder},
    {Name: "hooks", Scope: coreproj.ScopeProject, Decoder: hooksDecoder},
    // ...
})
```

`Scope` distinguishes which `Extras` map a key belongs to: `ScopeProject`,
`ScopeDefaults`, `ScopeCollection`, or `ScopeItem`. Each `(Scope, Name)`
binds to one decoder. `KapiProject.Validate()` walks every Extras map and
runs the matching decoder; unknown keys (no decoder registered) round-
trip without error so binaries with different sets of plugins linked in
remain forward-compatible.

Recipes can declare a hard dependency via `requires:` — validation fails
when no extension under the named group has been registered:

```yaml
version: v1
requires: [bowrain]
server:
  url: https://bowrain.example.com/team/proj
```

A binary that doesn't link the bowrain extensions rejects this recipe
with a clear "binary not built with bowrain linked in" message. A recipe
without `requires:` loads in any binary; the extras pass through.

Implementation details — including the `Scope` enum, decoder helpers, and
a worked example — live in
[Note: Plugin model](../notes-internal/plugin-model).

### Optional bowrain-server connection

A recipe with no `server:` block is a pure local project. Adding a `server:`
block with a compound URL marks the project as bowrain-connected — `kapi
push`, `kapi pull`, `kapi status`, and friends operate against the
declared server. Kapi tools tolerate the `server:` block but ignore it.

```yaml
server:
  url: https://bowrain.example.com/my-team/abc123
  stream: $auto

# Top-level lifecycle policy (applies whenever the trigger fires):
hooks:
  pre-push: [qa-check]
  post-pull: [update-stats]

automations:
  - name: auto-translate-on-push
    trigger: post-push
    actions:
      - type: wait_translate
      - type: pull

# Top-level governance / content policy:
assets:
  enabled: true
  max_size: 100MB

brand_voice:
  profile: company-profile
  channel: marketing
```

Only the connection coordinates (`url`, `stream`) live under `server:`.
Lifecycle (`hooks`, `automations`) and governance (`assets`, `brand_voice`)
are top-level — they describe project-owned policy, not server identity.
See [AD-010](https://neokapi.github.io/web/bowrain/docs/architecture-decisions/010-bowrain-cli-and-project-model)
for the full bowrain workflow semantics.

Auth tokens for bowrain servers live in the OS keychain (the same store
kapi uses for LLM provider keys), keyed by server URL. Non-secret metadata
(server URL, user info, expiry) lives at `~/.config/bowrain/auth.json`.

### Content collections

A `ContentCollection` lists the source patterns kapi extracts from and the
format reader used for each. Extracted blocks flow through the project's
flow executor; persistent block state (hashes, per-locale targets,
annotations) lives in the project's block store.

For subprocess-based extractors (JSX via kapi-react, bespoke DSL walkers), the
format is `exec`:

```yaml
items:
  - path: "src/**/*.tsx"
    format:
      name: exec
      config:
        command: "vp kapi-react extract --stream"
```

Kapi runs the declared command once per collection with every matched file
path streamed on stdin (NUL-separated) and reads NDJSON block records from
stdout. The developer picks the package manager (`vp`, `pnpm`, `npm`, `yarn`,
or a direct binary path) — kapi runs whatever the `command` says verbatim.

Generated translations land wherever the recipe's writers point — typically
outside `.kapi/`.

### State manifest

`.kapi/manifest.yaml` is kapi's bookkeeping: block counts, per-source SHA-256
fingerprints for staleness detection, generator identity, and last-updated
timestamps. Users do not hand-edit it. Deleting it is safe — it rebuilds from
`cache/blocks.db`; nothing authoritative lives only in the manifest.

### Extraction manifests

`.kapi/cache/extractions/<batch-id>/manifest.yaml` records each `kapi extract`
run (see [AD-017](017-bilingual-format-interop.md)): the emitted
source→output pairs, per-file source SHA-256, TM leverage counts, the
XLIFF / PO version, and skeleton filenames. The batch id is stamped in
each emitted bilingual file so `kapi merge` can resolve a returning
file back to the right extraction without guessing from the filename.
Stale segments on merge are detected by comparing the manifest's
recorded source hash against the current source content.

The `Defaults.Merge` section of the recipe (`conflict_policy`) governs
how merge applies a translator's target when an on-disk target or TM
TU already exists. The `Defaults.TM` section (`fuzzy_threshold`,
`read`) governs TM pre-fill on extract. The `Defaults.Segmentation`
section (`source`, `srx`) toggles the SRX segmentation overlay — block
identity is stable across toggles, so a project can change these
fields between extractions safely.

### BlockStore interface

Flows and tools read and write blocks and overlays through `BlockStore`
(package `core/blockstore`), not through raw channels. The streaming contract
is preserved as one capability among several.

```go
type BlockStore interface {
    Begin(ctx context.Context) (Session, error)
    Capabilities() Capabilities
    Close() error
}

type Session interface {
    Blocks(filter BlockFilter) iter.Seq2[*Block, error]
    GetBlock(hash string) (*Block, error)
    PutBlock(collection string, b *Block) error
    GetOverlay(kind, blockHash string) (Overlay, error)
    PutOverlay(s Overlay) error
    ListOverlays(kind string) iter.Seq2[Overlay, error]
    Commit() error
    Rollback() error
    Close() error
}

type Capabilities struct {
    RandomAccess bool
    Concurrent   bool
    Remote       bool
    Writable     bool
}
```

### Block store providers

| Provider | Backing                           | Use case                                         |
| -------- | --------------------------------- | ------------------------------------------------ |
| `memory` | Go maps                           | ephemeral flows, tests, ad-hoc CLI invocations   |
| `cache`  | SQLite at `.kapi/cache/blocks.db` | default for kapi projects, long-lived local work |

Tools never open `cache/blocks.db` directly — they operate on a session. Swapping
defines the interface.

### Flow executor operates on a Session

```go
session, err := store.Begin(ctx)
if err != nil {
    return err
}
defer session.Close()

for _, t := range flow.Tools {
    if err := t.Process(ctx, session); err != nil {
        return session.Rollback()
    }
}
return session.Commit()
```

The existing channel-based `Tool` interface ([AD-006: Tool System](006-tool-system.md))
remains. A `SessionTool` extension (in `core/tool/session.go`) is provided for
tools that want random access — term enforcement, multi-pass statistics, QA
across the whole store.

### ProjectContext

A `ProjectContext` (package `core/project`) bridges the static recipe and the
live runtime. Every consumer that runs in project mode constructs one:

```go
type ProjectContext struct {
    Project        *KapiProject
    ProjectDir     string

    SourceLocale   model.LocaleID
    TargetLocales  []model.LocaleID
    AllowedSources []string
    Encoding       string
    Concurrency    int
    ParallelBlocks int
    LocaleFormat   string
    FormatDefaults map[string]FormatDefaults
}

func NewProjectContext(proj *KapiProject, projectPath string) *ProjectContext
```

`AllowedSources` derives from the `plugins` section. It always includes
`"built-in"` plus each declared plugin name. A project without a `plugins`
section sees built-in formats only.

### Project-scoped format detection

```go
func (ctx *ProjectContext) DetectFormat(
    reg *registry.FormatRegistry, path string,
) string
```

Delegates to `FormatRegistry.DetectByExtensionForSources(ext, ctx.AllowedSources)`.
When a plugin (say `okapi-bridge`) is installed globally but the project
does not declare it, plugin formats at higher priority are excluded and
built-in formats are used instead. Explicitly declared formats in content
items (`format: okf_json`) bypass detection entirely and are always honored.

### Content resolution

```go
func (ctx *ProjectContext) ResolveContent(
    reg *registry.FormatRegistry,
) ([]ResolvedFile, error)

type ResolvedFile struct {
    Path       string
    Relative   string
    Format     string
    Collection string
    Pattern    string
    Item       *ContentItem
}
```

Matches content patterns against the filesystem, applies ignore rules,
detects formats using project-scoped detection, and returns the resolved
file list. Both the CLI and kapi-desktop use this single implementation.

### Reader and writer configuration

```go
func (ctx *ProjectContext) ConfigureReader(
    r format.DataFormatReader, formatName string,
) error

func (ctx *ProjectContext) ConfigureWriter(
    w format.DataFormatWriter, formatName string,
) error
```

Applies `FormatDefaults` from the project: preset selection and config
overrides. If the project declares `defaults.formats.okf_html.preset: strict-extraction`,
`ConfigureReader` applies that preset before opening. No project defaults for
a format means no-op.

### Flow execution settings

The executor's `flow.ResourceContext` carries project-scoped execution
settings:

```go
type ResourceContext struct {
    ProjectDir     string
    OutputDir      string
    SourceLocale   string
    TargetLocale   string
    ToolName       string

    Concurrency    int
    ParallelBlocks int
    Encoding       string
    FormatDefaults map[string]FormatDefaults
}
```

CLI flags and desktop UI settings override project defaults when explicitly
set. The project provides defaults, not mandates.

### Plugin scoping

`AllowedSources` generalizes beyond format detection:

- **Tool scoping** — `AllowedTools()` filters the tool registry to tools from
  declared plugins plus built-ins. The flow editor lists only available tools.
- **Preset scoping** — framework presets from undeclared plugins are excluded
  from preset selectors.
- **Flow validation** — flows referencing tools from undeclared plugins
  produce warnings during project validation.

### Sharing and CLI integration

A project is a folder. Sharing means sharing the folder — git, tarball, rsync.
Kapi does not prescribe a bundling format.

The kapi CLI ([AD-013: Kapi CLI](013-kapi-cli.md)) uses projects via the `-p`
flag or through `kapi init`:

```bash
kapi init                                     # scaffold {name}.kapi + .kapi/
kapi run translate -p my-app.kapi             # run a declared flow
kapi ai-translate -p my-app.kapi              # tool runs against the project
kapi pseudo-translate -i file.json            # tool runs ad-hoc, no project
```

kapi-desktop ([AD-014: Kapi Desktop](014-kapi-desktop.md)) opens `.kapi` files
as documents and operates on the project folder.

## Consequences

- Incremental work: re-running a flow translates only blocks whose source hash
  is not already in `targets/<locale>`.
- Concurrent tools: term match and TM lookup run in parallel, each writing an
  independent overlay layer.
- Multi-pass tools: compute statistics across the whole store, then use them
  in a second pass.
- Transaction semantics vary per provider: SQLite transaction for `cache`,
  tools calling `GetBlock` per-block are slow against remote stores.
- The project file is always free of credentials — safe for commit and sharing.

## Related

- [AD-002: Content Model](002-content-model.md) — Block, Fragment, Span
- [AD-004: Processing Engine](004-processing-engine.md) — flow execution
- [AD-006: Tool System](006-tool-system.md) — Tool and SessionTool interfaces
- [AD-013: Kapi CLI](013-kapi-cli.md) — CLI use of projects
- [AD-014: Kapi Desktop](014-kapi-desktop.md) — desktop app use of projects
- [Flow Steps Format](/contribute/notes-internal/flow-steps-format) — shared flow syntax
- [.kapi Project File](/contribute/notes-internal/kapi-project-file) — schema reference
