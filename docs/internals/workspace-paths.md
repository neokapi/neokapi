# Workspace paths

How neokapi names locations outside the repository (`NEOKAPI_WORKSPACE_DIR`,
`NEOKAPI_CHECKOUTS_DIR`, `NEOKAPI_OKAPI_DIR` and `NEOKAPI_DOCLANG_DIR`) and the
CI guard that keeps absolute home paths out of the tree.

This is contributor machine setup, not product documentation, so it lives here
in `docs/internals/` rather than on the documentation site.

A few build and audit targets reach outside this repository: sibling repos in
the multi-repo workspace, and reference checkouts of unrelated projects. Those
locations differ per contributor, so they are named by environment variable
with a repo-relative default. **No absolute home path
(`/Users/<name>`, `/home/<name>/`, `C:\Users\<name>`) may be committed.**

A fresh clone in the conventional layout needs no environment at all; a
different layout sets only the variable it needs.

## The variables

| Variable | Default | What it names |
| --- | --- | --- |
| `NEOKAPI_WORKSPACE_DIR` | the parent of this repo | The multi-repo workspace: this repo plus its siblings (`okapi-bridge/`, `registry/`, `homebrew-tap/`, …) |
| `NEOKAPI_CHECKOUTS_DIR` | the parent of `NEOKAPI_WORKSPACE_DIR` | Where unrelated reference checkouts live |
| `NEOKAPI_OKAPI_DIR` | `$NEOKAPI_CHECKOUTS_DIR/okapi/Okapi` | The upstream Okapi Framework (Java) clone, pinned to v1.48.0, the ground truth for parity, the contract audit, and fixture harvesting |
| `NEOKAPI_DOCLANG_DIR` | `$NEOKAPI_CHECKOUTS_DIR/doclang-project/doclang` | The DocLang specification checkout, referenced by the format-ops research notes |

The conventional layout the defaults assume:

```
<checkouts>/                      NEOKAPI_CHECKOUTS_DIR
├── neokapi/                      NEOKAPI_WORKSPACE_DIR
│   ├── neokapi/                  this repo
│   ├── okapi-bridge/
│   └── registry/ …
├── okapi/Okapi/                  NEOKAPI_OKAPI_DIR
└── doclang-project/doclang/      NEOKAPI_DOCLANG_DIR
```

Targets that consumed a hardcoded path keep their historical names as
overrides (`OKAPI_REPO` defaults to `$(NEOKAPI_OKAPI_DIR)`,
`OKAPI_BRIDGE_REPO` to `$NEOKAPI_WORKSPACE_DIR/okapi-bridge`), so existing
invocations and the CI workflows that already set them are unaffected.

## Using them

The root `Makefile` defines and exports all four, so recursive makes and any
process a target spawns inherit them. `make workspace-paths` prints what they
resolve to on your machine.

Shell scripts source the shared resolver rather than reimplementing it, and
never read `$HOME`, which points at the wrong tree the moment a contributor
keeps their checkouts anywhere else:

```bash
root="$(cd "$(dirname "$0")/.." && pwd)"
. "$root/scripts/lib/workspace.sh"
neokapi_init_workspace "$root"
```

Prose refers to the variable rather than to any resolved path: write
`$NEOKAPI_OKAPI_DIR/okapi/filters/`, not a specific directory.

### Worktrees

The workspace is the parent of the **main** checkout, not of the current tree.
Inside a linked git worktree (`.claude/worktrees/<name>/`) the parent is
`.claude/worktrees`, which is not a workspace. Both the Makefile and
`scripts/lib/workspace.sh` therefore derive the main checkout from
`git rev-parse --git-common-dir`, falling back to the parent directory outside
git (a source tarball, say). One place resolves the paths independently,
`.skills/refresh-format-maturity/scripts/audit-format.py`, which is Python, and
it repeats the same derivation. Anything new should source the shell helper
rather than add a fourth copy.

## The guard

`scripts/check-abs-paths.sh` scans every tracked file and fails on any absolute
home path. It runs as part of `make lint`, at the top of `make pre-push`
(ungated: an absolute path can land in any file), and in the *Repo guards* job
of `.github/workflows/ci.yml`.

```bash
make check-abs-paths                  # scan tracked files
./scripts/check-abs-paths.sh --self-test   # prove the matcher both ways
```

The script carries a small, explicit allowlist, documented inline: placeholder
user names used as documentation stand-ins (`me`, `dev`, `demo`, `user`,
`test`, `you`); fixed system paths that merely live under `/home/`
(`linuxbrew`, `runner`, `node`); and a handful of vendored or generated files
that reproduce an upstream author's path in content this repository does not
write. Extend it only with the same specificity, and say why in the comment.

## Generated artefacts

An absolute path can reach a committed file indirectly. `go test -json` records
subtest names, and the contract-audit dashboard dataset
(`web/static/data/contract-audit.json`) is generated from that output, so a
subtest named after an absolute fixture path put a home directory into a
tracked file. Name subtests after a repo-relative path (see `fixtureName` in
`core/formats/openxml/validity_test.go`); the guard catches the artefact if a
generator regresses.
