---
id: 040-tool-parameter-schemas
sidebar_position: 40
title: "AD-040: Tool Parameter Schemas"
---

# AD-040: Tool parameter schemas

## Context

neokapi tools declare their parameters imperatively: each tool manually
registers cobra flags via `AddFlags(cmd)` and reads them back in
`NewTool(cmd, targetLang)`. The visual flow editor stores tool configuration
as `map[string]any` in `FlowNode.Config` without any schema awareness.
Plugin tools have no way to advertise their parameters to the host system.

The format side already solved this with `FilterSchema` — JSON Schema with
`x-groups` and `x-widget` extensions that power schema-driven config editors,
validation, and the Okapi bridge parameter pipeline. Extending this pattern to
tools is a natural evolution.

## Decision

### Generalized ComponentSchema

A new `core/schema/` package provides generalized schema types. The core type
is `ComponentSchema` — a JSON Schema subset with neokapi extensions:

```go
type ComponentSchema struct {
    ID          string
    Title       string
    Description string
    Type        string                    // "object"
    Meta        ComponentMeta             // id, type, category, displayName
    Groups      []ParameterGroup          // UI groupings
    Properties  map[string]PropertySchema // parameter definitions
}
```

`FilterSchema` (used by Okapi bridge filters) is a specialization that embeds
`ComponentSchema` and adds filter-specific metadata (`x-filter`, `FlatProperties`,
`SectionMap`).

### Reflection-Based Schema Generation

`schema.FromStruct(cfg, meta)` generates a `ComponentSchema` by reflecting on a
Go struct. It maps Go types to JSON Schema types and supports struct tags for
additional metadata:

```go
type PseudoConfig struct {
    ExpansionPercent int    `schema:"description=Text expansion percentage,min=0,max=200"`
    Prefix           string `schema:"description=Prefix for pseudo text"`
    Suffix           string `schema:"description=Suffix for pseudo text"`
    InternalField    string `schema:"-"` // excluded from schema
}
```

The `schema:"-"` tag excludes a field from the generated schema entirely.

`schema.ApplyConfig()` bridges `map[string]any` configuration (e.g., from flow
YAML) to a typed struct via JSON round-trip: it marshals the map to JSON, then
unmarshals into the target struct.

### Tool Schema Provider

Tools optionally implement `tool.SchemaProvider`:

```go
type SchemaProvider interface {
    Schema() *schema.ComponentSchema
}
```

`BaseTool` has a `SchemaFn` field for this. The `ToolRegistry` accepts schemas
via `RegisterWithSchema(name, factory, schema)`.

`ToolCommandDef` supports `NewToolFromConfig` and `Schema` fields that drive
auto-generation. When `Schema` is set, `RegisterSchemaFlags()` generates cobra
flags from the schema, and `NewToolFromConfig` receives the resolved config map.

### Schema-Driven CLI Flags

`cli.RegisterSchemaFlags(cmd, schema)` generates cobra flags from a
`ComponentSchema`, mapping property names from camelCase to kebab-case
and types to appropriate flag types. `ReadSchemaFlags` reads the values
back. `ToolCommandDef` supports a `Schema` field alongside the legacy
`AddFlags` for incremental migration.

`kapi tools schema <name>` prints the JSON Schema for any tool.

### Steps-Based YAML Flow Format

A human-friendly steps format compiles to the internal graph representation:

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  input: auto
  output: auto
  steps:
    - tool: tm-leverage
      config:
        fuzzyThreshold: 75
    - tool: ai-translate
      config:
        provider: anthropic
    - tool: qa-check
```

`parallel:` blocks provide fan-out. `StepsToGraph()` compiles to nodes+edges
for the executor and visual editor. Both formats are auto-detected by the YAML
parser.

### Script Step

A `script` tool runs user-provided JavaScript (ES5) via the goja runtime.
Each tool instance owns its own `goja.Runtime` (safe: one goroutine per tool
instance via `ToolFactory`). The JS API exposes `part`, `emit()`, `skip()`,
and `log()` for filtering and transforming parts.

### Fan-Out and Batching

`tool.Tee()` copies parts to N output channels for fan-out flows. A `batch`
tool collects blocks into configurable batches for downstream batch processing
(e.g., batch MT APIs). Both are registered with schemas.

### Okapi Steps as Tools

The okapi-bridge is extended to expose Okapi pipeline steps as neokapi tools:

- `StepRegistry` discovers `BasePipelineStep` classes via classpath scanning
- `StepSchemaGenerator` produces JSON Schemas from `@UsingParameters`
- A `ProcessStep` gRPC RPC streams parts through step execution
- `BridgeStepTool` adapts the gRPC stream to the `tool.Tool` interface

### Visual Flow Editor

The `FilterConfigEditor` component (already schema-driven) is generalized to
accept `ComponentSchema`. When a tool node is selected in the FlowBuilder,
a side panel shows the schema-driven config form. Config values persist in
`FlowNode.config` and round-trip through save/load.

## Alternatives Considered

- **Separate type system for tools**: Would duplicate the PropertySchema,
  ParameterGroup, and UI widget infrastructure. Extending the existing filter
  schema system avoids this.

- **Code generation for tool schemas**: More precise but requires a build step.
  Reflection-based generation from config structs is simpler and keeps schemas
  in sync with Go types automatically.

- **TypeScript for script step**: Better DX with type hints but requires a
  transpile step. Plain JS via goja is simpler and sufficient for the
  typically short scripts used in localization flows.

## Consequences

- All 24 builtin tool commands use schema-driven CLI flags (all migrated from manual `AddFlags`)
- AI tool schemas include provider fields (Provider, APIKey, Model) with enum support for provider selection
- The visual flow editor shows schema-driven config forms for tool nodes
- Plugin tools can advertise parameters via manifests
- Flow definitions can be validated against tool schemas
- The steps-based YAML format makes flow authoring accessible to non-developers
- Okapi's 100+ pipeline steps become available as neokapi tools
- The script step enables lightweight custom transformations without Go code
