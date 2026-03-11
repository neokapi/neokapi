#!/usr/bin/env bash
# migrate-split.sh
#
# Splits the gokapi/gokapi monorepo into two standalone repositories:
#
#   1. neokapi  (github.com/neokapi/neokapi)
#      - framework core, shared CLI base, kapi CLI, examples, benchmarks,
#        shared UI packages, website, docs, scripts, assets
#
#   2. bowrain  (github.com/neokapi/bowrain)
#      - Bowrain platform (server, desktop, connectors), Bowrain CLI,
#        platform module, infrastructure (compose, deploy, docker, e2e)
#
# Both repos get the full relevant git history.  All "gokapi" references are
# renamed to "neokapi" (casing preserved).  Module paths are updated
# consistently across go.mod, go.sum, and source files.
#
# ── Requirements ──────────────────────────────────────────────────────────────
#   git >= 2.25
#   git-filter-repo >= 2.38  (pip install git-filter-repo)
#
# ── Quick start ───────────────────────────────────────────────────────────────
#   ./scripts/migrate-split.sh
#       # Reads current repo, writes to /tmp/gokapi-split/neokapi
#       # and /tmp/gokapi-split/bowrain
#
#   ./scripts/migrate-split.sh --source https://github.com/gokapi/gokapi.git \
#       --output ~/repos/migration
#
# ── Options ───────────────────────────────────────────────────────────────────
#   --source <path|url>   Repo to split (default: current repo root)
#   --output <dir>        Parent directory for the two new repos
#                         (default: /tmp/gokapi-split)
#   --only <repo>         Only produce one repo: "neokapi" or "bowrain"
#   --no-cleanup          Keep temporary working clones after completion
#   --dry-run             Print the plan without executing any git commands
#   -h, --help            Show this help

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(git -C "$SCRIPT_DIR" rev-parse --show-toplevel 2>/dev/null || echo "$SCRIPT_DIR/..")"

# ── Defaults ──────────────────────────────────────────────────────────────────
SOURCE="${SOURCE:-$REPO_ROOT}"
OUTPUT_DIR="${OUTPUT_DIR:-/tmp/gokapi-split}"
ONLY="${ONLY:-}"       # empty = both repos
NO_CLEANUP="${NO_CLEANUP:-false}"
DRY_RUN="${DRY_RUN:-false}"

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --source)     SOURCE="$2";     shift 2 ;;
        --output)     OUTPUT_DIR="$2"; shift 2 ;;
        --only)       ONLY="$2";       shift 2 ;;
        --no-cleanup) NO_CLEANUP=true; shift ;;
        --dry-run)    DRY_RUN=true;    shift ;;
        -h|--help)
            sed -n '/^# migrate-split/,/^set -euo/{ /^set -euo/d; s/^# \{0,2\}//; p }' "$0"
            exit 0 ;;
        *) echo "error: unknown option: $1" >&2; exit 1 ;;
    esac
done

# ── Utilities ─────────────────────────────────────────────────────────────────
info()   { printf '  ▸ %s\n' "$*"; }
step()   { printf '\n══ %s ══\n' "$*"; }
warn()   { printf '  ⚠ %s\n' "$*" >&2; }
die()    { printf '\nerror: %s\n' "$*" >&2; exit 1; }
run()    {
    if [[ "$DRY_RUN" == true ]]; then
        printf '  [dry-run] %s\n' "$*"
    else
        "$@"
    fi
}

# ── Prerequisites ─────────────────────────────────────────────────────────────
step "Checking prerequisites"

command -v git          &>/dev/null || die "git not found"
command -v python3      &>/dev/null || die "python3 not found (required for post-processing)"
command -v git-filter-repo &>/dev/null \
    || git filter-repo --version &>/dev/null 2>&1 \
    || die "git-filter-repo not found.
  Install with:  pip install git-filter-repo
             or: pipx install git-filter-repo
             or: brew install git-filter-repo"

# Prefer the standalone binary; fall back to 'git filter-repo'
FILTER_REPO="git-filter-repo"
command -v git-filter-repo &>/dev/null || FILTER_REPO="git filter-repo"

info "git             : $(git --version)"
info "git-filter-repo : $($FILTER_REPO --version 2>/dev/null || echo 'installed')"
info "Source          : $SOURCE"
info "Output dir      : $OUTPUT_DIR"
[[ -n "$ONLY" ]]         && info "Only repo       : $ONLY"
[[ "$DRY_RUN" == true ]] && info "Mode            : DRY-RUN (no changes will be made)"

