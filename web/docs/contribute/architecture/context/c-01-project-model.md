---
id: c-01-project-model
sidebar_position: 1
title: "C-01: The project model"
description: "Architecture decision: a kapi project is a folder with a kapi.yaml recipe at its root. The .kapi/ directory holds checkout-local data, and the project's context lives in a per-user workspace outside every checkout."
keywords: [kapi project, kapi.yaml, .kapi, YAML recipe, project model, context, workspace, store.db, architecture decision, neokapi]
---

# C-01: The project model

## Summary

A kapi project is a folder containing a `kapi.yaml` recipe at its root. The
recipe captures the user's declarative intent (identity, content collections,
flows, governance bindings) plus any extension blocks a plugin has registered a
schema for. Commit it together with the agent configuration and any flow files.

The project's **context** lives in a per-user workspace outside every checkout,
one database per project, reached by every clone and worktree of it
([C-03](c-03-context-store-and-graph.md)).

`.kapi/` holds checkout-local data: the derived block store and caches, personal
reader filters and any redaction vault. `kapi init` excludes the directory from
version control. Derived data can be rebuilt; personal filters and withheld
originals in the vault require separate preservation.

A `ProjectContext` resolves the recipe into a runtime configuration, and a
`Store` interface with pluggable providers gives tools random-access storage
beyond the streaming pipeline.

## Context

Real work needs to persist more than an in-flight stream of parts. Translators
add targets over time, per locale. Several tools contribute independent
annotation layers. Re-running a flow must not re-translate blocks whose source
has not changed. Content collections group heterogeneous source files with
different formats, writer outputs and language targets.

The channel-based `Part → Tool → Part` model
([E-01](../engine/e-01-processing-engine.md)) is a forward-only transform. It
does not cover random-access reads, incremental work, or parallel tools writing
independent annotation layers. A declarative recipe captures intent; a local
store captures the working state.

The project folder is the **day-to-day working unit** and the default unit users
share, back up and commit. It is discovered by a git-style upward walk, never
named on a command. Sharing a project usually means sharing the folder. When a
folder cannot travel (a hand-off to a translator, an archive, an air-gapped
transfer), the single-file `.kpz` parcel carries the same state losslessly
([M-06](../multilingual/m-06-content-packages.md)), and a task-scoped bilingual
`.kpz` is what goes to a reviewer
([M-01](../multilingual/m-01-bilingual-interop.md)). The parcel is for
boundaries, not a competing working model: you open it into a project, work in
the folder, and pack to ship.

## Decision

### Project layout

Ownership zones at the project root:

```
my-app/
├── kapi.yaml                   ← RECIPE (user edits; committed)
├── .mcp.json                   ← agent wiring written by kapi init (committed)
├── .claude/skills/kapi/SKILL.md
├── .kapi/                      ← THIS CHECKOUT'S CACHE (ignored as a whole)
│   ├── .gitignore              ← `*`, written by kapi init
│   ├── filters.local.json      ← personal saved filters
│   └── work/
│       ├── store.db            ← this checkout's projection (C-03)
│       ├── vault/              ← withheld originals (C-10), local-only
│       └── cache/              ← free to delete, always
│           ├── extractions/    ← per-extract batch state (M-01)
│           ├── redaction/      ← per-batch vault sidecars (C-10)
│           ├── refs.json       ← the observed freshness refs (C-05)
│           └── collections/    ← overlay layers per collection
├── flows/                      ← file-per-flow definitions named by flows_dir (E-04), when used
├── src/                        ← authored sources (user-owned)
└── i18n/                       ← writer output (generated)
```

Ownership, zone by zone:

- **`kapi.yaml`** is the user's. Hand-edited YAML, committed. A fixed,
  conventional config filename in the family of `package.json` or a CI workflow
  file: because it ends in `.yaml`, every editor and every code host's preview
  and diff apply YAML highlighting with no custom file-type registration. Wide,
  zero-config recognition was chosen over a branded document type.

  The same reasoning governs every other artifact kapi writes: each ends in the
  suffix of the serialization it actually is. Where a file also needs to say
  *which* document of that serialization it is, the marker goes in a segment
  ahead of the suffix (`.kbf.json`, `.memory.json`, `.overlays.jsonl`), so
  `jq`, diffs and highlighting keep working while the name stays self-describing.
  Only `.kpz`, a binary zip nobody hand-edits, keeps a dedicated extension.

