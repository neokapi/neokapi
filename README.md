# neokapi

[![CI](https://github.com/neokapi/neokapi/actions/workflows/ci.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/ci.yml)
[![Docs — kapi](https://github.com/neokapi/neokapi/actions/workflows/docs-kapi.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/docs-kapi.yml)
[![Pages Deploy](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml)

> **Experimental.** Neokapi is an ongoing experiment and not yet recommended for production use.

neokapi is a format-aware content engine in Go: parse any format — JSON, Markdown, HTML, config, office formats — into one unified content model, edit the content inside it, check it, and write it back byte-for-byte. At heart, it is both a localization engine and the tool that keeps your source content on brand.

The engine carries the [Okapi Framework](https://okapiframework.org/) heritage forward — channel-based concurrent processing and pluggable tools — in an AI-native design. Localization is the flagship application: extraction, translation, TM and termbase, XLIFF/PO interchange, and an Okapi-parity fidelity story. The same engine also serves on-brand source content, AI ingestion, and programmatic editing.

The bowrain platform — the team governance platform for on-brand content and translation, built on neokapi — lives under [`bowrain/`](bowrain/) with its own [README](bowrain/README.md).

## Install

```bash
brew install neokapi/tap/kapi-cli  # macOS/Linux
winget install Neokapi.Kapi        # Windows
```

Pre-built binaries for Linux, macOS, and Windows (amd64 + arm64) are on the [Releases](https://github.com/neokapi/neokapi/releases) page. Kapi Desktop ships a signed Windows installer and a macOS cask — see the [installation guide](https://neokapi.github.io/web/neokapi/kapi/get-started/installation).

## Repository Layout

The framework + kapi CLI live at the root. Companion areas are clearly marked.

```
core/                       Framework: content model, formats, tools, flows, plugin system
sievepen/                   Translation memory (interface + in-memory + SQLite + matching)
termbase/                   Terminology (interface + in-memory + SQLite + import)
providers/                  AI + MT provider integrations
cli/                        Shared CLI base (commands, output, config, credentials)
kapi/                       Standalone CLI tool — github.com/neokapi/neokapi/kapi
apps/kapi-desktop/          Wails v3 desktop app (Go + React/TS)
packages/                   Apache-licensed npm workspaces (UI, kapi-react, docs-shared, ...)
web/                   Docusaurus docs + landing home → /web/neokapi/
storybook/                  Kapi Storybook (UI primitives + flow editor)
bench/                      Benchmarks
examples/                   Plugin examples
docs/internals/             Internal architecture / interfaces / testing notes
bowrain/                    Bowrain platform (AGPL-3.0) — see bowrain/README.md
```

The Go side is a multi-module workspace coordinated by `go.work`:

| Module          | Path                  | Purpose                                              |
| --------------- | --------------------- | ---------------------------------------------------- |
| **Framework**   | `.` (root)            | Engine — `core/`, `sievepen/`, `termbase/`, `providers/` |
| **CLI base**    | `cli/`                | Shared CLI commands + output formatting              |
| **Kapi**        | `kapi/`               | Standalone file-processing CLI                       |
| **Kapi Desktop**| `apps/kapi-desktop/`  | Wails v3 desktop app                                 |
| **Bowrain Core**| `bowrain/core/`       | Shared platform types (see bowrain/README.md)        |
| **Bowrain plugin**| `bowrain/cli/`      | `kapi-bowrain` plugin — project sync, run as `kapi <cmd>` |
| **Bowrain**     | `bowrain/`             | Full platform                                        |

## Quick Start

```bash
make build              # Build kapi CLI → bin/kapi
make test               # Run all tests
make check              # fmt + vet + lint

go test ./core/flow/ -run TestExecutorCancellation -v   # Single test
```

For the bowrain platform (server, desktop app, web app), see [`bowrain/README.md`](bowrain/README.md).

### Frontend / docs site

A single root npm workspace coordinates the kapi side:

```bash
vp install                    # at the repo root
cd web && vp run start  # Docusaurus dev server (kapi docs + landing)
make kapi-storybook           # Storybook on :6007
```

## Documentation

- **[kapi docs](https://neokapi.github.io/web/neokapi/)** — published Docusaurus site
- **[Architecture](web/docs/architecture/)** — ADs, one per architectural concern
- **[Implementation notes](web/docs/notes-internal/)** — schemas, protocols, algorithms
- **[Internals (root)](docs/internals/)** — repo-wide testing, interfaces, release process

## Make targets

```
make build              Build kapi CLI → bin/kapi
make build-all          Build all Go binaries (kapi + bowrain side)
make test               Run all tests
make test-unit          Unit tests only (-short flag)
make test-race          Tests with race detector
make cover              Coverage report → coverage/coverage.html
make fmt                gofmt -w -s
make vet                go vet (all modules)
make lint               golangci-lint (all modules)
make check              fmt + vet + lint
make pre-push           Run checks relevant to your changes (mirrors CI)
```

## License

Apache 2.0 — see [LICENSE](LICENSE).

Code under [`bowrain/`](bowrain/) is AGPL-3.0 — see [bowrain/README.md](bowrain/README.md) for that subtree's licensing and build details.