# ── Helpers ───────────────────────────────────────────────────────────────────

# clone_source <dest-dir>
#   Makes a full (non-shallow) clone suitable for git-filter-repo.
clone_source() {
    local dest="$1"
    info "Cloning $SOURCE → $dest"
    if [[ -d "$dest" ]]; then
        warn "$dest already exists, removing it first"
        run rm -rf "$dest"
    fi
    # git-filter-repo requires a non-bare clone that is not the original repo.
    # Use file:// when SOURCE is a local path to force a true copy (not hardlinks).
    local clone_src="$SOURCE"
    if [[ -d "$SOURCE" ]]; then
        clone_src="file://${SOURCE}"
    fi
    run git clone "$clone_src" "$dest"
    # Ensure full history (in case source was a shallow clone)
    run git -C "$dest" fetch --unshallow origin 2>/dev/null || true
}

# write_replacements_file <file> <neokapi|bowrain>
#   Writes the git-filter-repo replacement text file for the given target.
#   Format: literal:old==>literal:new  (one per line; more-specific first)
write_replacements_file() {
    local file="$1"
    local target="$2"   # "neokapi" or "bowrain"

    # NOTE: git-filter-repo strips 'literal:' / 'regex:' / 'glob:' from the
    # LEFT side only.  The RIGHT side is used verbatim — do NOT prefix it.
    cat > "$file" << 'REPLACEMENTS_EOF'
# ── Go module paths (more-specific sub-paths before the root module) ──────────
# These are rewritten first so that e.g. ".../bowrain-cli" is handled
# before the shorter ".../bowrain" prefix.
literal:github.com/gokapi/gokapi/bowrain-cli==>github.com/neokapi/bowrain/cli
literal:github.com/gokapi/gokapi/bowrain==>github.com/neokapi/bowrain
literal:github.com/gokapi/gokapi/platform==>github.com/neokapi/bowrain/platform
literal:github.com/gokapi/gokapi/kapi==>github.com/neokapi/neokapi/kapi
literal:github.com/gokapi/gokapi/cli==>github.com/neokapi/neokapi/cli
literal:github.com/gokapi/gokapi==>github.com/neokapi/neokapi
# ── GitHub org / repo references ──────────────────────────────────────────────
literal:https://github.com/gokapi/==>https://github.com/neokapi/
literal:git@github.com:gokapi/==>git@github.com:neokapi/
# ── Homebrew tap ──────────────────────────────────────────────────────────────
literal:gokapi/tap==>neokapi/tap
# ── Display name / identifier (cased variants — specific before general) ──────
literal:GOKAPI==>NEOKAPI
literal:Gokapi==>Neokapi
literal:gokapi==>neokapi
REPLACEMENTS_EOF
}

# filter_repo_paths <work-dir> <mode>
#   <mode> = "neokapi"  → removes the bowrain-owned paths, keeps everything else
#   <mode> = "bowrain"  → keeps only the bowrain-owned paths
filter_repo_paths() {
    local work="$1"
    local mode="$2"

    # Paths that belong exclusively to the bowrain platform repo
    local bowrain_paths=(
        "bowrain/"
        "bowrain-cli/"
        "platform/"
        "compose.yaml"
        "compose.override.yaml"
        "deploy/"
        "e2e/"
        "docker/"
    )

    local path_args=()
    for p in "${bowrain_paths[@]}"; do
        path_args+=( "--path" "$p" )
    done

    if [[ "$mode" == "neokapi" ]]; then
        # Keep everything EXCEPT the bowrain paths
        info "Filtering out bowrain-owned paths"
        run $FILTER_REPO --source "$work" --target "$work" \
            "${path_args[@]}" --invert-paths --force
    else
        # Keep ONLY the bowrain paths (plus LICENSE / README / CHANGELOG / Makefile)
        info "Keeping only bowrain-owned paths"
        run $FILTER_REPO --source "$work" --target "$work" \
            "${path_args[@]}" \
            --path "LICENSE" \
            --path "README.md" \
            --path "CHANGELOG.md" \
            --path "Makefile" \
            --force
    fi
}

