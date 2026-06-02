---
sidebar_position: 1
title: Overview
---

# Flows

A flow is a composable pipeline that processes localization files through a
sequence of tools — AI translation, source QA, terminology enforcement, and
more. Flows are a [neokapi engine](https://neokapi.github.io/web/neokapi/)
feature; this page covers how they behave inside a Bowrain project. For the
mechanics of the streaming pipeline, the full tool catalog, and the supported
formats, see the neokapi reference:

- [Tool reference](https://neokapi.github.io/web/neokapi/tools) — every
  processing tool and its inputs and outputs.
- [Format reference](https://neokapi.github.io/web/neokapi/formats) — every
  format reader/writer and its configurable parameters.
- [Flows](https://neokapi.github.io/web/neokapi/docs/framework/flows) — the
  streaming-pipeline model.

## Flows in a synced project

In a Bowrain project, a flow reads the files matching the recipe's `content:`
collections, streams their blocks through the tools in order, and writes the
results back to your local files. You then [`kapi push`](/cli/commands/push)
those changes to the server like any other edit. The flow itself runs locally —
it does not touch the server until you push.

## Built-in flows

kapi ships a small set of composed flows you can run by name:

| Flow               | Description                                                       |
| ------------------ | ----------------------------------------------------------------- |
| `ai-translate`     | Translate with an AI/LLM provider                                 |
| `ai-translate-qa`  | AI translation followed by quality checks                         |
| `pseudo-translate` | Generate pseudo-translations for UI testing                       |
| `qa-check`         | Rule-based quality checks (whitespace, punctuation, placeholders) |
| `tm-leverage`      | Pre-fill translations from translation memory                     |
| `segmentation`     | Split source text into sentence segments                          |

List what is available in your installation — including any tools and formats
added by plugins — rather than relying on a fixed list:

```bash
kapi flows     # composed flows
kapi tools     # individual tools (flow steps)
kapi formats   # supported formats
```

Run a flow:

```bash
# Standalone (no project)
kapi run ai-translate-qa -i input.html -o output.html --source-lang en --target-lang fr

# In a project, against the recipe's content collections
kapi run ai-translate-qa
```

## Custom flows

Define a flow as a YAML file under `.kapi/flows/`, composing the tools you need:

`.kapi/flows/translate-with-qa.yaml`:

```yaml
name: translate-with-qa
description: AI translation with quality checks and terminology enforcement

steps:
  - tool: term-lookup
    config:
      termbase: .kapi/termbase.db

  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3

  - tool: term-enforce
    config:
      termbase: .kapi/termbase.db
      required: true

  - tool: qa-check
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - terminology
```

Run it with `kapi run translate-with-qa`. See [Custom flows](/cli/flows/custom-flows)
for the full recipe, and the [tool reference](https://neokapi.github.io/web/neokapi/tools)
for each step's configurable parameters.

## Next steps

- [Custom flows](/cli/flows/custom-flows)
- [Hooks](/cli/flows/hooks)
- [Run command reference](/cli/commands/run)
- [Server-side flows](/server/flows)
