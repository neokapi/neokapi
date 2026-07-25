# Contributing to neokapi

Thanks for your interest in contributing. This document is a short orientation;
the full contributor guide — architecture, conventions, and how the subsystems
fit together — lives in the documentation under
[`web/docs/contribute/`](web/docs/contribute/).

## Repository layout

neokapi is a multi-module Go monorepo coordinated by a `go.work` file at the
root, plus a pnpm workspace for the frontend packages. The framework
(`core/`, `memory/`, `terms/`, `providers/`) stays platform-agnostic; the
`bowrain/` platform attaches through the extension and plugin-registry
mechanisms rather than direct imports. See [`CLAUDE.md`](CLAUDE.md) for the
module map.

## Building and testing

```bash
make build       # Build the kapi CLI -> bin/kapi
make test        # Run all tests (framework + bowrain)
make check       # fmt + vet + lint
make check-gofmt # Guard: every tracked .go file is gofmt-clean (CI gates on this)
make pre-push    # Run the checks relevant to your changes (mirrors CI)
```

Run a single test with `go test ./core/flow/ -run TestName -v`. For the
frontend packages, use `vp` (viteplus) rather than `npx` — e.g.
`vp check --fix` before committing.

A fresh clone in the conventional layout needs no environment at all. A few
build and audit targets do reach outside this repository — sibling repos and
reference checkouts — and name those locations by environment variable; see
[`docs/internals/workspace-paths.md`](docs/internals/workspace-paths.md) if
your checkouts live somewhere else, or if `make check-abs-paths` fails.

### Formatting: `make fmt` is a fixer, `make check-gofmt` is the check

`make fmt` runs `gofmt -w -s` over the tree. It **rewrites files and exits 0**,
so it can never fail a build — the check that does is `make check-gofmt`
(`scripts/check-gofmt.sh`), which runs in the *Repo guards* CI job and in
`make lint` / `make pre-push`.

**Review the comment diff of a tree-wide format sweep, not just the code
diff.** `gofmt` canonicalises **doc comments**, and its canonicaliser reads
`''` and ` `` ` as TeX-style quotes, rewriting them to `”` and `“`. A sweep can
therefore silently change *prose meaning*: in #1444 a comment reading
`SetupTrial inserted ''` — describing an empty SQL string literal — became
`SetupTrial inserted ”`, which is valid Go and says nothing. Nothing failed.

When a comment needs to mention such a literal, word it (`an empty string`) or
use `""`; both survive gofmt. `make check-gofmt` warns explicitly when an
offending file contains `''` or ` `` ` in a comment, so take that warning as
"reword this", not "run `make fmt`".

## Go conventions

**Constructor style.** Match the constructor to the audience. Framework public
API — the exported surface under `core/`, `memory/`, `terms/`, `providers/`
that external callers and plugins build against — takes **functional options**
(`New(required, ...Option)`), so a call site stays source-compatible as options
grow and defaults stay centralised. Internal services — application wiring in
`host/`, `cli/`, and the `bowrain/` platform — may take a **config struct**
(`New(Config)`); it is plainer and fine when the caller and constructor evolve
together in-tree. Both styles are already common in the codebase; the rule is
just to pick by this boundary rather than by habit.

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
