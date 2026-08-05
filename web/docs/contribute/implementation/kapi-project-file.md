---
sidebar_position: 7
title: kapi.yaml Project File Format
description: Implementation note for AD-008 — the KapiProject YAML schema, ContentCollection/ContentItem and Defaults struct layouts, how extension extras are decoded, and how the kapi.yaml recipe is loaded, validated, and saved.
keywords: [kapi project file, kapi.yaml, KapiProject, YAML schema, ContentCollection, ContentItem, Defaults, project model, implementation note]
---

# kapi.yaml Project File Format

Implementation notes for the `kapi.yaml` project file format. See [AD-008](/contribute/architecture/008-project-model) for the architectural decision.

## Schema

The `kapi.yaml` recipe is a YAML document parsed by `core/project.KapiProject`:

```go
type KapiProject struct {
    Version  string                     `yaml:"version"`
    Name     string                     `yaml:"name,omitempty"`
    Plugins  map[string]PluginSpec      `yaml:"plugins,omitempty"`  // name → spec (scalar = version short form)
    Defaults Defaults                   `yaml:"defaults,omitempty"` // project-wide defaults (locales live here)
    Content  []ContentCollection        `yaml:"content,omitempty"`
    Preset   string                     `yaml:"preset,omitempty"`
    Flows    map[string]*flow.StepsSpec `yaml:"flows,omitempty"`
    Coordinates Coordinates             `yaml:"coordinates,omitempty"` // axis → declared values (see Context coordinates)
    Profiles    []ProfileBinding        `yaml:"profiles,omitempty"`    // governance bound to a coordinate match
    Requires RequiresMap                `yaml:"requires,omitempty"` // plugin name → semver constraint
    Extras   map[string]yaml.Node       `yaml:",inline"`            // unknown keys (platform extensions)
}

// Defaults holds project-wide processing defaults — including locales.
type Defaults struct {
    SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty"`
    TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
    Concurrency     int              `yaml:"concurrency,omitempty"`
    ParallelBlocks  int              `yaml:"parallel_blocks,omitempty"`
    Encoding        string           `yaml:"encoding,omitempty"`
    // (also: locale_format, formats, exclude, merge, memory, segmentation,
    //  redaction, brand_voice, terms_source, memory_source — see
    //  core/project/project.go.)
}

// ContentCollection is either a bare entry (path/format/target) or a named
// collection (name + items), and can carry its own source/target languages.
type ContentCollection struct {
    Name            string           `yaml:"name,omitempty"`
    SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty"`
    TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
    Items           []ContentItem    `yaml:"items,omitempty"`
    Base            string           `yaml:"base,omitempty"`   // dir items' paths are made relative to; items inherit it
    Context         map[string]string `yaml:"context,omitempty"` // the point this content sits at (named collections only)
    // Bare-entry fields (short form):
    Path   string      `yaml:"path,omitempty"`   // doublestar glob for source files
    Format *FormatSpec `yaml:"format,omitempty"` // format ID; auto-detect per file if empty
    Target string      `yaml:"target,omitempty"` // output path template (tokens below)
}
// ContentItem additionally carries its own `base` (yaml:"base,omitempty"),
// falling back to the collection's Base when empty.
```

Flow definitions reuse `core/flow.StepsSpec` and `core/flow.FlowStep` (see [flow-steps-format](./flow-steps-format.md)).

## Content model

`Content` is a list of `ContentCollection` values. Each entry is one of two
shapes, distinguished by `ContentCollection.IsBareEntry()`:

- **Bare entry** — has a `path` and no `items`. The `path`, `format`, and
  `target` fields are promoted onto the collection directly. Use this for a
  single glob with no grouping.
- **Named collection** — has a `name` and a non-empty `items` list of
  `ContentItem`, and may set its own `source_language` / `target_languages`.
  Use this to group related patterns and scope languages per group.

### Context coordinates

Content is written for a point in a **context space** the project defines. A
recipe declares its axes under `coordinates:`, binds governance to regions of
that space under `profiles:`, and each named collection names its point once:

```yaml
coordinates:                      # the taxonomy is the project's own
  product: [kapi, bowrain]
  channel: [docs, landing, app, email]
  market:  [us, de]
  tenant:  []                     # an open axis: declared, values not enumerated

