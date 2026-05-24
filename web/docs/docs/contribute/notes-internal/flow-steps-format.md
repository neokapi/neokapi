# Flow Steps Format

The steps-based flow format provides human-friendly YAML authoring that compiles to the internal nodes+edges graph representation.

## Format Detection

The flow parser auto-detects the format:

- `spec.steps` present -> steps format, compiled via `StepsToGraph()`
- `spec.nodes` present -> graph format, used directly
- Both enveloped (`apiVersion: v1`) and bare YAML are supported

## Steps Format Spec

```go
type FlowStep struct {
    Tool     string         `yaml:"tool,omitempty"`     // tool name
    Config   map[string]any `yaml:"config,omitempty"`   // tool parameters
    Label    string         `yaml:"label,omitempty"`    // display label
    Parallel []FlowStep     `yaml:"parallel,omitempty"` // fan-out branches
}
```

## Compilation

`StepsToGraph(spec)` generates:

1. A reader node (using `spec.input`, default "auto")
2. Tool nodes from steps, chained sequentially
3. Parallel branches for `parallel:` blocks (tee from previous, join at next)
4. A writer node (using `spec.output`, default "auto")

Auto-assigned IDs follow `tool-N` pattern. Positions auto-layout left-to-right.

## Examples

### Linear pipeline

```yaml
steps:
  - tool: tm-leverage
    config: { fuzzyThreshold: 75 }
  - tool: ai-translate
  - tool: qa-check
```

### Fan-out

```yaml
steps:
  - parallel:
      - tool: ai-translate
      - tool: word-count
  - tool: qa-check
```

### Script step

```yaml
steps:
  - tool: script
    label: Filter long segments
    config:
      code: |
        if (part.type === "block") {
          if (part.block.source[0].content.text.length > 200) emit(part);
          else skip();
        } else emit(part);
```
