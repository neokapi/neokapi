---
sidebar_position: 1
title: Overview
---

# Flows

A flow is a composable pipeline that processes content files through a
sequence of tools: AI drafting, source checks, term checks, and more. Flows
are a [neokapi engine](https://neokapi.github.io/) feature; this page covers
how they behave inside a Bowrain project. For the mechanics of the streaming
pipeline, the full tool catalog, and the supported formats, see the neokapi
reference:

- [Tool reference](https://neokapi.github.io/tools): every processing tool
  and its inputs and outputs.
- [Format reference](https://neokapi.github.io/formats): every format
  reader/writer and its configurable parameters.
- [Flows](https://neokapi.github.io/framework/flows): the streaming-pipeline
  model.

## Flows in a synced project

In a Bowrain project, a flow reads the source files, streams their blocks through
the tools in order, and, with no `-o`, commits the results to the project store
as overlays (a *process-only* run); `kapi merge` then materializes the target
files (pass `-o` to write a file directly instead). You then
[`kapi push`](/cli/commands/push) those changes to the server like any other
edit. The flow itself runs locally; it does not touch the server until you push.

For the everyday reconcile, [`kapi up`](/cli/commands/up) runs the project's
default flow for you, on the server when the project is connected. Run a named
flow yourself when you want one composition, one pass, and no gate loop.

## Built-in flows

kapi ships a small set of composed flows you can run by name:

| Flow               | Description                                                       |
| ------------------ | ----------------------------------------------------------------- |
| `translate`        | Draft with an AI provider                                         |
| `translate-qa`     | AI drafting followed by quality checks                            |
| `pseudo-translate` | Generate pseudo-translations for UI testing                       |
| `qa`               | Rule-based quality checks (whitespace, punctuation, placeholders) |
| `recycle`          | Pre-fill targets from content memory                              |
| `segmentation`     | Split source text into sentence segments                          |

List what is available in your installation, including any tools and formats
added by plugins, rather than relying on a fixed list:

```bash
kapi flows     # composed flows
kapi tools     # individual tools (flow steps)
kapi formats   # supported formats
```

Run a flow:

```bash
# Standalone (no project)
kapi run translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# In a project, against the recipe's content collections
kapi run translate-qa
```

## Custom flows

Define a flow as a YAML file under `.kapi/flows/`, composing the tools you need.
Step config keys are the tool's own schema keys, in camelCase, as the
[tool reference](https://neokapi.github.io/reference/tools/translate) lists
them; an unrecognized key is ignored.

`.kapi/flows/translate-with-checks.yaml`:

```yaml
name: translate-with-checks
description: AI drafting with quality checks and a term check

steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-5

  - tool: term-check
    config:
      caseSensitive: false

  - tool: qa
    config:
      checkDoubleSpaces: true
      checkPatterns: true
      checkTargetInconsistency: true
```

Run it with `kapi run translate-with-checks`. See
[Custom flows](/cli/flows/custom-flows) for more compositions, and the
[tool reference](https://neokapi.github.io/tools) for each step's
configurable parameters.

## Next steps

- [Custom flows](/cli/flows/custom-flows)
- [Hooks](/cli/flows/hooks)
- [Run command reference](/cli/commands/run)
- [Server-side flows](/server/flows)
