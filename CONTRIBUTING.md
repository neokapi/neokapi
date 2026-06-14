# Contributing to neokapi

Thanks for your interest in contributing. This document is a short orientation;
the full contributor guide — architecture, conventions, and how the subsystems
fit together — lives in the documentation under
[`web/docs/contribute/`](web/docs/contribute/).

## Repository layout

neokapi is a multi-module Go monorepo coordinated by a `go.work` file at the
root, plus a pnpm workspace for the frontend packages. The framework
(`core/`, `sievepen/`, `termbase/`, `providers/`) stays platform-agnostic; the
`bowrain/` platform attaches through the extension and plugin-registry
mechanisms rather than direct imports. See [`CLAUDE.md`](CLAUDE.md) for the
module map.

## Building and testing

```bash
make build       # Build the kapi CLI -> bin/kapi
make test        # Run all tests (framework + bowrain)
make check       # fmt + vet + lint
make pre-push    # Run the checks relevant to your changes (mirrors CI)
```

Run a single test with `go test ./core/flow/ -run TestName -v`. For the
frontend packages, use `vp` (viteplus) rather than `npx` — e.g.
`vp check --fix` before committing.

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
