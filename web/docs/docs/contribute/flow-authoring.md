---
sidebar_position: 4
title: YAML Flow Authoring
description: How to write neokapi flows as YAML — the steps-based human-authored format, sequential and parallel branches, tool configuration, and how the YAML compiles to the internal nodes-and-edges graph.
keywords: [flow authoring, YAML flows, steps format, parallel steps, pipeline YAML, neokapi, flow definition]
---

# YAML Flow Authoring

Flows define processing pipelines as YAML files. The steps-based format is the human-authored representation that compiles to an internal graph (nodes + edges) for execution.

## Steps format

A flow is a list of sequential steps. Each step references a tool by name and optionally provides configuration:

```yaml
steps:
  - tool: pseudo-translate
    config:
      targetLocale: fr
      expansionPercent: 30
      prefix: "["
      suffix: "]"
```

### Input and output formats

Specify input/output formats at the top level. When omitted, both default to `auto` (format detection from file extension and content):

```yaml
input: json
output: json
steps:
  - tool: pseudo-translate
    config:
      targetLocale: fr
```

### Step labels

Add a `label` for readability in the UI graph view:

```yaml
steps:
  - tool: pseudo-translate
    label: Generate test translations
    config:
      targetLocale: fr
```

## Sequential steps

Steps execute in order. The output channel of one tool feeds into the input channel of the next:

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: true

  - tool: search-replace
    config:
      search: "TODO"
      replace: ""
      target: true

  - tool: qa-check
    config:
      targetLocale: fr
```

This creates a three-stage pipeline: create target segments, clean up placeholder text, then run quality checks.

## Parallel blocks for fan-out

Use `parallel:` to run multiple tools concurrently on the same stream of Parts. Each branch receives a copy of the input and produces independent output:

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: true

  - parallel:
      - tool: word-count
        label: Count words
        config:
          targetLocale: fr
      - tool: qa-check
        label: Quality checks
        config:
          targetLocale: fr
      - tool: chars-listing
        label: Character inventory
        config:
          targetLocale: fr
```

All three analysis tools run at the same time, each in its own goroutine.

## How steps compile to the graph

The `StepsToGraph()` function transforms a `StepsSpec` into `FlowNode` and `FlowEdge` slices:

1. A **reader** node is created from the `input` format (default: `auto`)
2. Each sequential step becomes a **tool** node, chained by edges
3. A `parallel:` block creates multiple tool nodes, all connected from the previous node (fan-out)
4. After a parallel block, subsequent steps connect from all branch endpoints (fan-in)
5. A **writer** node is created from the `output` format (default: `auto`)

The resulting graph is what the `Executor` runs -- each node becomes a goroutine connected by buffered channels.

## Example flows

### Translation pipeline

A typical translation flow with TM leverage, AI translation for new segments, and quality checks:

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: false

  - tool: tm-leverage
    label: Apply TM matches
    config:
      targetLocale: fr
      threshold: 0.7

  - tool: ai-translate
    label: Translate remaining
    config:
      targetLocale: fr
      provider: anthropic

  - tool: qa-check
    label: Quality checks
    config:
      targetLocale: fr
```

### Fan-out analysis

Run multiple analysis tools in parallel after pseudo-translation:

```yaml
steps:
  - tool: pseudo-translate
    config:
      targetLocale: qps-ploc
      expansionPercent: 30

  - parallel:
      - tool: word-count
        config:
          targetLocale: qps-ploc
      - tool: length-check
        config:
          targetLocale: qps-ploc
          maxChars: 200
      - tool: qa-check
        config:
          targetLocale: qps-ploc
```

### Script filtering

Use the JavaScript script step to filter or transform parts programmatically:

```yaml
steps:
  - tool: script
    label: Skip short segments
    config:
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          if (text.length < 3) {
            skip();
          }
        }

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

## Running flows

### From the CLI

```bash
# Run a built-in composed flow
kapi run ai-translate-qa -i input.xliff --target-lang fr

# Run a flow defined in a .kapi project file
kapi run my-flow -p myproject.kapi -i input.json

# List available flows
kapi flows
```

### Programmatically

```go
spec := &flow.StepsSpec{
    Input: "json",
    Steps: []flow.FlowStep{
        {Tool: "pseudo-translate", Config: map[string]any{
            "targetLocale": "fr",
            "expansionPercent": 30,
        }},
        {Tool: "qa-check", Config: map[string]any{
            "targetLocale": "fr",
        }},
    },
}

nodes, edges, err := flow.StepsToGraph(spec)
// Build and execute with Executor...
```
