---
sidebar_position: 1
title: Introduction
slug: /getting-started/introduction
---

# Introduction

gokapi is an open localization platform built in Go. Bidirectional connectors pull content from CMS platforms, design tools, code repositories, and marketing systems into a versioned content store. Composable tools — translation, QA, terminology enforcement — process content through a concurrent pipeline, and connectors push translations back to the source system.

File-based workflows are fully supported — the `FileConnector` treats local files and format filters as just another integration path — but gokapi is designed around the assumption that most production content lives in systems, not in files on disk.

## Why gokapi?

Traditional localization workflows revolve around file exchange: export content from a system, process it through a chain of tools, and import the result. This approach breaks down as organizations scale — files go stale, deduplication is manual, and automation is fragile.

gokapi takes a connector-first approach. Instead of moving files between systems, connectors maintain a live, bidirectional link to each content source. A versioned content store tracks every block by content hash, so incremental sync only processes what actually changed. Event-driven automation replaces manual handoffs with triggers, quality gates, and webhooks.

The project draws on the conceptual model of the [Okapi Framework](https://okapiframework.org/) — filters, pipelines, events — but redesigns the APIs around Go idioms: interfaces, channels, goroutines, and composition over inheritance. A Java bridge allows reuse of Okapi's 40+ existing filters.

## Key Features

- **Connector-first** — Bidirectional connectors sync content from CMS, design tools, code repos, and marketing platforms. Files are one connector type (`FileConnector`), not the whole story.
- **Versioned content store** — Content-addressed blocks with SHA-256 identity. Deduplication across sources, version history, and incremental sync that only processes what changed.
- **Event-driven automation** — Triggers, quality gates, and webhooks. Content changes flow through rules that run flows, enforce quality, and notify teams.
- **AI-native tools** — First-class LLM integration with Anthropic, OpenAI, and Ollama, plus 5 MT services (DeepL, Google, Microsoft, ModernMT, MyMemory). AI tools compose in the same pipeline as every other tool.
- **15+ formats** — HTML, XML, XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX
- **Channel-based pipeline** — Concurrent streaming with goroutines, buffered channels, and automatic backpressure
- **Plugin system** — Crash-isolated gRPC plugins in any language, plus a Java bridge for 40+ Okapi filters
- **Translation memory** — Built-in Sievepen TM with Levenshtein fuzzy matching and TMX import/export
- **Terminology management** — Bowrain Termbase with concept-oriented TBX-inspired data model and pipeline enforcement tools
- **Desktop, CLI, and server** — Bowrain (cross-platform GUI), `kapi` (CLI), and `bowrain-server` (REST API) all compile to standalone executables from one codebase
- **Progressive complexity** — Day one: CLI on files. Grow into flows, content store, automation, and team collaboration. Same content model at every scale — single binary, no runtime dependencies.

## Architecture Overview

Content flows from source systems through connectors into a versioned store, gets processed by composable tools in a concurrent pipeline, and flows back through connectors to the source systems.

```
                    ┌─────────────────────────────────┐
                    │         Source Systems           │
                    │  CMS · Design · Code · Marketing │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │        Connectors               │
                    │  (bidirectional sync)            │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │    Versioned Content Store       │
                    │  SHA-256 · dedup · history       │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │     Processing Pipeline          │
                    │  Reader → [Tools...] → Writer    │
                    │  TM · Terms · AI · QA · MT       │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │   Automation & Events            │
                    │  triggers · gates · webhooks     │
                    └──────────┬──────────────────────┘
                               │
                    ┌──────────▼──────────────────────┐
                    │        Connectors               │
                    │  (push translations back)        │
                    └─────────────────────────────────┘
```

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