- **The agent wiring** is the MCP entry and the short skill `kapi init` writes
  for the coding agents that work in the project
  ([S-01](../surfaces/s-01-kapi-cli.md)). It is committed with the recipe.

- **`.kapi/`** holds data specific to this checkout.
  `work/store.db` is the projection of the working tree: the block cache, the
  overlays a flow wrote, the extraction stamps
  ([C-03](c-03-context-store-and-graph.md)). Beside it sit the caches, the
  redaction vault ([C-10](c-10-redaction.md)) and the personal saved reader
  filters with the choice of the active one. Shared reader filters are project settings kept in the context store
  (`core/projectdb.SettingSavedFilters`), and a project's flow files sit in the committed
  directory its recipe names with `flows_dir:`
  ([E-04](../engine/e-04-flows-and-io-binding.md)), and kapi reads nothing
  from `.kapi/flows/`.
  A second clone and a git worktree each keep a `.kapi/` of their own and share
  one context store.

- **Context files** are an input to the store, never a source a gate reads.
  `kapi context import <dir>` reads a directory of them, a terms bundle, a voice
  profile, content-memory bundles and decision-record shards, into operations
  ([C-11](c-11-context-operations.md)). The context itself moves between machines
  through a context backend or a transfer file ([C-03](c-03-context-store-and-graph.md)).
  With no path, import reads `.kapi/`, so a project that commits a snapshot
  there keeps its own `.kapi/.gitignore`, which kapi never overwrites. The
  recipe names none of these files: it binds the voice and terms they carry by
  name.

- **`src/**`** is user-authored content. Referenced by the recipe; never moved
  into `.kapi/`.

- **Writer outputs** are produced by the format writers the recipe declares. The
  application runtime consumes these; kapi does not.

### One directory, one ignore rule

`core/project.EnsureLayout` writes `.kapi/.gitignore` holding one line, `*`, so
the whole directory, the rule included, stays out of version control. The rule
is the constant `core/project.StateGitignore`.

It is written only into a `.kapi/` that is new or holds nothing but what kapi
keeps there. A directory that already carries other files, such as committed
context files, has an ignore arrangement of its own, and a rule that ignored
everything would silently keep the next of those files out of the commit. A
rule already present is never rewritten.

What deleting costs, stated exactly:

- `rm -rf .kapi/work/cache` is **always free**. Everything under it is rebuilt on
  the next run.
- `rm -rf .kapi` costs a re-extraction and one more thing: the redaction vault
  under `.kapi/work/vault/` holds withheld originals that are **local-only and
  not regenerable** ([C-10](c-10-redaction.md)): never committed, never synced,
  so nothing anywhere else has a copy. The project's terms, voice profiles,
  content memory and decisions are in the workspace and stay
  ([C-03](c-03-context-store-and-graph.md)).

### How a project is created

`kapi init` scans the directory (honouring every `.gitignore`, skipping hidden,
dependency, vendored and build directories) and writes a recipe with a
collection for each kind of content it recognises. The format registry decides
what a document is: a built-in format whose family carries documents (marked-up
text, office documents, timed text) is content wherever it sits, grouped by
top-level directory and extension, with a file at the root as an entry of its
own. i18n catalogs are recognised by the framework presets' layouts, or by a
path that names the source language; a file named for any other language is a
translation and is left out. Each collection carries a comment saying what it
matched. The scan is deterministic and asks nothing (`host.ProposeCollections`).

On a project that already has a recipe, `kapi init` leaves its collections
alone and reports the content no collection reads yet, as lines to paste.

### Recipe schema

The recipe is a YAML document parsed into `core/project.KapiProject`:

