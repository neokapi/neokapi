---
sidebar_position: 1
title: kapi.yaml Project File Format
description: Implementation note for C-01 — the KapiProject YAML schema, Collection/ContentItem and Defaults struct layouts, how extension extras are decoded, and how the kapi.yaml recipe is loaded, validated, and saved.
keywords: [kapi project file, kapi.yaml, KapiProject, YAML schema, Collection, ContentItem, Defaults, project model, implementation note]
---

# kapi.yaml Project File Format

Implementation notes for the `kapi.yaml` project file format. See [C-01](/contribute/architecture/context/c-01-project-model) for the architectural decision.

## Schema

The `kapi.yaml` recipe is a YAML document parsed by `core/project.KapiProject`:

```go
type KapiProject struct {
    Version     string                     `yaml:"version"`
    Name        string                     `yaml:"name,omitempty"`
    Plugins     map[string]PluginSpec      `yaml:"plugins,omitempty"`  // name → spec (scalar = version short form)
    Defaults    Defaults                   `yaml:"defaults,omitempty"` // project-wide defaults (locales live here)
    Collections []Collection               `yaml:"collections,omitempty"`
    Preset      string                     `yaml:"preset,omitempty"`
    Flows       map[string]*flow.StepsSpec `yaml:"flows,omitempty"`
    Profiles    map[string]Profile         `yaml:"profiles,omitempty"` // profile name → governance (see The context space)
    Requires    RequiresMap                `yaml:"requires,omitempty"` // plugin name → semver constraint
    Extras      map[string]yaml.Node       `yaml:",inline"`            // unknown keys (platform extensions)
}

// Defaults holds project-wide processing defaults — including locales.
type Defaults struct {
    SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty"`
    TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
    Concurrency     int              `yaml:"concurrency,omitempty"`
    ParallelBlocks  int              `yaml:"parallel_blocks,omitempty"`
    Encoding        string           `yaml:"encoding,omitempty"`
    // (also: locale_format, formats, exclude, merge, memory, segmentation,
    //  redaction, voice, terms_source, memory_source — see
    //  core/project/project.go.)
}

// Collection is either a bare entry (path/format/target) or a named collection
// (name + content), and can carry its own source/target languages.
type Collection struct {
    Name            string           `yaml:"name,omitempty"`
    SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty"`
    TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
    Content         []ContentItem    `yaml:"content,omitempty"`
    Base            string           `yaml:"base,omitempty"`    // the directory this collection lives in
    Channel         string           `yaml:"channel,omitempty"` // `profile/channel` (named collections only)
    // Bare-entry fields (short form):
    Path   string      `yaml:"path,omitempty"`   // doublestar glob for source files
    Format *FormatSpec `yaml:"format,omitempty"` // format ID; auto-detect per file if empty
    Target string      `yaml:"target,omitempty"` // output path template (tokens below)
}
// ContentItem additionally carries its own `base` (yaml:"base,omitempty"), the
// directory its matched paths are made relative to for target-token expansion.
```

Flow definitions reuse `core/flow.StepsSpec` and `core/flow.FlowStep` (see [flow-steps-format](../engine/flow-steps-format.md)).

## Content model

`Collections` is a list of `Collection` values. Each entry is one of two
shapes, distinguished by `Collection.IsBareEntry()`:

- **Bare entry** — has a `path` and no `content`. The `path`, `format`, and
  `target` fields are promoted onto the collection directly. Use this for a
  single glob with no grouping.
- **Named collection** — has a `name` and a non-empty `content` list of
  `ContentItem`, and may set its own `source_language` / `target_languages`.
  Use this to group related patterns and scope languages per group.

### Where a collection lives

`Collection.Base` is the directory the collection lives in. `EffectiveItems`
folds it in: every item's `Path`, `Target` and own `Base` is joined onto it
(`JoinBase`), so every consumer downstream sees project-relative paths and never
has to know a base was declared. An absolute path is left alone, so the escape
check downstream still sees it; an empty one stays empty.

An item that declares no `Base` of its own keeps none — the target tokens then
relativize against the joined pattern's own fixed prefix. That is what makes
`base:` a *location* rather than a second relativization root.

### The context space

Content is written for a point in a two-axis space: the PRODUCT it belongs to
and the CHANNEL it ships on. The space is **structural** rather than declared —
a key under `profiles:` is a product, the channels that profile lists are the
channels that product ships on, and a named collection names the point its
content sits at with one `channel:` reference:

```yaml
profiles:
  neokapi:
    channels: [cli, docs]
    voice: .kapi/voice.yaml                    # the framework's own voice
  bowrain:
    channels:
      - id: docs
        concept: term:9a1c0f42b7               # display only; never resolved
      - app
    voice: .kapi/profiles/bowrain/voice.yaml   # == the conventional location
    terms: .kapi/profiles/bowrain/terms.json   # optional; falls back to the project store