profiles:
  - when: {}                      # the base: matches every point
    voice: context/base-voice.yaml
  - when: { product: bowrain }
    voice: context/bowrain-voice.yaml
    terms: context/bowrain-terms.json   # optional; falls back to defaults.terms_source
  - when: { product: bowrain, market: de }
    voice: context/bowrain-de.yaml
    terms: context/de-terms.json

content:
  - name: docs
    context: { product: kapi, channel: docs }
  - name: landing
    context: { product: bowrain, channel: landing }
```

Axis names and values are **slugs** (`^[a-z0-9][a-z0-9-]*$`): stable machine
identifiers, never translated. A value may carry a concept for display —

```yaml
coordinates:
  product:
    - id: bowrain
      concept: term:9a1c0f42b7
    - id: kapi
```

— but the concept takes no part in matching or identity, and a coordinate value
must never *be* a concept reference. Concepts are designed to be renamed and
deprecated as vocabulary is revised; governance that moved when someone edited a
term would be governance nobody could rely on.

**Selection is most-specific-match-wins.** A profile matches when every key in
its `when:` equals the point's coordinate of the same name; among the matches,
the one with the most keys governs. `when: {}` matches everything and always
loses to a non-empty match. The winner *selects*, it does not layer: what it
leaves unbound comes from `defaults.brand_voice` / `defaults.terms_source`, not from the
broader profile it beat. Two profiles matching on the same number of coordinates
is a **load error** naming both — which voice a piece of content is written in
is not a map-order question.

`channel` is the one well-known axis. After a profile is selected, the point's
channel selects the override *inside* that profile's voice
(`profile.VoiceProfile.Channels`, [AD-022](/contribute/architecture/022-brand-voice)),
so a landing-page register is authored once beside the voice it varies rather
than duplicated into a voice file per product-and-channel pair. A channel the
profile declares no override for is not an error — the base voice applies. The
axis may also appear in a `when:`; both apply, matching choosing the voice and
the override refining the register within it.

`KapiProject.ResolveGovernance(collection)` resolves a point into a
`ResolvedGovernance` (channel, voice binding, terms, and the recipe key the
voice came from), falling back to the project defaults for an empty or unknown
collection name, and for a collection that declares no point;
`CollectionForPath(relPath)` names the collection that claims a file, by the
same first-match glob rule as target resolution. The name keeps its distance
from `profile.ResolveContext`, which is a different thing in a package used
alongside this one — the input to profile resolution, not the recipe's answer.

That is the recipe half, and it is an **authoring** half: the voice it names is
loaded by the host and then handed to `profile.ResolveProfileFromContext` as
`CollectionProfile` — the collection tier of the framework's single precedence
chain ([AD-022](/contribute/architecture/022-brand-voice)) — so an explicit
per-call profile still outranks the recipe and a project governed from the
server ranks its bindings identically. The point's channel goes in beside it as
`CollectionConfig[PropertyChannel]`, and `ResolveProfile` applies the override.

One venue applies the recipe at a time. A project that declares coordinates
(`KapiProject.GovernsByCoordinates`) and also carries a `server:` block is
warned at run time (`host.WarnUnsyncedCoordinates`, called by `kapi run`,
`kapi up` and `RunFlowAllLocales`) that coordinate governance applies to local
runs only until it is synced: the server has no coordinate rows yet and governs
by `defaults.brand_voice`. The run proceeds — this is a caveat, not a fault.

A run resolves its governance per collection and executes once per distinct
resolution: `groupInputsByBinding` (host) partitions the input set, and each
group gets its own bindings and its own tool chain — the chain is built before
any content is seen, so a per-file switch is not possible. Grouping keys on what
the coordinates *resolve to*, not on the coordinates themselves, so two markets
governed by one profile share a group, and a recipe where no collection declares
a point produces exactly one group: the single, unsplit run.

Every failure is caught at load, because a silent fall-back would translate that
content in a plausible-looking wrong voice: an axis absent from `coordinates:`,
a value the axis does not declare (unless the axis is open), a non-slug value, a
profile binding nothing, two profiles claiming one point, and the ambiguous
match above. Bare entries cannot carry a context at all: resolution is by
collection name, so a point on an unnamed entry could never be read.

`KapiProject.IterateContent` walks both shapes uniformly, yielding each
`ContentItem` paired with its parent collection so callers can resolve
fall-through fields. Language resolution falls through item → collection →
project defaults via `ContentItem.ResolvedSourceLanguage` /
`ResolvedTargetLanguages`. A bare entry's promoted fields are wrapped as a
single-item slice by `ContentCollection.EffectiveItems`, carrying its `Extras`
through so platform per-item fields survive.

## Defaults-scoped settings

`Defaults` holds project-wide processing settings that individual content items
can override. Beyond locales and the parallelism/encoding knobs shown above:

- `merge` (`MergeDefaults.ConflictPolicy`) — how `kapi merge` resolves a
  translator's target against an existing on-disk target or content-memory entry
  (`translator-wins` default, `existing-wins`, `newest-wins`). See
  [AD-017](/contribute/architecture/017-bilingual-format-interop).
- `memory` (`MemoryDefaults`) — the project's content memory:
  `fuzzy_threshold`, the pre-fill cutoff on `kapi extract` (default
  `DefaultFuzzyThreshold` = 75).
- `segmentation` (`SegmentationDefaults`) — opt-in SRX sentence segmentation
  overlay on extract (`source`, optional `srx` rules file).
- `redaction` (`*RedactionSpec`) — replace sensitive content with protected
  placeholders before processing and restore it afterwards. Overridable per
  `ContentItem.Redaction`.
- `brand_voice` (`*BrandVoiceBinding`) — bind a brand voice profile (one of
  `profile_file`, `profile`, or `pack`) as standing project context. This is the
  framework binding under `defaults:`, distinct from a platform's top-level
  `brand_voice` extension.
- `terms_source` / `memory_source` (string) — committed, git-tracked native
  source bundles (`.terms.json` / `.memory.json`) the project's terms store and
  content memory are indexed from. `kapi apply` edits the source and reindexes
  it, so the source is written by exactly one path and `git diff` is the review
  surface. `terms_source` left unset falls back to `<root>/terms.json`, then
  `<root>/.kapi/terms.json`; `memory_source` has no such fallback, because a
  project has one terms source but many memory bundles (one per content
  surface), leaving nothing single for a convention to name.

## The project store

The recipe binds sources; the sources are the truth. A project keeps one local
database, `.kapi/store.db` — a derived index over the committed sources
(`terms_source`, `memory_source`), the unit-decision record under `.kapi/units/`,
and the content files themselves — plus the working set of decisions staged since
the last `kapi commit`. Every subsystem's tables live in that one file: block
cache, terms store, content memory, working set, and the property graph. See
[AD-039](/contribute/architecture/039-local-context-graph-store) for the store's
shape, its rebuild guarantees, and how it converges with the server's graph.

## Platform extensions and the `server:` block

The framework knows nothing about platform-specific keys. Unknown top-level YAML
keys land in `Extras map[string]yaml.Node` (with `yaml:",inline"`) on
`KapiProject`, `Defaults`, `ContentCollection`, and `ContentItem`. Platform
layers decode their own typed schema from these maps via `GetExtra` and
re-encode on `SetExtra`; round-tripping a recipe through the framework alone
preserves the keys verbatim.

A vendor may use this to add their own recipe keys — for example, a `server:`
block (and `hooks`, `automations`, `assets`, `brand_voice` policy). A recipe
with no such extension is a pure local project. The kapi CLI tolerates unknown
blocks but ignores them; the owning plugin decodes them from `Extras`.
`requires:` (a map of plugin name → semver constraint) gates loading: a recipe
declaring `requires: { myplugin: "^1.0" }` refuses to load in a binary that has
not registered the `myplugin` extension. See
[AD-008](/contribute/architecture/008-project-model) for the full extension
model and `server:` schema.

## Validation Rules

- `version` is required, must be `"v1"`
- For each `content[]` entry:
  - Bare entry — `path` is required and `items` must be empty.
  - Named collection — `path` must be empty (use `items`) and `items` must be
    non-empty; each item requires a non-empty `path`.
  - `context` requires a `name`; every axis it names must be declared under
    `coordinates:`, and every value must be one the axis declares (any slug, on
    an open axis).
- `coordinates:` axis names and values are slugs; a value is declared once and
  may carry a whitespace-free `concept` reference.
- Every `profiles[]` entry must bind a `voice`, `terms`, or both; its `when:` is
  checked like a `context:`; its `voice` is shape-checked exactly like
  `defaults.brand_voice` (one of `profile_file`, `profile`, `pack`, or a bare
  path); no two entries may claim the same point, and no two may match one
  collection with equal specificity.
- `defaults.merge.conflict_policy`, `defaults.memory.fuzzy_threshold` (0..100),
  `defaults.redaction.detectors`, and `defaults.brand_voice` are each
  shape-checked.
- Each flow must have at least one step
- Each step must have a non-empty `tool` field (unless it uses `parallel`)
- Steps with `parallel` can omit `tool` (the parallel branches provide tools)
- Each `requires:` entry must have a non-empty plugin name and a well-formed
  semver constraint (`^1.0`, `>=1.4.0`, `1.4.0`, `~1.4.2`, or `*`). Unless
  `SkipRequiresCheck` is set, every named plugin must have a registered
  extension group, else loading fails with an install hint.
- Extras at each scope are validated against any registered extension schema.

Note: `name` is optional (`yaml:"name,omitempty"`); the framework does not
require it.

## File Paths

- Content patterns are expanded via `core/project.ExpandGlob`, backed by
  `github.com/bmatcuk/doublestar/v4` — recursive `**` directory matching is
  supported (e.g. `src/**/*.json`). `ExpandGlob` filters out any match that
  matches one of the `defaults.exclude` glob patterns (matched with
  `doublestar.Match`)
- Patterns are resolved relative to the project root (the recipe's parent
  directory)
- `target` is expanded per source file and target language by
  `core/project.ResolveTargetPath(itemPath, base, target, source, lang)`:
  - `base` is the directory the source path is made relative to. When empty it
    defaults to `GlobFixedPrefix(path)` — the literal prefix of the glob before
    the first `*`/`?`/`[`/`{` (so `input/docs/*.md` mirrors just filenames while
    `input/**/*.md`, or an explicit `base`, mirrors the subtree). On a named
    collection, an item inherits the collection's `base` when it sets none.
  - Tokens: `{lang}`, `{relpath}` (rel path with extension), `{path}` (rel path
    without extension), `{dir}`, `{filename}`, `{name}` (alias `{basename}`),
    `{ext}`; a bare `*` is shorthand for `{name}`. `{lang}` is handled by
    `ResolvePathPattern`; the rest by `ExpandTemplate`.
  - **Directory-mirror form:** when the target (after `{lang}` expansion) ends
    with `/`, is empty, or its final segment has no extension and no
    wildcard/token, it denotes a directory — the source's `{relpath}` (under
    `base`) is appended. So `target: output/{lang}` mirrors the source tree under
    each per-language root with no token and no doubled extension. See
    `isDirectoryTarget` in `core/project/path.go`.

## Credential Resolution

The `kapi.yaml` recipe references AI providers by type (e.g., `provider: anthropic`), not by key. API keys are resolved at runtime:

1. OS keychain via `host/credentials.Store` (non-secret config at
   `providers.json` under the user config directory — `~/.config/kapi` on Linux,
   `~/Library/Application Support/kapi` on macOS; `kapi config path` prints the
   resolved location. Keys live under the keychain service `"kapi"`)
2. Environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) or the
   `--api-key` flag
3. The `--provider` and `--model` CLI flags override project defaults

## CLI Integration

```bash
# One-shot (no project)
kapi translate -i file.json --target-lang fr

# With project file: run a built-in flow with project defaults
kapi run translate-qa -p kapi.yaml --target-lang de

# Or run a flow defined in the recipe's flows: map (here named "translate")
kapi run translate -p kapi.yaml
```

Built-in flows are `translate`, `translate-qa`, `pseudo-translate`,
`qa`, `recycle`, and `secure-translate` (see
`host/flowdef.BuiltInFlows`). A recipe's `flows:` map can add new flows and
override the single-tool built-ins (`translate`, `pseudo-translate`,
`qa`, `recycle`). It cannot override the composed built-ins
(`translate-qa`, `secure-translate`) when invoked via `-p`: `runWithProject`
(`cli/run.go`) dispatches those to the built-in pipeline before consulting
`proj.GetFlow`.

With `-p`:

- The flow name is matched against the built-in composed flows first (currently
  `translate-qa` and `secure-translate` — the `BuiltInFlows` entries with 2+
  tool nodes); if it is not one of those, it is looked up in the project's
  `flows` map (and finally the plugin fallback)
- `defaults.source_language` and `defaults.target_languages[0]` provide
  defaults (CLI flags override)
- For single-file flows, `--input` selects the file. The project's `content`
  collections describe which files `kapi extract` / `kapi merge` operate on
  across the project

## Desktop Integration

Kapi Desktop at `apps/kapi-desktop/`:

- Opens a project by its folder (which contains `kapi.yaml`) — File > Open, drag-and-drop
- Edits flows inline (steps editor)
- Resolves content patterns against the filesystem via `App.MatchContent(tabID)`,
  using the same `core/project` glob expansion the CLI relies on for `extract` /
  `merge` — pattern resolution is shared framework code, not a desktop-only feature
- Stores recent files (`recent.json`) and settings (`settings.json`) in its own
  config root — `~/.config/kapi-desktop` on Linux,
  `~/Library/Application Support/kapi-desktop` on macOS — overridable with
  `KAPI_DESKTOP_CONFIG_DIR` (`apps/kapi-desktop/backend/paths.go`)

## Example Files

### Minimal

```yaml
version: v1
name: Quick Translate
```

### Full

```yaml
version: v1
name: Acme App

defaults:
  source_language: en
  target_languages: [fr, de, ja]
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
  exclude:
    - "**/*.generated.json"
  merge:
    conflict_policy: translator-wins
  memory:
    fuzzy_threshold: 75
  segmentation:
    source: true
  terms_source: context/terms.json
  memory_source: context/memory.json

content:
  # Bare entry — single glob, languages inherited from defaults.
  # Directory-mirror target: src/i18n/en/app.json → src/i18n/{lang}/app.json.
  - path: "src/i18n/en/*.json"
    target: "src/i18n/{lang}"

  # Named collection — groups patterns, scopes languages, and shares a base.
  - name: Marketing
    target_languages: [fr, de]
    base: en
    items:
      - path: "en/docs/**/*.md"
        target: "{lang}/docs"
      - path: "en/site/**/*.html"
        target: "{lang}/site"

preset: nextjs
requires:
  okapi-bridge: ">=1.47.0"

flows:
  translate:
    steps:
      - tool: translate
        config:
          provider: anthropic
          model: claude-sonnet-4-20250514

  full-pipeline:
    steps:
      - tool: recycle
        config:
          fuzzyThreshold: 75
      - tool: translate
        config:
          provider: anthropic
      - tool: qa

  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansionPercent: 30
```
