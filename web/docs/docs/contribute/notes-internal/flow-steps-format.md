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

## Source-transform stage

An optional `source_transforms:` list declares the leading **source-transform
stage** — tools that rewrite the source/model (redaction, a simplifier,
normalization) and run *before* the main steps, so downstream tools see one
settled, canonical source ([AD-006](../architecture/006-tool-system.md)). It
takes the same `FlowStep` shape as `steps`. Only source-transform-capable tools
(those that may rewrite source) are permitted here; placing any other tool in
the stage is rejected at flow-resolution time.

```yaml
source_transforms:
  - tool: redact          # settle the model first
steps:
  - tool: ai-translate    # downstream sees the redacted source
  - tool: qa-check
```

## Compilation

`StepsToGraph(spec)` generates:

1. Source-transform tool nodes from `spec.source_transforms`, chained first and
   marked `stage: source-transform`
2. Tool nodes from `steps`, chained sequentially after the source-transform stage
3. Parallel branches for `parallel:` blocks (tee from previous, join at next)

Auto-assigned IDs follow `tool-N` pattern. Positions auto-layout left-to-right,
so the graph order is source-transforms → main tools. The graph is tool nodes
only; the flow's source and sink are bindings resolved at run time
([AD-026](../architecture/026-flow-io-binding.md)), not nodes.

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