# filter_repo_rename <work-dir> <replacements-file>
#   Rewrites file contents and renames files that contain "gokapi".
filter_repo_rename() {
    local work="$1"
    local replacements="$2"

    info "Rewriting file contents (text substitutions)"
    run $FILTER_REPO --source "$work" --target "$work" \
        --replace-text "$replacements" \
        --force

    info "Renaming files and directories containing 'gokapi'"
    # This Python callback renames any path component that contains "gokapi".
    run $FILTER_REPO --source "$work" --target "$work" \
        --filename-callback 'return filename.replace(b"gokapi", b"neokapi")' \
        --force
}

# write_neokapi_go_work <work-dir>
write_neokapi_go_work() {
    local work="$1"
    cat > "$work/go.work" << 'GO_WORK_EOF'
go 1.26.0

use (
	.
	./cli
	./kapi
)
GO_WORK_EOF
}

# write_bowrain_go_work <work-dir>
write_bowrain_go_work() {
    local work="$1"
    cat > "$work/go.work" << 'GO_WORK_EOF'
go 1.26.0

use (
	./bowrain
	./bowrain-cli
	./platform
)
GO_WORK_EOF
}

# For the bowrain repo the go.work lives at the root of the monorepo but
# after filtering the top-level files (Makefile, LICENSE…) are kept while
# the original go.work references modules that no longer exist in this repo.
# We therefore write a fresh go.work for the bowrain layout.
#
# The bowrain repo has no root-level go.mod; the three modules sit in
# sub-directories, so the go.work `use` directives reference sub-paths.

# write_bowrain_gitignore <work-dir>
write_bowrain_gitignore() {
    local work="$1"
    cat > "$work/.gitignore" << 'GITIGNORE_EOF'
# Build artifacts
bin/
/bowrain-server
/bowrain-worker
/actionlint
coverage/

# IDE
.idea/
.vscode/
*.swp
*.swo

# OS
.DS_Store
Thumbs.db

# Go
vendor/

# Frontend
node_modules/
dist/
*.tsbuildinfo
**/public/version.json

# Wails
**/build/bin/
**/build/darwin/
!bowrain/apps/bowrain/build/darwin/
**/build/appicon.png
!bowrain/apps/bowrain/build/appicon.png
**/frontend/wailsjs/
**/frontend/bindings/
!bowrain/apps/bowrain/frontend/bindings/
**/frontend/package.json.md5
**/.task/

# Keycloak theme build output
dist_keycloak/

# Dev server
.bowrain-server.pid
bowrain-dev.db*
bowrain-e2e.db*
bowrain-ci.db*

# Storybook
storybook-static/

# Playwright
test-results/
playwright-report/

# Local TLS certs (mkcert)
docker/traefik/certs/

# Generated documentation assets
bowrain/apps/bowrain/frontend/recordings-output/
bowrain/website/

# Remotion video pipeline
website/videos/output/
website/videos/public/raw/

.java-version
.claude/worktrees/
GITIGNORE_EOF
}

# write_neokapi_readme <work-dir>
# Writes a stub README indicating this was migrated from gokapi/gokapi.
write_neokapi_readme() {
    local work="$1"
    # Only write a stub if README.md doesn't already exist after filtering
    # (it always will since we keep it, but we patch the first heading/badges).
    # We do a minimal in-place sed rather than replacing the whole file.
    if [[ -f "$work/README.md" ]]; then
        # The rename pass already rewrote gokapi→neokapi inside README.md,
        # so we just need to confirm the content is sensible.
        info "README.md rewritten by rename pass (neokapi references updated)"
    fi
}

# ensure_git_identity <work-dir>
#   Configures a fallback git author/committer identity if the environment
#   doesn't have one, so that post-processing commits succeed.
ensure_git_identity() {
    local work="$1"
    local name email
    name="$(git -C "$work" config --get user.name 2>/dev/null || echo "")"
    email="$(git -C "$work" config --get user.email 2>/dev/null || echo "")"
    if [[ -z "$name" ]]; then
        git -C "$work" config user.name "neokapi-migration"
    fi
    if [[ -z "$email" ]]; then
        git -C "$work" config user.email "migration@neokapi"
    fi
}