```yaml
# kapi.yaml
version: v1
id: prj_k3m7xq2vw9ta5r8ybn4dcp
name: Northsea App

profiles:
  northsea:
    channels: [app, docs]
    voice:
      profile: northsea

collections:
  - name: ui
    channel: northsea/app
    content:
      - path: "src/**/*.{tsx,jsx}"
        format:
          name: exec
          config:
            command: "vp neokapi-i18n extract --stream"
        target: "i18n/{lang}.json"

plugins:
  okapi: "^1.47.0"

flows:
  translate:
    steps:
      - tool: recycle
        config:
          fuzzyThreshold: 75
      - tool: translate
      - tool: qa

defaults:
  source_language: en
  target_languages: [fr, de, ja]
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
  tools:
    redact:
      detectors: [rules]
```

`defaults.tools` holds project-level **tool presets**: per-tool config defaults
applied wherever that tool runs in a project flow. A flow step's own config
overrides the preset per key, so a project pins its redaction rules or a
pseudo-translation prefix once while an individual flow refines them. Resolution
happens at tool construction, and the data-flow and placement gates
([E-03](../engine/e-03-tool-system.md)) validate against the same merged config
the runtime uses: a preset that enables the redact tool's entity detection makes
the upstream `entity` port required exactly as an inline config would. A tool's
config keys are its JSON field names, in camelCase.

Required fields: `version: v1`, which must equal the current schema version, and
a non-empty `path` on each content item. Every flow contains at least one step
with a non-empty `tool`, unless the step uses `parallel`, in which case the
parallel branches carry the tools. `name` is the project's human label. Since
the recipe filename is fixed, it is the only place the label lives; `kapi init`
defaults it to the current directory's basename.

The other recipe families each have an AD or a reference section of their own:

- `defaults.coordinates` and a collection's `coordinates:` place content on the
  declared axes of the context space; `profiles:` and `channel:` derive the
  structural ones ([C-02](c-02-coordinates-and-governance.md)).
- `source_only: true` on a collection asserts that it has no target language:
  a run reads and checks it and writes nothing back. A collection that sets it
  and also carries a target is rejected at load.
- `defaults.flow` names the flow `kapi up` runs; `defaults.source_gate`,
  `ship_gate`, `ship_gates`, `verified_gate` and `gates` are the convergence
  gates ([Convergence](/kapi/convergence)).
- `defaults.materialize` governs delivery of target-language files. With
  `manual`, the default, a pass writes where the recipe points as it produces
  each unit, and delivery is an explicit `kapi merge` or `kapi up --materialize`.
  With `on-converge` the run owns delivery under the ship gate: its passes draft
  into a run-local tree, and only a locale whose gated scopes are all shippable
  has its files written to the collection's `target:` path.
- `defaults.annotations` decides which stand-off annotations a writer draws
  inline; `defaults.locales` holds per-target-language tool presets;
  `defaults.formats` holds per-format reader configuration and detection
  priorities (below).

The [project file reference](/reference/project-file) lists every key.

The recipe holds provider **names** only. Credentials live in the OS keychain
([S-01](../surfaces/s-01-kapi-cli.md)) or the environment. Nothing in the recipe
is secret; it is safe to commit.

Discovery is git-style: kapi walks up from the current directory until it finds a
file named exactly `kapi.yaml` (`core/project.ResolveLayout`). A directory holds
at most one, so discovery is unambiguous; an explicit `-p <path>` overrides it,
and `KAPI_NO_PROJECT=1` opts out of discovery entirely.

### Project identity

`id:` is the stable project identity; `name:` is an editable display label.

The id is minted once, by `kapi init`, out of `crypto/rand` and nothing else.
No part of it is derived from the name, the folder or the repository's origin,
so editing any of those leaves it alone. The form is `prj_` followed by
lowercase RFC 4648 base32 of 128 random bits, which leaves the value
case-insensitive and free of every character that needs quoting in YAML, a URL
or a shell word (`core/project.NewID`, checked on load by `ValidateID`).

