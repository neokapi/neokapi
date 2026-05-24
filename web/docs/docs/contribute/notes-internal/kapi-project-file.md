---
sidebar_position: 7
title: .kapi Project File Format
description: Implementation note for AD-008 — the KapiProject YAML schema, ContentEntry and Defaults struct layouts, how extension extras are decoded, and how the .kapi recipe is loaded, validated, and saved.
keywords: [kapi project file, KapiProject, YAML schema, ContentEntry, Defaults, project model, implementation note]
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
    // Bare-entry fields (short form):
    Path   string      `yaml:"path,omitempty"`   // glob pattern for source files
    Format *FormatSpec `yaml:"format,omitempty"` // format ID; auto-detect if empty
    Target string      `yaml:"target,omitempty"` // output path with {lang} placeholder
}
```

Flow definitions reuse `core/flow.StepsSpec` and `core/flow.FlowStep` (see [flow-steps-format](./flow-steps-format.md)).

## Validation Rules

- `version` is required, must be `"v1"`
- `name` is required, non-empty
- Each `content[].path` must be non-empty
- Each flow must have at least one step
- Each step must have a non-empty `tool` field (unless it uses `parallel`)
- Steps with `parallel` can omit `tool` (the parallel branches provide tools)

## File Paths

- Content patterns are expanded via `core/project.ExpandGlob`, backed by
  `github.com/bmatcuk/doublestar/v4` — recursive `**` directory matching is
  supported (e.g. `src/**/*.json`), along with `exclude` glob patterns
- Patterns are resolved relative to the `.kapi` file's parent directory
- The `{lang}` placeholder in `target` is expanded with the target locale at runtime

## Credential Resolution

The `.kapi` file references AI providers by type (e.g., `provider: anthropic`), not by key. API keys are resolved at runtime:

1. Kapi: OS keychain via `cli/credentials.Store` (`~/.config/kapi/providers.json` + keyring service `"kapi"`)
2. Kapi CLI: environment variables (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) or `--api-key` flag
3. The `--provider` and `--model` CLI flags override project defaults

## CLI Integration

```bash
# One-shot (no project, unchanged behavior)
kapi ai-translate -i file.json --target-lang fr

# With project file
kapi run translate -p translation.kapi
kapi run translate-and-qa -p translation.kapi --target-lang de
```

With `-p`:

- Flow name is looked up in the project's `flows` map
- `source_language` and `target_languages[0]` provide defaults (CLI flags override)
- `--input` is still required (content pattern resolution is Kapi only)

## Desktop Integration

Kapi Desktop at `apps/kapi-desktop/`:

- Opens `.kapi` files as documents (File > Open, drag-and-drop, OS file association)
- Edits flows inline (steps editor)
- Resolves content patterns against the filesystem via `App.MatchContent(tabID)`
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

content:
  - path: "src/i18n/en/*.json"
    format: json
    target: "src/i18n/{lang}/*.json"
  - path: "docs/en/*.md"
    format: markdown

preset: nextjs
requires:
  okapi-bridge: ">=1.47.0"

flows:
  translate:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
          model: claude-sonnet-4-5-20241022

  full-pipeline:
    steps:
      - tool: tm-leverage
        config:
          fuzzy_threshold: 75
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check

  pseudo:
    steps:
      - tool: pseudo-translate
        config:
          expansion_rate: 1.3

defaults:
  concurrency: 4
  parallel_blocks: 3
  encoding: utf-8
```