collections:
  - name: bowrain-app
    channel: bowrain/app
  - name: neokapi-docs
    channel: neokapi/docs                      # both declare `docs` — qualify it
```

The map key under `profiles:` is the profile's name: the product-axis value its
collections carry, and the directory under `.kapi/profiles/<name>/` holding the
files it overrides. A profile that binds neither a voice nor a vocabulary is
still a profile — that directory is the binding, and a project keeping its
overrides there should not have to restate every one of them in the recipe.

Profile names and channels are **slugs** (`^[a-z0-9][a-z0-9-]*$`): stable
machine identifiers, never translated, comparable byte for byte. A profile and a
channel may each *carry* a concept for display, but resolution never looks at
it: concepts are designed to be renamed and deprecated as vocabulary is revised,
and governance that moved when someone edited a term would be governance nobody
could rely on.

**Resolution is by declaration, not by matching.** `KapiProject.ResolveChannel`
reads one `channel:` reference, always the qualified `profile/channel`. A bare
channel name is an error that spells out the qualified form(s). The result
is a `ChannelRef{Profile, Channel}`, whose zero value is the project's default
point.

After a profile is selected, the collection's channel selects the override
*inside* that profile's voice (`profile.VoiceProfile.Channels`,
[C-07](/contribute/architecture/context/c-07-voice-profiles)), so a landing-page register
is authored once beside the voice it varies rather than duplicated into a voice
file per product-and-channel pair. A channel the profile declares no override
for is not an error — the base voice applies, which is the right answer for a
voice that reads the same everywhere.

`KapiProject.ResolveGovernance(collection)` resolves a collection name into a
`ResolvedGovernance` (channel, voice binding, terms, the profile's name, and the
recipe key the voice came from), falling back to the project defaults for an
empty or unknown collection name, and for a collection that binds no channel;
`CollectionForPath(relPath)` names the collection that claims a file, by the
same first-match glob rule as target resolution. The name keeps its distance
from `profile.ResolveContext`, which is a different thing in a package used
alongside this one — the input to profile resolution, not the recipe's answer.

That is the recipe half, and it is an **authoring** half: the voice it names is
loaded by the host and then handed to `profile.ResolveProfileFromContext` as
`CollectionProfile` — the collection tier of the framework's single precedence
chain ([C-07](/contribute/architecture/context/c-07-voice-profiles)) — so an explicit
per-call profile still outranks the recipe and a project governed from the
server ranks its bindings identically. The point's channel goes in beside it as
`CollectionConfig[PropertyChannel]`, and `ResolveProfile` applies the override.

`ChannelRef.Coordinates()` renders the point as the two axes that travel on the
sync wire — `project.ProductAxis` (`"product"`) carrying the profile name and
`project.ChannelAxis` (`"channel"`) carrying the channel — and the default point
renders as nil. A push carries every declared collection, its point and the
voice governing it, so both venues resolve the same voice for the same content.

What does not cross is a profile's `terms:`. That is a path into the local
project, and a path means nothing to a server that governs terminology from the
workspace vocabulary. A recipe that binds terms per profile
(`KapiProject.BindsTermsByProfile`) and also binds a venue is warned at run time
(`host.WarnUnsyncedCoordinates`, called by `kapi run`, `kapi up` and
`RunFlowAllLocales`) that the binding applies to local runs only. The run
proceeds — this is a caveat, not a fault.

One function resolves governance for every surface:
`KapiProject.ResolveGovernanceFor(GovernancePoint{Collection, Path, At})`. It
walks the declared bindings finest-first — a content item's own `channel:`, then
its collection's, then the project default — skipping any whose profile is
outside its validity window at `At`, and records the skip on
`ResolvedGovernance.Fallback` so the caller can report it. `ResolveGovernance`
and `ResolveGovernanceForPath` are the as-declared views over the same walk (a
zero `At` applies no window); `ResolveGovernanceAt` is the as-of view.

A run resolves per FILE and executes once per distinct resolution:
`groupInputsByBinding` (host) partitions the input set through that function,
and each group gets its own bindings and its own tool chain — the chain is built
before any content is seen, so the partition is what makes per-file governance
possible. Grouping keys on what the points *resolve to*, not on the points
themselves, so two collections governed by one profile share a group, and a
recipe where nothing binds a channel produces exactly one group: the single,
unsplit run.

The instant is fixed once per run (`App.GovernanceInstant`, shared by every
converge worker), so a long pass cannot cross a validity boundary halfway
through. The first fall-through per run is printed on stderr as
`governance: profile "x" expired <date>; governing with …`, deduplicated by
`App.NoteGovernance`; `kapi context search` carries the same sentence in its
result notes.

Every failure is caught at load, because a silent fall-back would translate that
content in a plausible-looking wrong voice: a non-slug profile name or channel, a
channel declared twice by one profile, a profile voice binding that names no
source or more than one, a malformed concept reference, a channel reference
naming an undeclared profile or channel, and any bare (unqualified) reference.
Bare entries cannot carry a channel at all: resolution is by collection name, so
a point on an unnamed entry could never be read.

`KapiProject.IterateContent` walks both shapes uniformly, yielding each
`ContentItem` — base already folded in — paired with its parent collection so
callers can resolve fall-through fields. Language resolution falls through item →
collection → project defaults via `ContentItem.ResolvedSourceLanguage` /
`ResolvedTargetLanguages`. A bare entry's promoted fields are wrapped as a
single-item slice by `Collection.EffectiveItems`, carrying its `Extras`
through so platform per-item fields survive.

## Defaults-scoped settings

`Defaults` holds project-wide processing settings that individual content items
can override. Beyond locales and the parallelism/encoding knobs shown above:

- `merge` (`MergeDefaults.ConflictPolicy`) — how `kapi merge` resolves a
  translator's target against an existing on-disk target or content-memory entry
  (`translator-wins` default, `existing-wins`, `newest-wins`). See
  [M-01](/contribute/architecture/multilingual/m-01-bilingual-interop).
- `memory` (`MemoryDefaults`) — the project's content memory:
  `fuzzy_threshold`, the pre-fill cutoff on `kapi extract` (default
  `DefaultFuzzyThreshold` = 75).
- `segmentation` (`SegmentationDefaults`) — opt-in SRX sentence segmentation
  overlay on extract (`source`, optional `srx` rules file).
- `redaction` (`*RedactionSpec`) — replace sensitive content with protected
  placeholders before processing and restore it afterwards. Overridable per
  `ContentItem.Redaction`.
- `voice` (`*VoiceBinding`) — bind a voice profile (one of
  `profile_file`, `profile`, or `pack`) as standing project context. This is the
  framework binding under `defaults:`, distinct from a platform's top-level
  `brand_voice` extension.
- `terms_source` / `memory_source` (string) — committed, git-tracked native
  source bundles (`.terms.json` / `.memory.json`) the project's terms store and
  content memory are indexed from. `kapi apply` edits the source and reindexes
  it, so the source is written by exactly one path and `git diff` is the review
  surface. Both keys bind any path; the conventional homes are inside the
  committed context graph. `terms_source` left unset falls back to
  `<root>/.kapi/terms.json`, then `<root>/terms.json`; `memory_source`
  has no such fallback, because a project has one terms source but many memory
  bundles (one per content surface), leaving nothing single for a convention to
  name.

## The project store

The recipe binds sources; the sources are the truth. A project keeps one local
database, `.kapi/work/store.db` — a derived index over the committed sources
(`terms_source`, `memory_source`), the unit-state record under `.kapi/state/`,
and the content files themselves — plus the working set of unit state staged since
the last `kapi commit`. Every subsystem's tables live in that one file: block
cache, terms store, content memory, working set, and the property graph. See
[C-03](/contribute/architecture/context/c-03-context-store-and-graph) for the store's
shape, its rebuild guarantees, and how it converges with the server's graph.

## Platform extensions and the venue

The framework knows nothing about platform-specific keys. Unknown top-level YAML
keys land in `Extras map[string]yaml.Node` (with `yaml:",inline"`) on
`KapiProject`, `Defaults`, `Collection`, and `ContentItem`. Platform
layers decode their own typed schema from these maps via `GetExtra` and
re-encode on `SetExtra`; round-tripping a recipe through the framework alone
preserves the keys verbatim.

A vendor may use this to add their own recipe keys — bowrain's are `bowrain:`
(and `hooks`, `automations`, `assets`, `brand_voice` policy). A recipe with no
such extension is a pure local project. The kapi CLI tolerates unknown blocks
but ignores them; the owning plugin decodes them from `Extras`. `requires:` (a
map of plugin name → semver constraint) gates loading: a recipe declaring
`requires: { myplugin: "^1.0" }` refuses to load in a binary that has not
registered the `myplugin` extension.

One of those keys may be the project's **convergence venue** — a server that
holds the content memory, runs the loop on org keys, and carries a review queue.
The framework's interest in it is exactly two fields, `url:` and `converge:`, so
it asks the registry which extension *is* the venue rather than looking for a key
by name: an `Extension` registered with `Venue: true` claims that role, and a
plugin manifest declares it as `"venue": true` on a schema extension.
`KapiProject.Venue()` returns the `VenueBinding{Key, URL, Converge}` of the
first registered venue extension the recipe carries, `VenueKey()` names the
registered venue key with no recipe in hand (so a message about an absent block
can still name the block to add), and `IsVenueKey(name)` tests one key. An
unregistered key of the same name — a recipe loaded by a binary without the
platform — reports no venue and no opinion. See
[C-01](/contribute/architecture/context/c-01-project-model) for the full extension
model.

## Validation Rules

- `version` is required, must be `"v1"`. A top-level key the recipe does not
  have (`content:`, `coordinates:`) is rejected by name rather than silently
  captured as an unknown extension.
- For each `collections[]` entry:
  - Bare entry — `path` is required and `content` must be empty.
  - Named collection — `path` must be empty (use `content`) and `content` must be
    non-empty; each item requires a non-empty `path`.
  - `channel` requires a `name`, and must resolve against the declared profiles
    as qualified `profile/channel`.
- Every `profiles:` key is a slug, and so is every channel it declares; a channel
  is declared at most once per profile. A profile's `voice` is shape-checked
  exactly like `defaults.voice` (one of `profile_file`, `profile`, `pack`, or a
  bare path), and a `concept` on the profile or on a channel must be
  whitespace-free.
- `defaults.merge.conflict_policy`, `defaults.memory.fuzzy_threshold` (0..100),
  `defaults.redaction.detectors`, and `defaults.voice` are each
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
  - `base` here is the item's own base — the directory the source path is made
    relative to — after the collection's `base` has been folded in. When empty it
    defaults to `GlobFixedPrefix(path)` — the literal prefix of the glob before
    the first `*`/`?`/`[`/`{` (so `input/docs/*.md` mirrors just filenames while
    `input/**/*.md`, or an explicit `base`, mirrors the subtree).
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
- For single-file flows, `--input` selects the file. The project's
  `collections` describe which files `kapi extract` / `kapi merge` operate on
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
  terms_source: .kapi/terms.json
  memory_source: .kapi/memory/memory.json

profiles:
  acme:
    channels: [app, marketing]
    voice: .kapi/voice.yaml

collections:
  # Bare entry — single glob, languages inherited from defaults.
  # Directory-mirror target: src/i18n/en/app.json → src/i18n/{lang}/app.json.
  - path: "src/i18n/en/*.json"
    target: "src/i18n/{lang}"

  # Named collection — groups patterns, scopes languages, binds a channel, and
  # names the directory it lives in. Paths and targets below are relative to it:
  # marketing/en/docs/api.md → marketing/fr/docs/api.md.
  - name: Marketing
    channel: console/marketing
    target_languages: [fr, de]
    base: marketing
    content:
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
