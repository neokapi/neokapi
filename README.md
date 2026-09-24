# neokapi

[![CI](https://github.com/neokapi/neokapi/actions/workflows/ci.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/ci.yml)
[![Docs: kapi](https://github.com/neokapi/neokapi/actions/workflows/docs-kapi.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/docs-kapi.yml)
[![Pages Deploy](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml/badge.svg)](https://github.com/neokapi/neokapi/actions/workflows/pages-deploy.yml)

> **Experimental.** neokapi is an ongoing experiment and not yet recommended for production use.

neokapi is a content engine in Go. It reads documents into a common content
model, runs editing and checking tools, and writes changes through format-aware
writers. The context graph resolves the terms, voice guidance and recorded
decisions that apply to each piece of content.

The engine supports source-language checks and multilingual workflows,
including translation, content memory reuse and XLIFF/PO interchange. Concurrent
processing flows and pluggable tools build on concepts from the
[Okapi Framework](https://okapiframework.org/). Format fidelity and check coverage
are verified separately; see the [format documentation](web/docs/framework/formats.md).

The Bowrain platform adds shared project context and server workflows. Its code
and documentation are under [`bowrain/`](bowrain/README.md).

## Install

```bash
brew install neokapi/tap/kapi-cli  # macOS/Linux
winget install Neokapi.KapiCli     # Windows
```

Pre-built binaries for Linux, macOS, and Windows (amd64 + arm64) are on the [Releases](https://github.com/neokapi/neokapi/releases) page. Kapi Desktop ships a signed Windows installer and a macOS cask; see the [installation guide](https://neokapi.github.io/kapi/get-started/installation).

## Repository layout

The repository contains the framework, kapi applications and Bowrain platform.

```
core/                       Framework: content model, formats, tools, flows, plugin system
memory/                     Content memory (interface + in-memory + SQLite + matching)
terms/                      Terminology (interface + in-memory + SQLite + import)
providers/                  AI + MT provider integrations
host/                       Cobra-free runtime + services (config, credentials, plugin host)
cli/                        Thin Cobra shell over host (command factories, flags, dispatch)
kapi/                       The kapi CLI: github.com/neokapi/neokapi/kapi
apps/kapi-desktop/          Wails v3 desktop app (Go + React/TS)
packages/                   Apache-licensed pnpm workspace packages (ui, i18n-react, docs-shared, ...)
web/                        Docusaurus docs + landing home → published at the site root
storybook/                  Kapi Storybook (UI primitives + flow editor)
bench/                      Benchmarks
examples/                   Plugin examples
docs/internals/             Internal architecture / interfaces / testing notes
bowrain/                    Bowrain platform (AGPL-3.0; bowrain/plugin/ is Apache-2.0), see bowrain/README.md
```

The Go side is a multi-module workspace coordinated by `go.work`:

| Module          | Path                  | Purpose                                              |
| --------------- | --------------------- | ---------------------------------------------------- |
| **Framework**   | `.` (root)            | Engine: `core/`, `memory/`, `terms/`, `providers/`; context, checks, the loop |
| **Host**        | `host/`               | Cobra-free runtime + services (config, credentials)  |
| **CLI base**    | `cli/`                | Shared Cobra commands + output formatting            |
| **Kapi**        | `kapi/`               | The `kapi` binary: project context, checks, `kapi up`, file processing |
| **Kapi Desktop**| `apps/kapi-desktop/`  | Wails v3 desktop app                                 |
| **Bowrain Core**| `bowrain/core/`       | Shared platform types (see bowrain/README.md)        |
| **Bowrain plugin**| `bowrain/plugin/`   | `kapi-bowrain` plugin (Apache-2.0): project sync, run as `kapi <cmd>` |
| **Bowrain**     | `bowrain/`             | Full platform                                        |

## Quick start

The build needs ICU development libraries: the SQLite FTS5 ICU tokenizer is
compiled through cgo:

```bash
brew bundle                                    # macOS; see Brewfile
sudo apt-get install libicu-dev pkg-config     # Debian / Ubuntu
make doctor                                    # report what is still missing
```

```bash
make build              # Build kapi CLI → bin/kapi
make test               # Run all tests
make check              # fmt + vet + lint

go test -tags fts5 ./core/flow/ -run TestExecutorCancellation -v   # Single test
```

Prefer `make` over a bare `go build` / `go test`: it passes `-tags fts5` and
locates Homebrew's ICU, without which `go` reports `no such function: fts5` or
a bare `[build failed]` that never mentions ICU.

For the bowrain platform (server, desktop app, web app), see [`bowrain/README.md`](bowrain/README.md).

### Frontend / docs site

A single root pnpm workspace coordinates the kapi side:

```bash
vp install                    # at the repo root
cd web && vp run start        # Docusaurus dev server (kapi docs + landing)
make kapi-storybook           # Storybook on :6007
```

## Documentation

- **[kapi docs](https://neokapi.github.io/)**: published Docusaurus site
- **[Architecture](web/docs/contribute/architecture/)**: ADs, one per architectural concern
- **[Implementation notes](web/docs/contribute/implementation/)**: schemas, protocols, algorithms
- **[Internals (root)](docs/internals/)**: repo-wide testing, interfaces, release process

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

Apache 2.0; see [LICENSE](LICENSE).

Code under [`bowrain/`](bowrain/) is AGPL-3.0, except [`bowrain/plugin/`](bowrain/plugin/), the `kapi-bowrain` plugin binary, which is Apache-2.0: it is a client of the server and links nothing else under `bowrain/`. See [bowrain/README.md](bowrain/README.md) for that subtree's licensing and build details.
