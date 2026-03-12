# Developing with neokapi and bowrain Side-by-Side

After the repository split, **neokapi** (the framework) and **bowrain** (the
platform) live in separate Git repositories.  Bowrain depends on neokapi but
not the other way around.

This guide explains how to work across both repositories efficiently using a
**parent Go workspace** — a single `go.work` file that spans both repos so
you never need `replace` directives or manual module-proxy hacks.

---

## Directory Layout

Check out both repos under a common parent directory, and create a parent
`go.work` alongside them:

```
~/src/
├── go.work           # Parent workspace (spans both repos)
├── neokapi/          # github.com/neokapi/neokapi  (framework + cli + kapi)
└── bowrain/          # github.com/neokapi/bowrain   (bowrain + bowrain-cli + platform)
```

---

## Parent Workspace Setup (one-time)

Create a single `go.work` at the parent directory that references every
module from both repos:

```bash
cd ~/src
go work init \
    ./neokapi \
    ./neokapi/cli \
    ./neokapi/kapi \
    ./bowrain/bowrain \
    ./bowrain/bowrain-cli \
    ./bowrain/platform
```

This produces:

```go
// ~/src/go.work
go 1.24.0

use (
    ./neokapi
    ./neokapi/cli
    ./neokapi/kapi
    ./bowrain/bowrain
    ./bowrain/bowrain-cli
    ./bowrain/platform
)
```

That's it.  Go now resolves **all** cross-repo imports from local source
automatically — no `replace` directives, no `go work sync` after every edit.
Change a type in `neokapi/core/model/` and bowrain sees it instantly.

> **The parent `go.work` is not committed** — it lives only on your machine.
> Each repo keeps its own `go.work` for CI and standalone use; the parent
> workspace overlays them for local cross-repo development.

Each repo still has its own internal `go.work` for CI and single-repo work:

```
# neokapi/go.work              # bowrain/go.work
use (                           use (
    .                               ./bowrain
    ./cli                           ./bowrain-cli
    ./kapi                          ./platform
)                               )
```

---

## Typical Cross-Repo Workflow

### 1. Make a framework change in neokapi

```bash
cd ~/src/neokapi
# edit core/model/block.go (for example)
go test ./core/model/...       # verify in isolation
```

### 2. Test the change from bowrain (instantly — no extra steps)

```bash
cd ~/src/bowrain
go test ./bowrain/...          # bowrain sees the local neokapi changes
go test ./platform/...
```

Because the parent `go.work` is active, bowrain resolves neokapi from
`../neokapi` automatically.  No `replace` directives to add or remove.

### 3. Commit and push independently

```bash
# neokapi
cd ~/src/neokapi
git add -A && git commit -m "feat(model): add Metadata field to Block"
git push

# bowrain (after neokapi is merged and tagged)
cd ~/src/bowrain
go get github.com/neokapi/neokapi@v0.5.0
go mod tidy
git add -A && git commit -m "chore: bump neokapi to v0.5.0"
git push
```

> **Tip:** The parent workspace is transparent to git.  Each repo's
> `go.mod` files always point at published versions, so CI works without
> any workspace tricks.

---

## Versioning Strategy

neokapi follows **Go module versioning** (semver tags on the repo root):

```
v0.1.0    # framework root module
cli/v0.1.0   # cli sub-module (if tagged separately)
kapi/v0.1.0  # kapi sub-module (if tagged separately)
```

bowrain depends on neokapi via `go.mod`:

```go
require github.com/neokapi/neokapi v0.5.0
require github.com/neokapi/neokapi/cli v0.5.0
```

**Best practice:** Tag and release neokapi first, then update bowrain's
`go.mod` to point at the new tag.

---

## Running Tests Across Both Repos

When running from within either repo directory, the parent `go.work`
ensures cross-repo imports resolve locally.

### neokapi

```bash
cd ~/src/neokapi
make test              # all framework + cli + kapi tests
make test-framework    # framework only
make test-cli          # cli module only
make test-kapi         # kapi CLI only
```

### bowrain

```bash
cd ~/src/bowrain
make test              # all bowrain + bowrain-cli + platform tests
make test-bowrain      # bowrain module only
make test-bowrain-cli  # bowrain CLI only
make test-platform     # platform module only
```

### Full integration (both repos)

```bash
cd ~/src
go test ./neokapi/... ./bowrain/...    # test everything via parent workspace
```

Or use a simple script:

```bash
#!/usr/bin/env bash
set -euo pipefail
echo "=== neokapi ==="
(cd ~/src/neokapi && make test)
echo "=== bowrain ==="
(cd ~/src/bowrain && make test)
```

---

## IDE Setup

### VS Code

Open the **parent directory** (`~/src/`) so gopls picks up the parent
`go.work` and resolves all modules from both repos.  Alternatively, use a
multi-root workspace:

```jsonc
// ~/src/gokapi.code-workspace
{
  "folders": [
    { "path": "neokapi", "name": "neokapi" },
    { "path": "bowrain", "name": "bowrain" }
  ],
  "settings": {
    "gopls": {
      "experimentalWorkspaceModule": true
    },
    "go.goroot": "",
    "go.toolsEnvVars": {
      "GOWORK": "${workspaceFolder}/../go.work"
    }
  }
}
```

The `GOWORK` env var points gopls at the parent workspace so cross-repo
navigation, autocompletion, and refactoring all work seamlessly.

### GoLand / IntelliJ

Open the **parent directory** (`~/src/`) as the project root.  GoLand
detects the `go.work` file automatically and resolves all modules.
Cross-repo "Go to Definition" and "Find Usages" work out of the box.

---

## Avoiding Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| bowrain CI fails because neokapi change isn't published | Always publish neokapi first, then update bowrain `go.mod` |
| `go.sum` mismatches after local development | Run `go mod tidy` in the affected repo before committing |
| IDE can't resolve neokapi types in bowrain | Ensure `GOWORK` points at the parent `go.work`, or open `~/src/` as root |
| Parent `go.work` gets stale after adding a module | Run `go work use ./path/to/new/module` from `~/src/` |
| Import cycle between repos | By design this is impossible — neokapi has zero bowrain imports |

---

## Quick Reference

```bash
# Clone both repos
git clone https://github.com/neokapi/neokapi.git ~/src/neokapi
git clone https://github.com/neokapi/bowrain.git  ~/src/bowrain

# Create the parent workspace (one-time)
cd ~/src
go work init \
    ./neokapi ./neokapi/cli ./neokapi/kapi \
    ./bowrain/bowrain ./bowrain/bowrain-cli ./bowrain/platform

# Daily workflow — no replace directives, no sync steps
cd ~/src/neokapi && go test ./...    # test framework changes
cd ~/src/bowrain && go test ./...    # bowrain sees local neokapi instantly

# Before pushing bowrain (after neokapi is tagged)
cd ~/src/bowrain
go get github.com/neokapi/neokapi@v0.5.0
go mod tidy
```