Everything kapi records about a project locally is keyed on
`KapiProject.Identity()`, the id when the recipe carries one. Two keys derive
from it: the workspace's `ProjectKey`, which decides which context store the
project reaches ([C-03](c-03-context-store-and-graph.md)), and the context
graph's scope tuple, which `host.ProjectScope` writes every node and edge id
under. What a project can do without disturbing either follows directly:

- Rename it. The label is one line in the recipe, and editing it leaves every
  derived key where it stands.
- Move the folder, or clone the repository somewhere else. No key is derived
  from a path.
- Work in two checkouts at once. Both carry the same recipe and therefore the
  same id, so a per-user store keyed by project reads them as one project.

`Identity()` falls back to `name:` for a recipe that carries no `id:`. Such a
project keeps working with the name as its key and no warning about it, and
`kapi init --mint-id` writes an id into it through the ordinary recipe setter
(`project.SetField`, saved over `core/yamledit`), so the file keeps its
comments, its blank lines and its key order. A recipe stating neither is keyed
by the checkout it was found at (`workspace.KeyForCheckout`, an `at_` prefix
over a digest of the normalized path), so it still has a context store of its
own, and a second checkout of it gets a second one. Minting an `id:` is what
joins them.

The setter rejects clearing or replacing an existing id, which would disconnect
the project from its recorded context. Turning a copied recipe into a genuinely different project means
deleting the `id:` line and minting a fresh one.