# remove_cross_repo_replace_directives <work-dir>
#   In the bowrain repo, the go.mod files inherited replace directives that
#   pointed to the framework module at "../" (valid inside the monorepo).
#   After the split, those paths no longer exist — remove them.
#   Directives pointing to other modules within the bowrain repo are kept.
remove_cross_repo_replace_directives() {
    local work="$1"
    info "Removing cross-repo replace directives from bowrain go.mod files"
    # Modules that live OUTSIDE the bowrain repo (in neokapi/neokapi):
    #   github.com/neokapi/neokapi
    #   github.com/neokapi/neokapi/cli
    # These had replace => ../ and => ../cli which are meaningless post-split.
    python3 - "$work" << 'PYEOF'
import sys, os, re

work = sys.argv[1]
# Modules that live in the neokapi repo and are NOT local to bowrain:
cross_repo = {
    b'github.com/neokapi/neokapi',
    b'github.com/neokapi/neokapi/cli',
}

for root, dirs, files in os.walk(work):
    dirs[:] = [d for d in dirs if d != '.git']  # skip .git dirs
    if 'go.mod' in files:
        path = os.path.join(root, 'go.mod')
        with open(path, 'rb') as f:
            content = f.read()

        # Parse and rebuild, dropping cross-repo replace lines.
        lines = content.split(b'\n')
        new_lines = []
        in_replace_block = False
        skip_next_close = False
        replace_lines_removed = 0

        i = 0
        while i < len(lines):
            line = lines[i]
            stripped = line.strip()

            if stripped.startswith(b'replace ('):
                in_replace_block = True
                block_lines = [line]
                i += 1
                while i < len(lines):
                    inner = lines[i]
                    inner_stripped = inner.strip()
                    if inner_stripped == b')':
                        block_lines.append(inner)
                        i += 1
                        break
                    # Check if this line is a cross-repo replace
                    is_cross = any(inner_stripped.startswith(m) for m in cross_repo)
                    if not is_cross:
                        block_lines.append(inner)
                    else:
                        replace_lines_removed += 1
                    i += 1
                in_replace_block = False
                # If the block only has "replace (\n)" left, drop it entirely
                inner_kept = [l for l in block_lines[1:-1] if l.strip()]
                if inner_kept:
                    new_lines.extend(block_lines)
                # else: drop the whole empty replace block
            elif stripped.startswith(b'replace '):
                # Single-line replace directive
                is_cross = any(stripped[8:].lstrip().startswith(m) for m in cross_repo)
                if not is_cross:
                    new_lines.append(line)
                else:
                    replace_lines_removed += 1
                i += 1
            else:
                new_lines.append(line)
                i += 1

        if replace_lines_removed > 0:
            new_content = b'\n'.join(new_lines)
            with open(path, 'wb') as f:
                f.write(new_content)
            print(f"  cleaned {os.path.relpath(path, work)}: removed {replace_lines_removed} cross-repo replace directive(s)")
PYEOF
}

# commit_post_processing <work-dir> <message>
commit_post_processing() {
    local work="$1"
    local msg="$2"
    ensure_git_identity "$work"
    run git -C "$work" add -A
    run git -C "$work" diff --cached --quiet || \
        run git -C "$work" commit -m "$msg"
}

# ── Build neokapi repo ─────────────────────────────────────────────────────────

build_neokapi() {
    step "Building neokapi repo"
    local dest="$OUTPUT_DIR/neokapi"
    local work="$OUTPUT_DIR/.work-neokapi"

    clone_source "$work"
    filter_repo_paths "$work" "neokapi"

    local repl
    repl="$(mktemp)"
    write_replacements_file "$repl" "neokapi"
    filter_repo_rename "$work" "$repl"
    rm -f "$repl"

    # ── Post-processing ──────────────────────────────────────────────────────
    info "Writing go.work (neokapi layout: . ./cli ./kapi)"
    if [[ "$DRY_RUN" != true ]]; then
        write_neokapi_go_work "$work"
        commit_post_processing "$work" \
            "chore: update go.work for neokapi mono-module layout"
    fi

    # ── Move to final destination ────────────────────────────────────────────
    if [[ "$NO_CLEANUP" == false ]]; then
        run mv "$work" "$dest"
    else
        info "Leaving work dir at $work (--no-cleanup)"
        dest="$work"
    fi

    info "neokapi repo ready at $dest"
    info "Next steps:"
    info "  cd $dest"
    info "  go work sync && go mod tidy  # refresh checksums"
    info "  git remote set-url origin https://github.com/neokapi/neokapi.git"
    info "  git push --mirror origin"
}

# ── Build bowrain repo ─────────────────────────────────────────────────────────

