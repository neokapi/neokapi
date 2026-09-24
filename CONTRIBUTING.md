# Contributing to neokapi

This document covers contributor setup and repository conventions. See
[`web/docs/contribute/`](web/docs/contribute/) for architecture and guides to
extending the engine.

## Repository layout

neokapi is a multi-module Go monorepo coordinated by a `go.work` file at the
root, plus a pnpm workspace for the frontend packages. The framework
(`core/`, `memory/`, `terms/`, `providers/`) stays platform-agnostic; the
`bowrain/` platform attaches through the extension and plugin-registry
mechanisms rather than direct imports. See [`CLAUDE.md`](CLAUDE.md) for the
module map.

## Prerequisites

Beyond a Go toolchain and Node, the build needs **ICU development libraries**:
the SQLite FTS5 ICU tokenizer is compiled through cgo, so content memory and
terms do not build without them.

```bash
brew bundle                                    # macOS — see Brewfile
sudo apt-get install libicu-dev pkg-config     # Debian / Ubuntu
make doctor                                    # report what is still missing
```

`make doctor` is read-only: it checks Go, pkg-config, ICU (through pkg-config,
the same way the cgo build looks for it), Node and `vp`, and reports the install
command for anything absent. Run it first if a build fails in a way that does
not name a cause.

## Building and testing

```bash
make build       # Build the kapi CLI -> bin/kapi
make test        # Run all tests (framework + bowrain)
make check       # fmt + vet + lint
make check-gofmt # Guard: every tracked .go file is gofmt-clean (CI gates on this)
make pre-push    # Run the checks relevant to your changes (mirrors CI)
```

**Use `make`, not a bare `go test ./...`.** The Makefile passes `-tags fts5`
and puts Homebrew's ICU on `PKG_CONFIG_PATH`; without those, `go` fails with
`no such function: fts5` at runtime, or a bare `[build failed]` that never
mentions ICU. `make help` is the catalog of targets.

Run a single test with `go test -tags fts5 ./core/flow/ -run TestName -v`. For
the frontend packages, use `vp` (viteplus) rather than `npx` — e.g.
`vp check --fix` before committing.

Some suites need a PostgreSQL. They skip themselves when neither Docker nor
`BOWRAIN_TEST_POSTGRES_URL` is available, so they are never a blocker locally;
with Docker running, `make test-integration` exercises the cross-store
(SQLite/Postgres) parity lane.

Build and audit targets use default paths for sibling repositories and
reference checkouts. To use a different directory layout, see
[`docs/internals/workspace-paths.md`](docs/internals/workspace-paths.md).
That guide also explains failures from `make check-abs-paths`.

### Formatting: `make fmt` is a fixer, `make check-gofmt` is the check

`make fmt` runs `gofmt -w -s` over the tree. It **rewrites files and exits 0**,
so use `make check-gofmt` (`scripts/check-gofmt.sh`) to detect formatting drift.
That check runs in the *Repo guards* CI job and through `make lint` and
`make pre-push`.

Review comment changes after formatting. `gofmt` interprets `''` and double
backticks in doc comments as TeX quotation marks and replaces them with curly
quotes. This can corrupt a description of an empty SQL string while leaving
valid Go source.

Write such literals as `an empty string` or `""`. If `make check-gofmt` warns
about these quote sequences in a comment, reword the comment before formatting.

## Go conventions

Use functional options (`New(required, ...Option)`) for public framework APIs
under `core/`, `memory/`, `terms/` and `providers/`. This keeps external callers
source-compatible as optional settings are added.

Internal application services in `host/`, `cli/` and `bowrain/` may use a config
struct (`New(Config)`) when constructors and callers evolve together in the
repository.

## Pull requests

- Keep changes focused; one logical change per PR.
- Use clear, conventional commit messages.
- Make sure `make pre-push` and CI are green.
- Add or update tests alongside behavioural changes (table-driven tests are the
  norm; format changes use read -> write -> compare roundtrips).
- Follow the writing and brand conventions in
  [`docs/internals/brand-communication.md`](docs/internals/brand-communication.md)
  for any user-facing prose.

## Licensing of contributions

The framework, CLI, and shared frontend are licensed under Apache-2.0; the
`bowrain/` platform is licensed under AGPL-3.0. By contributing, you agree that
your contributions are licensed under the license that governs the part of the
tree you are changing.