Unknown top-level keys are preserved rather than rejected (see [Recipe extension
mechanism](#recipe-extension-mechanism)), so a kapi release built before this
field reads a recipe carrying an `id:` and identifies the project by its name.
The near-miss reporter stays quiet about it as well, since `id` resembles no
field of `KapiProject`.

### Content paths

Each content item's `path` is a
[doublestar](https://github.com/bmatcuk/doublestar) glob: `**` matches across
directories and `{a,b,c}` matches alternatives, so a single glob covers a
directory of mixed content with the format detected per file.

A collection's `base` is the directory it lives in: every `path`, `target` and
item `base` below it is written relative to that directory and joined onto it, so
a collection reads as the tree it governs rather than as a prefix repeated on
every line. An item's own `base` is the narrower thing, the directory a matched
file's path is made relative to, which drives the path tokens. It defaults to the
glob's fixed prefix.

`target` is a path template expanded per file and language. The common case is
**directory-mirror**: when the target names a directory, the source path relative
to `base` is reproduced under it, so `target: output/{lang}` turns
`input/docs/api.md` into `output/fr/docs/api.md`. For custom layouts, tokens
(`{lang}`, `{relpath}`, `{path}`, `{dir}`, `{filename}`, `{name}`/`{basename}`,
`{ext}`) reshape the path explicitly. The resolver is
`core/project.ResolveTargetPath`; the token reference lives in the
[project file reference](/reference/project-file#source-and-target-paths).

### Recipe extension mechanism

`KapiProject` carries an `Extras map[string]yaml.Node` field with `yaml:",inline"`
on `KapiProject`, `Defaults`, `Collection` and `ContentItem`. Unknown YAML keys
are captured as raw nodes; a layer built on the framework declares its own typed
schema and decodes from `Extras` at load time. The framework knows nothing about
those extensions and round-trips them verbatim.

A package registers schemas at `init()`:

```go
coreproj.RegisterExtensionGroup("myplugin", []coreproj.Extension{
    {Name: "myplugin", Scope: coreproj.ScopeProject, Decoder: venueDecoder, Venue: true},
    {Name: "hooks", Scope: coreproj.ScopeProject, Decoder: hooksDecoder},
})
```

`Scope` distinguishes which `Extras` map a key belongs to: `ScopeProject`,
`ScopeDefaults`, `ScopeCollection` or `ScopeItem`. Each `(Scope, Name)` binds to
one decoder. `KapiProject.Validate()` walks every Extras map and runs the
matching decoder; keys with no decoder registered round-trip without error, so
binaries with different plugin sets linked in stay forward-compatible. An
unknown key that is one edit away from a known field of the same struct is a
likely typo rather than an extension, and `KapiProject.KeyWarnings` reports it
with the field it resembles.

A recipe declares a hard dependency with `requires:`, a map of plugin name to
version constraint (`"*"` for any version; semver forms such as `^1.0` are also
accepted). Validation fails when no extension under the named group has been
registered:

```yaml
version: v1
requires:
  myplugin: "*"
myplugin:
  url: https://platform.example.com/team/proj
```

A binary that does not link the `myplugin` extensions rejects this recipe with an
actionable error rather than silently ignoring the block. A recipe without
`requires:` loads in any binary and the extras pass through.

The venue client in `host/venue` registers one collection-scoped extension key
of its own, `preview`: where a collection's strings can be read in place (a
component explorer or a running site), as a kind and a URL. It is decoded only
when the recipe also carries the venue key, it travels on the collection's
context entry with the coordinates and the governance, and it is folded into
that entry's hash so that re-pointing a collection reconciles. A binary without
the venue client round-trips it like any other extra.

Implementation detail (the `Scope` enum, the decoder helpers and a worked
example) lives in [Note: Plugin model](../../implementation/engine/plugin-model.md).

### Converging somewhere else

The framework defines the sync wire (`core/venue`: the context content type,
refs and tree shapes both sides hash the same way) and a client for it
(`host/venue`), but no venue key of its own. Server, account and venue are not
recipe fields. A layer that adds one builds it on the generic mechanism above: it
registers a `ScopeProject` extension under its own name and gates it with
`requires:`, so a recipe carrying the key is meaningful only where that plugin
is installed.

One thing the framework does need from such a key is whether the project
converges somewhere other than this machine. It asks the registry rather than
looking for a name. An extension registered with `Venue: true` claims that role
(a plugin manifest declares it as `"venue": true` on a schema extension), and the
framework then reads exactly two fields out of the block, `url:` and `converge:`,
through `KapiProject.Venue()`. Which layer provides the venue, and what its key
is called, stays that layer's business: a recipe key kapi cannot name is a key it
cannot grow an opinion about. An unregistered key of the same name reports no
venue and no opinion.

### Content collections

A `Collection` lists the source patterns kapi extracts from and the reader used
for each. Extracted blocks flow through the project's executor; persistent block
state (hashes, per-locale targets, annotations) lives in the project store.

For subprocess-based extractors, the format is `exec`: kapi runs the declared
command once per collection with every matched file path streamed on stdin
(NUL-separated) and reads NDJSON block records from stdout. The developer picks
the package manager or binary path. kapi runs whatever `command` says verbatim,
which is why the exec class is gated by explicit consent
([E-06](../engine/e-06-execution-trust.md)).

### The store interface

Flows and tools read and write blocks and overlays through the `Store` and
`Session` interfaces (`core/blockstore`), not through raw channels. The streaming
contract is preserved as one capability among several.

```go
type Store interface {
    Begin(ctx context.Context) (Session, error)
    Capabilities() Capabilities
    Close() error
}

type Session interface {
    Capabilities() Capabilities
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
    Persistent   bool
}
```

Two providers back it: an ephemeral map-based one for tests and ad-hoc CLI
invocations, and the block tables inside `.kapi/work/store.db` for long-lived
project work. Tools never open the database directly; they operate on a session.
In the browser the block store is a path-keyed in-memory implementation
([C-03](c-03-context-store-and-graph.md)); nothing above the `Store` interface
notices.

A tool that needs random access implements the optional `SessionTool` extension
(`core/tool/session.go`), which adds a session handle alongside the same
streaming channels:

```go
type SessionTool interface {
    Tool
    SessionProcess(
        ctx context.Context,
        sess blockstore.Session,
        in <-chan *model.Part,
        out chan<- *model.Part,
    ) error
}
```

The executor opens one session per run, dispatches each stage through
`SessionProcess` when the tool implements it, and owns the transaction boundary:
tools must not call `Commit`/`Rollback` themselves. `SessionTool` is the path for
term enforcement, multi-pass statistics and checks that read the whole store.

### ProjectContext

A `ProjectContext` (`core/project`) bridges the static recipe and the live
runtime. Every consumer that runs in project mode constructs one:

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
```

`AllowedSources` derives from the `plugins` section. It always includes
`"built-in"` plus each declared plugin name; a project without a `plugins`
section sees built-in formats only.

**Project-scoped detection.** `ProjectContext.DetectFormat` delegates to
`FormatRegistry.Detect(path, DetectOptions{AllowedSources, PriorityOverrides})`.
Detection is content-aware, since the file head disambiguates a shared
extension, and takes per-call priority overrides. When a plugin is installed
globally but the project does not declare it, its formats are excluded and
built-ins are used instead. Explicitly declared formats on a content item bypass
detection entirely.

The overrides come from `defaults.formats.<format>.priority`: where an extension
is claimed by several formats at equal priority, a recipe steers detection by
bumping the preferred engine. This lets a single wildcard content item
auto-detect the right engine per file instead of pinning one format per
extension. The override is applied per detection call rather than by mutating the
registry, so concurrently open projects with different priorities do not race.

**Content resolution.** `ProjectContext.ResolveContent` matches content patterns
against the filesystem, applies ignore rules, detects formats and returns
`[]ResolvedFile`. Both the CLI and the desktop app use this one implementation.

**Reader and writer configuration.** `ConfigureReader` applies a format's
`FormatDefaults.Config` overrides from `defaults.formats.<format>` onto the
reader's config; it takes the `Configurable` interface (any component exposing
`Config() format.DataFormatConfig`) and is a no-op when the project declares no
defaults for that format. `ConfigureWriter` sets the writer's encoding from the
project defaults. Preset selection is resolved separately, through
`resolver.ResolveFormatConfig`, which merges the named preset's config before the
reader is opened.

Reader config decides which text a reader emits at all, so it is part of a
content unit's identity rather than a detail of one code path: the config a
collection declares is `defaults.formats.<format>.config` overlaid by the
content item's own `format.config`, and every path that reads a declared file
reads it under that merged config. That covers the flow run, extract, merge, and
equally the measurement paths (coverage, status, the review queue, the bilingual
checks), which resolve it per unit as they resolve content. A measurement blind
to the config counts a different set of units than the run produced, so the
percentage is a fraction over two denominators.

**Plugin scoping** generalizes beyond detection: `AllowedTools()` filters the tool
registry to tools from declared plugins plus built-ins, presets from undeclared
plugins are excluded from preset selectors, and flows referencing tools from
undeclared plugins produce warnings during validation.

### Bookkeeping

`.kapi/work/cache/extractions/<batch-id>/manifest.yaml` records each `kapi
extract` run ([M-01](../multilingual/m-01-bilingual-interop.md)): the emitted
source→output pairs, per-file source SHA-256, leverage counts, the bilingual
format version and the skeleton filenames. The batch id is stamped in each
emitted bilingual file so `kapi merge` resolves a returning file back to the
right extraction without guessing from its name.

## Consequences

- Incremental work: re-running a flow translates only blocks whose source hash is
  not already recorded for the target locale.
- Concurrent tools: term match and memory lookup run in parallel, each writing an
  independent overlay layer.
- Transaction semantics vary per provider, and tools calling `GetBlock` per block
  are slow against a remote store.
- The recipe is always free of credentials, so it is safe to commit and to share.
- The recipe binds by name what governs a point (`defaults.voice`,
  `profiles.<n>.termstore`) and names no context file. A recipe that names one
  fails to load with the import that reads it. No binding reaches a checkout on
  a read path, so two branches of one project answer the same question the same
  way.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  point a collection sits at and what governs it.
- [C-03: The context store and graph](c-03-context-store-and-graph.md):
  `.kapi/work/store.db` and the workspace holding the other half.
- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md):
  the ledger and the shards a snapshot writes.
- [E-01: Processing Engine](../engine/e-01-processing-engine.md): flow
  execution.
- [E-03: Tool System](../engine/e-03-tool-system.md): the `Tool` and
  `SessionTool` interfaces.
- [Flow steps format](../../implementation/engine/flow-steps-format.md): the shared
  flow syntax.
- [kapi.yaml project file](../../implementation/context/kapi-project-file.md): the
  schema reference.
