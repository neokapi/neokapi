---
id: 042-project-context
sidebar_position: 42
title: "AD-042: Project Context — Scoped Runtime Configuration"
---

# AD-042: Project Context — Scoped Runtime Configuration

## Context

A `.kapi` project file ([AD-041](./041-kapi-desktop.md)) is a declarative
recipe for localization: plugins, languages, format defaults, concurrency,
content patterns, and flows. When a project is opened or a flow is executed,
the framework needs to resolve this declaration into a configured runtime —
scoped format detection, resolved defaults, applied presets. This is the
**ProjectContext**: the bridge between the static project file and the live
processing environment.

Without a unified resolution layer, each consumer (CLI, desktop app, MCP)
would independently extract and apply project settings, leading to
inconsistency and duplication.

## Decision

### `ProjectContext` in `core/project/`

A `ProjectContext` is constructed from a `KapiProject` and the project's file
path. It resolves all declared settings into ready-to-use values.

```go
// core/project/context.go

type ProjectContext struct {
    Project        *KapiProject
    ProjectDir     string              // absolute path to the .kapi file's directory

    SourceLocale   model.LocaleID
    TargetLocales  []model.LocaleID
    AllowedSources []string            // ["built-in"] or ["built-in", "okapi-bridge", ...]
    Encoding       string              // default: "UTF-8"
    Concurrency    int                 // 0 = auto
    ParallelBlocks int                 // 0 = flow default
    LocaleFormat   string              // "bcp-47" (default) or "posix"
    FormatDefaults map[string]FormatDefaults
}

func NewProjectContext(proj *KapiProject, projectPath string) *ProjectContext
```

`AllowedSources` is derived from the project's `plugins` map: always includes
`"built-in"`, plus the name of each declared plugin. A project with no plugins
section sees only built-in formats.

### Project-Scoped Format Detection

```go
func (ctx *ProjectContext) DetectFormat(reg *registry.FormatRegistry, path string) string
```

Delegates to `FormatRegistry.DetectByExtensionForSources(ext, ctx.AllowedSources)`.
When the okapi-bridge plugin is installed globally but a project doesn't
declare it, plugin formats (priority 100) are excluded from auto-detection and
built-in formats (priority 50) are used instead.

The underlying registry method:

```go
// core/registry/format.go

func (r *FormatRegistry) DetectByExtensionForSources(
    ext string, allowedSources []string,
) (string, error)
```

Filters candidate formats by their `Source` field before applying the standard
priority ranking. `nil` or empty `allowedSources` disables filtering
(equivalent to `DetectByExtension`).

Explicitly declared formats in content items (`format: okf_json`) bypass
detection entirely and are always honored, regardless of plugin scope.

### Content Resolution

```go
func (ctx *ProjectContext) ResolveContent(
    reg *registry.FormatRegistry,
) ([]ResolvedFile, error)

type ResolvedFile struct {
    Path       string
    Relative   string          // relative to ProjectDir
    Format     string          // detected or explicit
    Collection string
    Pattern    string
    Item       *ContentItem
}
```

Matches content patterns against the filesystem, applies ignore rules, detects
formats using project-scoped detection, and returns the resolved file list.
This is the single implementation used by both CLI (replacing the current
"not yet implemented" error in `runFromProject`) and the desktop `MatchContent`.

### Format Configuration

```go
func (ctx *ProjectContext) ConfigureReader(
    reader format.DataFormatReader, formatName string,
) error

func (ctx *ProjectContext) ConfigureWriter(
    writer format.DataFormatWriter, formatName string,
) error
```

Applies `FormatDefaults` from the project: preset selection and config
overrides. If the project declares `defaults.formats.okf_html.preset: strict-extraction`,
`ConfigureReader` applies that preset to the HTML reader before opening. No
project defaults for a given format = no-op.

### Flow Execution

The existing `flow.ResourceContext` is extended with project-scoped settings:

```go
type ResourceContext struct {
    ProjectDir     string
    OutputDir      string
    SourceLocale   string
    TargetLocale   string
    ToolName       string

    // Project-scoped execution settings
    Concurrency    int
    ParallelBlocks int
    Encoding       string
    FormatDefaults map[string]FormatDefaults
}
```

The executor reads `Concurrency` and `ParallelBlocks` from the resource
context. When running outside a project (ad-hoc CLI mode), these default to
zero (auto).

### Plugin Scoping

`AllowedSources` controls format detection today. The same mechanism extends
to other project-scoped concerns:

- **Tool scoping**: `AllowedTools()` filters the tool registry to tools from
  declared plugins plus built-ins. The flow editor shows only available tools.
- **Preset scoping**: Framework presets from undeclared plugins are excluded
  from preset selectors.
- **Flow validation**: Flows referencing tools from undeclared plugins produce
  warnings during project validation.

### Consumer Integration

All consumers construct a `ProjectContext` when operating in project mode:

```go
// CLI
ctx := project.NewProjectContext(proj, projectPath)
files, _ := ctx.ResolveContent(a.FormatReg)

// Desktop
ctx := project.NewProjectContext(op.Project, op.Path)
matches := ctx.ResolveContent(a.formatReg)

// Ad-hoc mode (no project) — no ProjectContext, uses global registries directly
```

CLI flags and desktop UI settings override project defaults when explicitly
set. The project provides defaults, not mandates.

## Trade-offs

**Unified resolution vs. consumer flexibility.** ProjectContext standardizes
how project settings are applied, which reduces flexibility for consumers that
want to handle settings differently. The override model (explicit flags win)
mitigates this — consumers can always bypass project defaults.

**Framework dependency on project model.** Format detection, content resolution,
and reader/writer configuration now flow through `ProjectContext`. This
couples the processing pipeline to the project model. The coupling is
acceptable because project-mode execution is inherently project-aware, and
ad-hoc mode bypasses `ProjectContext` entirely.

**Source filtering vs. priority adjustment.** An alternative to filtering
formats by source would be to dynamically adjust priorities based on the
project's plugin declarations (e.g., suppress plugin priority to 0 for
undeclared plugins). Source filtering is simpler and more predictable:
a format is either available or it isn't. Priority adjustment would create
subtle ordering changes that are harder to reason about.

**Content resolution in framework vs. consumers.** Moving content pattern
matching (glob, ignore rules, format detection) into the framework means
the framework depends on filesystem operations. This is acceptable because
content resolution is fundamentally a filesystem operation, and the framework
already performs file I/O for format reading/writing.
