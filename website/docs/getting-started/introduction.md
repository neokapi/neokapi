---
sidebar_position: 1
title: Introduction
slug: /getting-started/introduction
---

# Introduction

neokapi is an open-source localization framework in Go. It provides format-aware document parsing, composable processing tools, and a concurrent streaming pipeline for translation workflows.

## What is neokapi?

neokapi consists of two products:

- **Neokapi Framework + Kapi CLI** — open-source localization engine and command-line tool for file processing
- **Bowrain Platform** — full-stack localization platform with web editor, desktop app, and server

This section covers the framework and kapi CLI. For the Bowrain platform, see [Bowrain Introduction](/bowrain/introduction).

## Kapi CLI

Kapi is a standalone command-line tool for file-based localization tasks:

```bash
# Pseudo-translate a file for UI testing
kapi pseudo-translate messages.json --target-lang qps

# Count words for cost estimation
kapi word-count content/*.md

# Translate with AI
kapi flow run ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# List supported formats
kapi formats

# Manage terminology
kapi termbase import terms.csv --format csv -s en -t fr
```

No project initialization, server, or configuration required — kapi operates directly on files.

## Key Features

- **15+ formats** — HTML, XML, XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX
- **Channel-based pipeline** — Concurrent streaming with goroutines, buffered channels, and automatic backpressure
- **AI-native tools** — LLM integration with Anthropic, OpenAI, and Ollama, plus 5 MT services (DeepL, Google, Microsoft, ModernMT, MyMemory)
- **Translation memory** — Built-in Sievepen TM with Levenshtein fuzzy matching and TMX import/export
- **Terminology management** — Concept-oriented termbase with pipeline enforcement tools
- **Plugin system** — Crash-isolated gRPC plugins in any language, plus the Okapi bridge for 40+ additional filters
- **Quality assurance** — Rule-based and AI-powered QA checks

## Architecture Overview

Content flows through a concurrent streaming pipeline:

```
Input File → DataFormatReader → [Tool 1] → [Tool 2] → ... → DataFormatWriter → Output File
                                    ↕            ↕
                              chan *Part    chan *Part
```

Each tool runs in its own goroutine. Buffered channels provide backpressure. Context cancellation propagates to all stages.

## Terminology

If you're familiar with the Okapi Framework, here's how concepts map:

| Okapi (Java) | neokapi (Go) |
|---|---|
| Filter | DataFormat (Reader/Writer) |
| Step | Tool |
| Pipeline | Flow |
| PipelineDriver | FlowExecutor |
| Event | Part |
| TextUnit | Block |
| TextFragment | Fragment |
| Code | Span |
| StartSubDocument/SubFilter | Child Layer |
| Tikal | kapi (CLI) |
| Rainbow | Bowrain (desktop app) |

## Next Steps

- [Installation](/docs/getting-started/installation) — install kapi CLI
- [Quick Start](/docs/getting-started/quickstart) — process your first file
- [Bowrain Platform](/bowrain/introduction) — team collaboration with Bowrain CLI, web editor, and server
