---
sidebar_position: 1
title: Introduction
slug: /getting-started/introduction
---

# Introduction

neokapi is an open-source localization framework in Go. It provides format-aware document parsing, composable processing tools, and a concurrent streaming pipeline for translation workflows.

## What is neokapi?

neokapi is an open-source localization engine (a Go framework) together with
**kapi**, a command-line tool that exposes the engine for file-based work.

## Kapi CLI

Kapi is a standalone command-line tool for file-based localization tasks:

```bash
# Pseudo-translate a file for UI testing
kapi pseudo-translate messages.json --target-lang qps

# Count words for cost estimation
kapi word-count content/*.md

# Translate with AI
kapi ai-translate -i input.html -o output.html --source-lang en --target-lang fr

# List supported formats
kapi formats

# Manage terminology
kapi termbase import terms.csv --format csv -s en -t fr
```

No project initialization, server, or configuration required — kapi operates directly on files.

## Capabilities

neokapi reads and writes localization, document, data, subtitle, and office
formats, detecting the format from extension, MIME type, or content (see the
[Format Reference](/formats) for the current set). Content moves through a
channel-based pipeline in which each tool runs in its own goroutine, connected
by buffered channels with backpressure.

The processing tools include LLM-assisted translation, QA, and review (Anthropic,
OpenAI, Google Gemini, and Ollama), machine-translation backends (DeepL, Google,
Microsoft, ModernMT, MyMemory), a content-aware translation memory
([Sievepen](/features/translation-memory)) with TMX import/export, and
concept-oriented [terminology](/features/terminology) with pipeline enforcement.
The format and tool sets extend through crash-isolated gRPC plugins and a bridge
to the Java Okapi filters.

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

| Okapi (Java)               | neokapi (Go)               |
| -------------------------- | -------------------------- |
| Filter                     | DataFormat (Reader/Writer) |
| Step                       | Tool                       |
| Pipeline                   | Flow                       |
| PipelineDriver             | Executor                   |
| Event                      | Part                       |
| TextUnit                   | Block                      |
| TextFragment               | Fragment                   |
| Code                       | Span                       |
| StartSubDocument/SubFilter | Child Layer                |
| Tikal                      | kapi (CLI)                 |

## Next Steps

- [Installation](/getting-started/installation) — install kapi CLI
- [Quick Start](/getting-started/quickstart) — process your first file
