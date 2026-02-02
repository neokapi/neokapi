---
id: 001-go-reimagining-of-okapi
sidebar_position: 1
title: "ADR-001: Go Reimagining of Okapi"
---
# ADR-001: Go reimagining of the Okapi Framework

## Context

The [Okapi Framework](https://okapiframework.org/) is a mature, open-source
Java toolkit for localization and translation. It provides format-aware document
parsing (filters), a pipeline execution model, and decades of production-proven
format support. However, its Java foundation introduces friction for modern
deployment: JVM startup cost, classpath complexity, heavyweight distributions,
and limited concurrency primitives.

We needed a localization framework that could:

- Ship as a single static binary (CLI, server, or desktop app)
- Exploit Go's goroutine-based concurrency for parallel document processing
- Integrate natively with LLM APIs for AI-powered translation workflows
- Support a plugin model that isolates crashes and allows multi-language plugins
- Remain compatible with Okapi's 40+ filters during a gradual migration

## Decision

Build gokapi as an AI-native reimagining of Okapi in Go. Reuse Okapi's
conceptual model (filters, pipelines, events) but redesign the APIs around
Go idioms: interfaces, channels, goroutines, and composition over inheritance.

Provide a Java bridge to access existing Okapi filters without porting them
immediately, allowing the two ecosystems to coexist.

### Terminology Mapping

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

### Package Layout

- `core/` -- model types, format/tool/flow interfaces, registry, config,
  encoding
- `formats/` -- built-in format implementations
- `ai/` -- LLM provider interface and AI-powered tools
- `connectors/` -- external translation service integrations
- `lib/pensieve/` -- translation memory system
- `lib/tools/` -- utility tools
- `plugin/` -- plugin system (host, bridge, loader, registry)
- `cmd/kapi/` -- Cobra CLI
- `cmd/gokapi-server/` -- Echo v4 REST API server
- `apps/bowrain/` -- Wails v3 desktop app

## Consequences

- Single-binary distribution via `go build` (CLI, server, desktop app)
- Native concurrency model with goroutines and channels
- Java bridge provides backward compatibility with all Okapi filters
- New formats can be written in Go without JVM dependency
- Terminology is shorter and more intuitive for new contributors
