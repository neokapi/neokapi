# Go build flags and the shared build cache

Every Go command that `make`, CI and agent sessions run builds with
`-trimpath`. This page says why, what changes for you, and how to get the same
behaviour from `go` in your own terminal.

This is contributor machine setup, so it lives here in `docs/internals/`
rather than on the documentation site.

## Why

Without `-trimpath`, `cmd/go` writes each package's absolute directory into
the package's build cache key (`buildActionID` in
`src/cmd/go/internal/work/exec.go` hashes `dir <p.Dir>`). Two checkouts of the
same commit in different directories then share nothing: each git worktree
compiles the whole repository into cache entries of its own.

Measured on 2026-11-22 with a scratch `GOCACHE`, for
`go test -tags fts5 -run '^$' ./core/... ./host/...` in two directories holding
the same commit:

| | First checkout | Second checkout |
| --- | --- | --- |
| Without `-trimpath` | 117 s, cache 1.30 GB | 81 s, cache 1.99 GB |
| With `-trimpath` | 130 s, cache 1.29 GB | 22 s, cache +3 MB (3 s when test results are reused) |

With five agents in five worktrees, the shared cache grew by 54 GB in two hours
before the flag was set.

## Where the flag is set

| Path | Where |
| --- | --- |
| `make` in the root and in `bowrain/` | `scripts/lib/goflags.mk`, included by both Makefiles, appends `-trimpath` to `GOFLAGS` once and exports it, so every recipe and every script a recipe runs inherits it |
| CI | `.github/actions/setup-go-modules` exports it; a workflow that installs Go without that action sets `GOFLAGS: -trimpath` in its `env` |
| Agent sessions | `.claude/settings.json` sets `GOFLAGS=-trimpath` in `env` |
| Release builds | Each release `go build` passes `-trimpath` itself, as it did before; the shipped binaries do not change |

`scripts/check-go-trimpath.py` (in `make lint`, `make pre-push` and the *Repo
guards* CI job) fails when one of these drifts: a Makefile that stops including
the fragment, a recipe, script or workflow that assigns `GOFLAGS` without the
flag or unsets it, a workflow job that installs Go without setting it, or agent
settings without it.

## Raw `go` in your terminal

A terminal outside `make` and outside an agent session has no `GOFLAGS`, so
`go test ./...` there fills a second cache population beside the shared one and
compiles everything again. To share the cache, export the flag in your shell
profile:

```bash
export GOFLAGS=-trimpath
```

`go env -w GOFLAGS=-trimpath` does the same for every Go project of your user
account. Either one is your choice; the repository sets neither for you.

## What changes when the flag is set

- **Stack traces and test failure traces** name files by module path rather
  than absolute path: `github.com/neokapi/neokapi/core/flow/executor.go:123`.
  An editor that opens a file from an absolute path in terminal output does
  not resolve these. The line numbers are the same.
- **A test that finds its own source file** through `runtime.Caller` gets the
  module path, which is not a file on disk. A test reads files relative to its
  working directory instead: `go test` runs each package's tests in the
  package directory.
- **The debugger.** `dlv debug` and `dlv test` inherit `GOFLAGS`, so the binary
  records module paths and a breakpoint set by file path does not bind. Either
  build the debug binary without the flag:

  ```bash
  GOFLAGS= dlv test ./core/flow -- -test.run TestName
  ```

  or keep the flag and map the module path to your checkout in Delve
  (`config substitute-path github.com/neokapi/neokapi <checkout>`). The first
  writes cache entries of its own, which is the cost of one debug build.
- **Coverage profiles** name files by import path with or without the flag, so
  coverage tooling is unaffected.
- **Analyzers** (`go vet`, `golangci-lint`, `fieldguard`, `govulncheck`) load
  packages through `go list`, which reports absolute file paths either way.
- **cgo** passes `-trimpath` through to the C compiler as a file prefix map, so
  C objects are shared between checkouts as well.
