---
sidebar_position: 6
title: Flow Steps Format
description: Implementation note — the YAML steps-based flow format that compiles to the internal nodes-and-edges graph, including format detection rules, the FlowStep struct, and the StepsToGraph compilation algorithm.
keywords: [flow steps format, YAML, steps, nodes, edges, StepsToGraph, flow compilation, implementation note]
---

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

## Transformers as ordered steps

Tools that rewrite the source/model (redaction, a simplifier, normalization)
are ordinary entries in `steps:`; there is no separate structural stage
([AD-006](../architecture/006-tool-system.md)). A flow that declares the
removed `source_transforms:` field is rejected by `StepsToGraph` with a
migration error pointing at AD-006 and directing the author to list the
transformers as ordered steps.

```yaml
steps:
  - tool: redact          # applied inline; later steps see the redacted source
  - tool: ai-translate
  - tool: qa-check
```

Transformer ordering is validated by the placement pass
(`core/flow/placement.go`), which runs beside data-flow validation at every
flow build/load gate and emits these diagnostics:

| Rule id | Severity | Trigger |
| --- | --- | --- |
| `transformer-after-target` | error | a transformer follows a step that produces a committed target; exempt when the transformer produces the target port itself (e.g. `unredact`) |
| `transformer-after-remote-egress` | error | a recoverable transformer (`redact`) follows a step with the remote-source-egress side effect; exempt for the step(s) producing an input the transformer's config-resolved contract requires |
| `transformer-late-placement` | warning | a transformer sits later than its earliest valid slot (after its last required input), forcing avoidable overlay rebasing |

## Compilation

`StepsToGraph(spec)` generates:

1. Tool nodes from `steps`, chained sequentially
2. Parallel branches for `parallel:` blocks (tee from previous, join at next)

Auto-assigned IDs follow `tool-N` pattern. Positions auto-layout left-to-right.
The graph is tool nodes only; the flow's source and sink are bindings resolved
at run time ([AD-026](../architecture/026-flow-io-binding.md)), not nodes.

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
