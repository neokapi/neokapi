# gokapi

[![CI](https://github.com/gokapi/gokapi/actions/workflows/ci.yml/badge.svg)](https://github.com/gokapi/gokapi/actions/workflows/ci.yml)
[![Screenshots & Recordings](https://github.com/gokapi/gokapi/actions/workflows/screenshots-recordings.yml/badge.svg)](https://github.com/gokapi/gokapi/actions/workflows/screenshots-recordings.yml)
[![Docs](https://github.com/gokapi/gokapi/actions/workflows/docs.yml/badge.svg)](https://github.com/gokapi/gokapi/actions/workflows/docs.yml)

> **Experimental:** Gokapi is, like some current government administrations, an ongoing experiment and should not be used in production.

An AI-native reimagining of the [Okapi Framework](https://okapiframework.org/) in Go. Format-aware document parsing, channel-based concurrent processing, and pluggable tools for localization and translation.

## Architecture

Documents flow through a channel-based concurrent pipeline where each tool runs in its own goroutine:

```
RawDocument → DataFormatReader → [Tool 1] → [Tool 2] → ... → DataFormatWriter → Output
                                    ↕            ↕
                              chan *Part    chan *Part
```

Buffered channels provide backpressure. `errgroup.Group` coordinates error handling. Context cancellation propagates to all stages.

The content model uses **Parts** as the fundamental streaming unit:

- **Layer** — structural grouping; layers nest for embedded content (e.g. HTML inside JSON)
- **Block** — translatable content with source/target segments per locale
- **Fragment** — text with inline spans using coded text (Unicode PUA markers)
- **Data** — non-translatable structure
- **Media** — binary content

## Installation

**Homebrew (CLI):**

```bash
brew install --cask gokapi/tap/kapi
```

**Homebrew (Bowrain desktop app, macOS):**

```bash
brew install --cask gokapi/tap/bowrain
```

**Direct download:**

Pre-built binaries for Linux, macOS, and Windows are available on the [Releases](https://github.com/gokapi/gokapi/releases) page.

**From source:**

```bash
go install github.com/gokapi/gokapi/bowrain/cmd/kapi@latest
```

## Quick Start

```bash
# Convert HTML to XLIFF for translation
kapi convert -i document.html -o document.xliff -f html -t xliff2

# Translate using AI (requires API key)
kapi translate -i document.xliff -p anthropic -l fr

# Merge translations back
kapi merge -i document.xliff -o translated.html -f html

# Run a processing flow
kapi flow -c pipeline.yaml

# List supported formats
kapi formats

# List available tools
kapi tools
```

## Supported Formats

| Format | ID | Extensions |
|--------|-----|------------|
| Plain Text | `plaintext` | `.txt` |
| HTML | `html` | `.html`, `.htm` |
| XML | `xml` | `.xml` |
| XLIFF 1.2 | `xliff` | `.xlf`, `.xliff` |
| XLIFF 2.0 | `xliff2` | `.xlf`, `.xliff` |
| YAML | `yaml` | `.yml`, `.yaml` |
| JSON | `json` | `.json` |
| GNU gettext PO | `po` | `.po`, `.pot` |
| Java Properties | `properties` | `.properties` |
| Markdown | `markdown` | `.md`, `.markdown` |
| CSV | `csv` | `.csv` |
| SRT Subtitles | `srt` | `.srt` |
| WebVTT Subtitles | `vtt` | `.vtt` |
| TMX | `tmx` | `.tmx` |

## Applications

### Bowrain

Bowrain is a native desktop translation editor built with [Wails](https://wails.io/) and React. It provides a graphical interface for translation projects with:

- Project management with SQLite-backed content store
- Side-by-side translation editing
- Document preview
- AI-powered translation via Anthropic, OpenAI, and Ollama
- Translation memory (Sievepen)

Build: `cd bowrain/apps/bowrain && wails build`

### REST Server

The `bowrain-server` binary exposes the framework as an Echo v4 REST API:

```bash
make build-server
./bin/bowrain-server
```

## Development

```bash
make build              # Build kapi CLI → bin/kapi
make build-server       # Build REST server → bin/bowrain-server
make test               # Run all tests
make test-unit          # Unit tests only (-short flag)
make test-race          # Tests with race detector
make cover              # Coverage report → coverage/coverage.html
make fmt                # Format Go source
make vet                # Run go vet
make lint               # Run golangci-lint
make check              # fmt + vet + lint
```

Run a single test:

```bash
go test ./flow/ -run TestExecutorCancellation -v
```

Bowrain development:

```bash
cd bowrain/apps/bowrain && wails dev          # Dev mode with hot reload
cd bowrain/apps/bowrain/frontend && npm run test:e2e   # Playwright E2E tests
```

## Plugin System

gokapi uses [HashiCorp go-plugin](https://github.com/hashicorp/go-plugin) with gRPC for out-of-process tool plugins. A Java bridge enables running legacy Okapi filters as gokapi plugins.

```bash
kapi plugins                          # List installed plugins
make build-bridge-jar                 # Build Java bridge
make test-bridge-integration          # Test bridge integration
```

## Documentation

- [Overview](docs/OVERVIEW.md) — project vision and terminology mapping
- [Architecture](docs/ARCHITECTURE.md) — technical architecture
- [Interfaces](docs/INTERFACES.md) — interface definitions and contracts
- [Phases](docs/PHASES.md) — implementation roadmap
- [Testing](docs/TESTING.md) — testing strategy
- [Release](docs/RELEASE.md) — release process

## License

Apache 2.0 — see [LICENSE](LICENSE).
