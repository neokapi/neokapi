# Developing with neokapi and bowrain Side-by-Side

After the repository split, **neokapi** (the framework) and **bowrain** (the
platform) live in separate Git repositories.  Bowrain depends on neokapi but
not the other way around.

This guide explains how to work across both repositories efficiently using
standard Go tooling.

---

## Directory Layout

Check out both repos under a common parent directory:

```
~/src/
├── neokapi/          # github.com/neokapi/neokapi  (framework + cli + kapi)
└── bowrain/          # github.com/neokapi/bowrain   (bowrain + bowrain-cli + platform)
```

Each repo has its own `go.work` that coordinates its internal modules:

```
# neokapi/go.work
use (
    .
    ./cli
    ./kapi
)

# bowrain/go.work
use (
    ./bowrain
    ./bowrain-cli
    ./platform
)
```

---

## Pointing bowrain at a Local neokapi Checkout

When you change neokapi and want bowrain to pick up those changes without
publishing a release, add `replace` directives to bowrain's `go.work`:

```go
// bowrain/go.work
go 1.26.0

use (
    ./bowrain
    ./bowrain-cli
    ./platform
)

replace (
    github.com/neokapi/neokapi     => ../neokapi
    github.com/neokapi/neokapi/cli => ../neokapi/cli
)
```

> **Why `go.work` and not `go.mod`?**  Workspace-level replacements keep
> `go.mod` files clean for CI and release.  The `go.work` file is typically
> gitignored in bowrain (or committed on a dev branch only).

After adding the replacements, run:

```bash
cd ~/src/bowrain
go work sync
```

Go will now resolve `github.com/neokapi/neokapi` imports from your local
`../neokapi` directory instead of the module proxy.

---

## Typical Cross-Repo Workflow

### 1. Make a framework change in neokapi

```bash
cd ~/src/neokapi
# edit core/model/block.go (for example)
go test ./core/model/...       # verify in isolation
```

### 2. Test the change from bowrain

```bash
cd ~/src/bowrain
# Ensure go.work has the replace directives above
go test ./bowrain/...          # bowrain sees the local neokapi changes
go test ./platform/...
```

### 3. Commit and push independently

```bash
# neokapi
cd ~/src/neokapi
git add -A && git commit -m "feat(model): add Metadata field to Block"
git push

# bowrain (after neokapi is merged and tagged)
cd ~/src/bowrain
# Update go.mod to point at the published neokapi version:
#   go get github.com/neokapi/neokapi@v0.5.0
# Remove the replace directive from go.work
go work sync && go mod tidy
git add -A && git commit -m "chore: bump neokapi to v0.5.0"
git push
```

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
`go.mod` to point at the new tag.  Avoid long-lived `replace` directives
in committed code.

---

## Running Tests Across Both Repos

### neokapi (self-contained)

```bash
cd ~/src/neokapi
make test              # all framework + cli + kapi tests
make test-framework    # framework only
make test-cli          # cli module only
make test-kapi         # kapi CLI only
```

### bowrain (with local neokapi)

```bash
cd ~/src/bowrain
# With replace directives in go.work:
make test              # all bowrain + bowrain-cli + platform tests
make test-bowrain      # bowrain module only
make test-bowrain-cli  # bowrain CLI only
make test-platform     # platform module only
```

### Full integration (both repos)

There is no single `make` target that tests both repos together.  Use a
simple shell script if needed:

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

Open a **multi-root workspace** so that gopls resolves both repos:

```jsonc
// ~/src/gokapi.code-workspace
{
  "folders": [
    { "path": "neokapi", "name": "neokapi" },
    { "path": "bowrain", "name": "bowrain" }
  ],
  "settings": {
    "go.toolsEnvVars": {
      "GOWORK": "auto"
    }
  }
}
```

### GoLand / IntelliJ

Open each repo as a separate project.  GoLand automatically detects `go.work`
files.  For cross-repo navigation, add the neokapi directory as an
**external library** in the bowrain project.

---

## Avoiding Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| bowrain CI fails because neokapi change isn't published | Always publish neokapi first, then update bowrain `go.mod` |
| `go.sum` mismatches after local development | Run `go work sync && go mod tidy` in bowrain before committing |
| IDE can't resolve neokapi types in bowrain | Ensure `go.work` has `replace` directives pointing at the local checkout |
| Accidentally committing `replace` directives | Add a CI check: `grep -r 'replace.*\.\./neokapi' go.work && exit 1` |
| Import cycle between repos | By design this is impossible — neokapi has zero bowrain imports |

---

## Quick Reference

```bash
# Clone both repos
git clone https://github.com/neokapi/neokapi.git ~/src/neokapi
git clone https://github.com/neokapi/bowrain.git ~/src/bowrain

# Set up local development (one-time)
cd ~/src/bowrain
cat >> go.work << 'EOF'

replace (
    github.com/neokapi/neokapi     => ../neokapi
    github.com/neokapi/neokapi/cli => ../neokapi/cli
)
EOF
go work sync

# Daily workflow
cd ~/src/neokapi && go test ./...    # test framework changes
cd ~/src/bowrain && go test ./...    # test with local framework

# Before pushing bowrain
cd ~/src/bowrain
# Remove replace directives from go.work
go get github.com/neokapi/neokapi@latest
go work sync && go mod tidy
```