build_bowrain() {
    step "Building bowrain repo"
    local dest="$OUTPUT_DIR/bowrain"
    local work="$OUTPUT_DIR/.work-bowrain"

    clone_source "$work"
    filter_repo_paths "$work" "bowrain"

    local repl
    repl="$(mktemp)"
    write_replacements_file "$repl" "bowrain"
    filter_repo_rename "$work" "$repl"
    rm -f "$repl"

    # ── Post-processing ──────────────────────────────────────────────────────
    info "Writing go.work (bowrain layout: ./bowrain ./bowrain-cli ./platform)"
    if [[ "$DRY_RUN" != true ]]; then
        write_bowrain_go_work "$work"
        write_bowrain_gitignore "$work"
        remove_cross_repo_replace_directives "$work"
        commit_post_processing "$work" \
            "chore: add go.work/.gitignore, remove cross-repo replace directives for bowrain standalone repo"
    fi

    # Always write a fresh bowrain README (the inherited one is neokapi-centric)
    if [[ "$DRY_RUN" != true ]]; then
        cat > "$work/README.md" << 'README_EOF'
# bowrain

[![CI](https://github.com/neokapi/bowrain/actions/workflows/ci.yml/badge.svg)](https://github.com/neokapi/bowrain/actions/workflows/ci.yml)

> **Experimental:** Bowrain is an ongoing experiment and should not be used in production.

The Bowrain localization platform: REST server, desktop app, project CLI, CMS connectors,
and authentication. Built on top of the [neokapi](https://github.com/neokapi/neokapi)
open-source localization framework.

## Install

Pre-built binaries are on the [Releases](https://github.com/neokapi/bowrain/releases) page.

## Repository Layout

Three Go modules coordinated by `go.work`:

| Module | Path | Description |
|--------|------|-------------|
| **bowrain** | `bowrain/` | Platform: server, desktop app, connectors, auth, SQLite/PostgreSQL storage |
| **bowrain-cli** | `bowrain-cli/` | Bowrain project CLI (`bowrain` binary) |
| **platform** | `platform/` | Shared platform types, auth types, connector interfaces, REST client |

## Dependency on neokapi

All three modules depend on
[github.com/neokapi/neokapi](https://github.com/neokapi/neokapi)
for core localization primitives (content model, format readers/writers, tools, pipelines).
The dependency is one-way: neokapi does not depend on bowrain.

## Development

```bash
# Build
go work sync
cd bowrain && go build ./...
cd bowrain-cli && go build ./...

# Test
go test ./bowrain/...
go test ./bowrain-cli/...
go test ./platform/...

# Run server
go run ./bowrain/cmd/bowrain-server
```
README_EOF
        commit_post_processing "$work" "docs: add bowrain README for standalone repo"
    fi

    # ── Move to final destination ────────────────────────────────────────────
    if [[ "$NO_CLEANUP" == false ]]; then
        run mv "$work" "$dest"
    else
        info "Leaving work dir at $work (--no-cleanup)"
        dest="$work"
    fi

    info "bowrain repo ready at $dest"
    info "Next steps:"
    info "  cd $dest"
    info "  # Update replace directives in go.mod files to point at"
    info "  # the published neokapi module (remove local replace once published)"
    info "  go work sync && go mod tidy  # refresh checksums"
    info "  git remote set-url origin https://github.com/neokapi/bowrain.git"
    info "  git push --mirror origin"
}

# ── Main ──────────────────────────────────────────────────────────────────────

step "Preparing output directory"
run mkdir -p "$OUTPUT_DIR"

case "$ONLY" in
    "")        build_neokapi; build_bowrain ;;
    neokapi)   build_neokapi ;;
    bowrain)   build_bowrain ;;
    *)         die "--only must be 'neokapi' or 'bowrain'" ;;
esac

step "Done"
echo ""
echo "  Output: $OUTPUT_DIR/"
[[ "$ONLY" != "bowrain" ]] && echo "    neokapi/  ← push to github.com/neokapi/neokapi"
[[ "$ONLY" != "neokapi" ]] && echo "    bowrain/  ← push to github.com/neokapi/bowrain"
echo ""
echo "  After pushing:"
echo "    1. Rename the gokapi GitHub org to neokapi"
echo "    2. In the bowrain repo, replace the local 'replace' directives in"
echo "       go.mod with real published versions once neokapi is on pkg.go.dev"
echo "    3. Regenerate protobuf files (make proto) in each repo"
echo "    4. Run 'go work sync && go mod tidy' in each repo"
echo ""
