# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

gokapi is an AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. It provides format-aware document parsing, channel-based concurrent processing flows, and pluggable tools for localization and translation. The module path is `github.com/gokapi/gokapi`.

## Build & Test Commands

```bash
make build              # Build kapi CLI → bin/kapi
make build-server       # Build REST server → bin/gokapi-server
make build-all          # Build all Go binaries
make test               # Run all tests (go test ./... -count=1)
make test-unit          # Unit tests only (-short flag)
make test-race          # Tests with race detector
make test-verbose       # Verbose test output
make cover              # Coverage report → coverage/coverage.html
make fmt                # Format Go source (gofmt -w -s)
make vet                # Run go vet
make lint               # Run golangci-lint (install via: make tools)
make check              # fmt + vet + lint
make deps               # Download and tidy Go modules
make proto              # Generate gRPC code from protobuf definitions
```

Run a single test: `go test ./core/flow/ -run TestExecutorCancellation -v`

**Bowrain (desktop GUI):**
```bash
cd apps/bowrain && wails3 build       # Build native macOS/Linux/Windows app
cd apps/bowrain && wails3 dev         # Dev mode with hot reload
make frontend-deps                    # npm install for frontend
make frontend-build                   # Production frontend build
```

**Documentation site:**
```bash
cd website && npm start              # Dev server with hot reload
cd website && npm run build          # Production build → website/build/
```

## Architecture

### Streaming Pipeline

Documents flow through a channel-based concurrent pipeline:

```
RawDocument → DataFormatReader → [Tool 1] → [Tool 2] → ... → DataFormatWriter → Output
                                    ↕            ↕
                              chan *Part    chan *Part
```

Each tool runs in its own goroutine. Buffered channels (default 64) provide backpressure. `errgroup.Group` coordinates error handling. Context cancellation propagates to all stages.

### Content Model (core/model/)

The Part is the fundamental streaming unit, carrying a PartType discriminator and a Resource:

- **Layer** — structural grouping (document, section, embedded content). Layers nest: embedded content (HTML inside JSON) becomes a child Layer with its own DataFormat.
- **Block** — translatable content with Source segments and Target segments per locale
- **Fragment** — text with inline Spans using coded text (Unicode private use area markers replace inline markup)
- **Data** — non-translatable structure
- **Media** — binary content

### Key Interfaces

- `format.DataFormatReader` — `Open(ctx, doc)` then `Read(ctx) <-chan PartResult`
- `format.DataFormatWriter` — `SetOutput(path)`, `Write(ctx, <-chan *Part)`
- `tool.Tool` — `Process(ctx, in <-chan *Part, out chan<- *Part) error`
- `flow.FlowExecutor` — orchestrates tool chains with goroutines and channels
- `registry.FormatRegistry` — factory registry for readers/writers with format detection
- `ai/provider.LLMProvider` — interface for Anthropic, OpenAI, Ollama backends

### Terminology Mapping from Okapi

| Okapi (Java) | gokapi (Go) |
|---|---|
| Filter | DataFormat (Reader/Writer) |
| Step | Tool |
| Pipeline | Flow |
| PipelineDriver | FlowExecutor |
| Event | Part |
| TextUnit | Block |
| TextFragment | Fragment |
| Code | Span |
| StartSubDocument/StartSubFilter | Child Layer |

## Package Layout

- `core/` — model types, format/tool/flow interfaces, registry, config, encoding
- `formats/` — 15 built-in format implementations (html, xml, xliff, xliff2, json, yaml, po, properties, plaintext, markdown, csv, srt, vtt, tmx). Each has reader.go, writer.go, config.go. Registration in `register.go`
- `ai/` — LLM provider interface + implementations (anthropic, openai, ollama), AI-powered tools (translate, QA, terminology, review), prompt templates
- `mt/` — machine translation provider interface + implementations (deepl, google, microsoft, modernmt, mymemory), MT translate tool
- `lib/sievepen/` — translation memory system with Levenshtein fuzzy matching and TMX import/export
- `lib/tools/` — utility tools (wordcount, charcount, pseudo-translation, search/replace)
- `plugin/` — HashiCorp go-plugin + gRPC plugin system (host, server, proto definitions, Java bridge)
- `cmd/kapi/` — Cobra CLI (convert, translate, extract, merge, flow, formats, tools, plugins)
- `cmd/gokapi-server/` — Echo v4 REST API server
- `apps/bowrain/` — Wails v3 desktop app (Go backend + React 19/TypeScript/Vite frontend)
- `internal/testutil/` — shared test helpers
- `docs/` — ARCHITECTURE.md, INTERFACES.md, TESTING.md, RELEASE.md, adr/

## Implementing a New Format

Create a package under `formats/` with reader.go, writer.go, config.go. The reader must implement `format.DataFormatReader` (embed `format.BaseFormatReader`). The writer must implement `format.DataFormatWriter` (embed `format.BaseFormatWriter`). Register both in `formats/register.go` via `init()`.

## Implementing a New Tool

Create a type embedding `tool.BaseTool` and set `HandleBlockFn` / `HandleDataFn` / `HandleMediaFn` function fields for the part types you want to process. Parts you don't handle pass through unchanged. Register in the tool registry.

## Testing

Tests use `github.com/stretchr/testify` (assert/require). Table-driven tests are the standard pattern. Format tests typically do roundtrip validation (read → write → compare). Test files colocate with implementation (`*_test.go`).

## Architecture Decision Records

ADRs live in `docs/adr/`. They are organized by architectural concern (content model, plugin system, Java bridge, etc.), not by chronological order. Each ADR should describe the current state of its subsystem as a self-contained document. When a subsystem evolves, update the existing ADR in place rather than appending a new one. Only create a new ADR when a genuinely new architectural concern is introduced.
