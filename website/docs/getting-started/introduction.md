---
sidebar_position: 1
title: Introduction
slug: /getting-started/introduction
---

# Introduction

gokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. It provides format-aware document parsing, channel-based concurrent processing flows, and pluggable tools for localization and translation.

## Why gokapi?

The Okapi Framework is a mature, open-source Java toolkit for localization. However, its Java foundation introduces friction for modern deployment: JVM startup cost, classpath complexity, heavyweight distributions, and limited concurrency primitives.

gokapi reuses Okapi's conceptual model (filters, pipelines, events) but redesigns the APIs around Go idioms: interfaces, channels, goroutines, and composition over inheritance. A Java bridge provides backward compatibility with all existing Okapi filters.

## Key Features

- **15 built-in formats** — HTML, XML, XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX
- **Channel-based pipeline** — concurrent streaming with goroutines, buffered channels, and automatic backpressure
- **AI-native translation** — first-class LLM integration with Anthropic, OpenAI, and Ollama
- **Plugin system** — crash-isolated gRPC plugins in any language, plus a Java bridge for 40+ Okapi filters
- **Translation memory** — built-in Pensieve TM with Levenshtein fuzzy matching and TMX import/export
- **Desktop app** — Bowrain, a cross-platform GUI built with Wails v3, React, and TypeScript
- **Single binary** — CLI, server, and desktop app all compile to standalone executables

## Architecture Overview

Documents flow through a channel-based concurrent pipeline:

```
RawDocument → DataFormatReader → [Tool 1] → [Tool 2] → ... → DataFormatWriter → Output
```

Each tool runs in its own goroutine. Buffered channels provide backpressure. Context cancellation propagates to all stages.

For a detailed architecture overview, see the [Architecture](/docs/developer/architecture) page.

## Terminology

If you're familiar with the Okapi Framework, here's how concepts map:

| Okapi (Java)               | gokapi (Go)                |
|----------------------------|----------------------------|
| Filter                     | DataFormat (Reader/Writer)  |
| Step                       | Tool                       |
| Pipeline                   | Flow                       |
| PipelineDriver             | FlowExecutor               |
| Event                      | Part                       |
| TextUnit                   | Block                      |
| TextFragment               | Fragment                   |
| Code                       | Span                       |
| StartSubDocument/SubFilter | Child Layer                |
| Tikal                      | kapi (CLI)                 |
| Rainbow                    | Bowrain (desktop app)      |
