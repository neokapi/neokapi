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

**Web UI (embedded in kapi serve):**
```bash
make web-deps                        # npm install for web UI
make web-build                       # Build web UI → apps/web/dist/
```

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

## Build Conventions

Always prefer `make` targets over raw `go build` / `go test` commands. The Makefile handles prerequisites (e.g. `make build` requires `make web-build` first for the embedded web UI) and places binaries in `bin/` rather than the repo root. Use direct `go test` only when targeting a specific package or test function.

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

## Demo Recordings & Screencasts

Video recordings serve as documentation and are embedded on the website. **Whenever UI-related code changes, all recordings must be regenerated** as part of the verification process before committing.

### Recording systems

There are three independent recording pipelines:

1. **Bowrain (desktop GUI)** — 13 Playwright scenarios in `apps/bowrain/frontend/e2e/recordings.spec.ts`. Self-contained (auto-starts a Vite dev server).
2. **Web app** — 8 scenarios x 2 themes (dark + light) in `apps/web/e2e/recordings.spec.ts`. Requires a running gokapi-server with Dex OIDC.
3. **CLI** — VHS terminal recordings from `.tape` files in `website/tapes/`. Some tapes require a running server.

### How to regenerate

```bash
# 1. Bowrain recordings (self-contained)
make recordings                  # runs + copies to website/static/video/bowrain/

# 2. Web app recordings (needs Docker stack for real auth)
cd e2e && docker compose up -d   # starts Dex + gokapi-server
# Wait for healthy: curl -sf http://localhost:8080/api/v1/health
cd apps/web && npm run e2e:recordings
THEME=dark  bash apps/web/scripts/copy-recordings.sh
THEME=light bash apps/web/scripts/copy-recordings.sh
cd e2e && docker compose down -v

# 3. CLI recordings (needs VHS: brew install charmbracelet/tap/vhs)
make cli-recordings              # runs tapes + copies to website/static/video/cli/

# Or generate everything at once:
make docs-assets                 # screenshots + recordings + cli-recordings
```

### Real systems, not mocks

All recordings must run against real gokapi infrastructure. Specifically:

- **Authentication & identity**: Use the real Dex OIDC provider via `e2e/docker-compose.yml`. Never mock the auth flow.
- **gokapi-server**: Use the real server binary (locally built or Docker image). Never use a mock API server.
- **Database & storage**: Use a real database instance (the Docker stack provisions one automatically).
- **External integrations** outside the scope of this project (e.g. third-party MT providers, external LLM APIs) may be mocked if needed for isolation.

### Verification checklist for UI changes

Before committing any UI-related change:

1. TypeScript checks pass for all 4 projects (`packages/ui`, `apps/web`, `apps/kapi-web`, `apps/bowrain/frontend`)
2. All unit tests pass (`cd packages/ui && npm test`)
3. All 3 frontend production builds succeed
4. All recordings regenerated and copied to `website/static/video/`
5. Go build succeeds (`make build build-server`)

## Architecture Decision Records

ADRs live in `docs/adr/`. They are organized by architectural concern (content model, plugin system, Java bridge, etc.), not by chronological order. Each ADR should describe the current state of its subsystem as a self-contained document. When a subsystem evolves, update the existing ADR in place rather than appending a new one. Only create a new ADR when a genuinely new architectural concern is introduced.
