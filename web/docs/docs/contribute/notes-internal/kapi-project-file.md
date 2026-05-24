# .kapi Project File Format

Implementation notes for the `.kapi` project file format. See [AD-008](/contribute/architecture/008-project-model) for the architectural decision.

## Schema

The `.kapi` file is a YAML document parsed by `core/project.KapiProject`:

```go
type KapiProject struct {
    Version         string                     `yaml:"version"`
    Name            string                     `yaml:"name"`
    SourceLanguage  string                     `yaml:"source_language,omitempty"`
    TargetLanguages []string                   `yaml:"target_languages,omitempty"`
    Content         []ContentEntry             `yaml:"content,omitempty"`
    Preset          string                     `yaml:"preset,omitempty"`
    Plugins         []string                   `yaml:"plugins,omitempty"`
    Flows           map[string]*flow.StepsSpec  `yaml:"flows,omitempty"`
    Defaults        Defaults                   `yaml:"defaults,omitempty"`
}

type ContentEntry struct {
    Path   string `yaml:"path"`             // glob pattern for source files
    Format string `yaml:"format,omitempty"` // format ID; auto-detect if empty
    Target string `yaml:"target,omitempty"` // output path with {lang} placeholder
}

type Defaults struct {
    Concurrency    int    `yaml:"concurrency,omitempty"`
    ParallelBlocks int    `yaml:"parallel_blocks,omitempty"`
    Encoding       string `yaml:"encoding,omitempty"`
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

- `.kapi` files use `filepath.Glob` for content pattern matching
- Patterns are resolved relative to the `.kapi` file's parent directory
- The `{lang}` placeholder in `target` is expanded with the target locale at runtime
- No recursive globbing (`**`) — use `filepath.Glob` compatible patterns

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

Kapi at `framework/apps/desktop/`:

- Opens `.kapi` files as documents (File > Open, drag-and-drop, OS file association)
- Edits flows inline (steps editor)
- Resolves content patterns against the filesystem via `MatchContent(basePath)`
- Stores recent files at `~/.config/desktop/recent.json`
- Stores settings at `~/.config/desktop/settings.json`

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
source_language: en-US
target_languages: [fr-FR, de-DE, ja-JP]

content:
  - path: "src/i18n/en/*.json"
    format: json
    target: "src/i18n/{lang}/*.json"
  - path: "docs/en/*.md"
    format: markdown

preset: nextjs
plugins: [okapi@1.47.0]

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
