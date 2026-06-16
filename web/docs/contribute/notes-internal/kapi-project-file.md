---
sidebar_position: 7
title: .kapi Project File Format
description: Implementation note for AD-008 — the KapiProject YAML schema, ContentCollection/ContentItem and Defaults struct layouts, how extension extras are decoded, and how the .kapi recipe is loaded, validated, and saved.
keywords: [kapi project file, KapiProject, YAML schema, ContentCollection, ContentItem, Defaults, project model, implementation note]
---

# .kapi Project File Format

Implementation notes for the `.kapi` project file format. See [AD-008](/contribute/architecture/008-project-model) for the architectural decision.

## Schema

The `.kapi` file is a YAML document parsed by `core/project.KapiProject`:

```go
type KapiProject struct {
    Version  string                     `yaml:"version"`
    Name     string                     `yaml:"name,omitempty"`
    Plugins  map[string]PluginSpec      `yaml:"plugins,omitempty"`  // name → spec (scalar = version short form)
    Defaults Defaults                   `yaml:"defaults,omitempty"` // project-wide defaults (locales live here)
    Content  []ContentCollection        `yaml:"content,omitempty"`
    Preset   string                     `yaml:"preset,omitempty"`
    Flows    map[string]*flow.StepsSpec `yaml:"flows,omitempty"`
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
    // (also: locale_format, formats, exclude, merge, tm, segmentation,
    //  redaction, brand_voice, termbase — see core/project/project.go)
}

// ContentCollection is either a bare entry (path/format/target) or a named
// collection (name + items), and can carry its own source/target languages.
type ContentCollection struct {
    Name            string           `yaml:"name,omitempty"`
    SourceLanguage  model.LocaleID   `yaml:"source_language,omitempty"`
    TargetLanguages []model.LocaleID `yaml:"target_languages,omitempty"`
    Items           []ContentItem    `yaml:"items,omitempty"`
    Base            string           `yaml:"base,omitempty"`   // dir items' paths are made relative to; items inherit it
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
  translator's target against an existing on-disk target or TM entry
  (`translator-wins` default, `existing-wins`, `newest-wins`). See
  [AD-017](/contribute/architecture/017-bilingual-format-interop).
- `tm` (`TMDefaults`) — `fuzzy_threshold` (TM pre-fill cutoff on `kapi extract`,
  default 75) and `read` (additional read-only TM files; writes always go to the
  project TM).
- `segmentation` (`SegmentationDefaults`) — opt-in SRX sentence segmentation
  overlay on extract (`source`, optional `srx` rules file).
- `redaction` (`*RedactionSpec`) — replace sensitive content with protected
  placeholders before processing and restore it afterwards. Overridable per
  `ContentItem.Redaction`.
- `brand_voice` (`*BrandVoiceBinding`) — bind a brand voice profile (one of
  `profile_file`, `profile`, or `pack`) as standing project context. This is the
  framework binding under `defaults:`, distinct from a platform's top-level
  `brand_voice` extension.
- `termbase` (string) — path to a glossary/termbase, resolved relative to the
  project root, used for project-scoped term enforcement with no `--termbase`
  flag.

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
- `defaults.merge.conflict_policy`, `defaults.tm.fuzzy_threshold` (0..100),
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
    `{ext}`; a bare `*` is legacy shorthand for `{name}`. `{lang}` is handled by
    `ResolvePathPattern`; the rest by `ExpandTemplate`.
  - **Directory-mirror form:** when the target (after `{lang}` expansion) ends
    with `/`, is empty, or its final segment has no extension and no
    wildcard/token, it denotes a directory — the source's `{relpath}` (under
    `base`) is appended. So `target: output/{lang}` mirrors the source tree under
    each per-language root with no token and no doubled extension. See
    `isDirectoryTarget` in `core/project/path.go`.

## Credential Resolution

The `.kapi` file references AI providers by type (e.g., `provider: anthropic`), not by key. API keys are resolved at runtime:

1. OS keychain via `cli/credentials.Store` (non-secret config at
   `~/.config/kapi/providers.json`; keys under the keychain service `"kapi"`)
2. Environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) or the
   `--api-key` flag
3. The `--provider` and `--model` CLI flags override project defaults

## CLI Integration

```bash
# One-shot (no project)
kapi ai-translate -i file.json --target-lang fr

# With project file: run a built-in flow with project defaults
kapi run ai-translate-qa -p translation.kapi --target-lang de

# Or run a flow defined in the recipe's flows: map (here named "translate")
kapi run translate -p translation.kapi
```

Built-in flows are `ai-translate`, `ai-translate-qa`, `pseudo-translate`,
`qa-check`, `tm-leverage`, and `secure-translate` (see
`core/flow.BuiltInFlows`). A recipe's `flows:` map can add new flows and
override the single-tool built-ins (`ai-translate`, `pseudo-translate`,
`qa-check`, `tm-leverage`). It cannot override the composed built-ins
(`ai-translate-qa`, `secure-translate`) when invoked via `-p`: `runWithProject`
(`cli/run.go`) dispatches those to the built-in pipeline before consulting
`proj.GetFlow`.

With `-p`:

- The flow name is matched against the built-in composed flows first (currently
  `ai-translate-qa` and `secure-translate` — the `BuiltInFlows` entries with 2+
  tool nodes); if it is not one of those, it is looked up in the project's
  `flows` map (and finally the plugin fallback)
- `defaults.source_language` and `defaults.target_languages[0]` provide
  defaults (CLI flags override)
- For single-file flows, `--input` selects the file. The project's `content`
  collections describe which files `kapi extract` / `kapi merge` operate on
  across the project

## Desktop Integration

Kapi Desktop at `apps/kapi-desktop/`:

- Opens `.kapi` files as documents (File > Open, drag-and-drop, OS file association)
- Edits flows inline (steps editor)
- Resolves content patterns against the filesystem via `App.MatchContent(tabID)`,
  using the same `core/project` glob expansion the CLI relies on for `extract` /
  `merge` — pattern resolution is shared framework code, not a desktop-only feature
- Stores recent files at `~/.config/kapi-desktop/recent.json`
- Stores settings at `~/.config/kapi-desktop/settings.json`

## Example Files

### Minimal

```yaml
version: v1
name: Quick Translate
```

### Full

```yaml
version: v1
name: Acme App Localization

defaults:
  source_language: en-US
  target_languages: [fr-FR, de-DE, ja-JP]
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
  exclude:
    - "**/*.generated.json"
  merge:
    conflict_policy: translator-wins
  tm:
    fuzzy_threshold: 75
  segmentation:
    source: true
  termbase: glossary/terms.db

content:
  # Bare entry — single glob, languages inherited from defaults.
  # Directory-mirror target: src/i18n/en/app.json → src/i18n/{lang}/app.json.
  - path: "src/i18n/en/*.json"
    target: "src/i18n/{lang}"

  # Named collection — groups patterns, scopes languages, and shares a base.
  - name: Marketing
    target_languages: [fr-FR, de-DE]
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
      - tool: ai-translate
        config:
          provider: anthropic
          model: claude-sonnet-4-20250514

  full-pipeline:
    steps:
      - tool: tm-leverage
        config:
          fuzzyThreshold: 75
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check

  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansionPercent: 30
```
