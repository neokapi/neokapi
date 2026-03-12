#!/usr/bin/env bash
#
# rename-gokapi-to-neokapi.sh
#
# Renames gokapi → neokapi across the entire repository with case-preserving
# substitution:
#   gokapi  → neokapi
#   Gokapi  → Neokapi
#   GOKAPI  → NEOKAPI
#
# New GitHub org/repo: github.com/neokapi/neokapi
#
# Usage:
#   ./scripts/rename-gokapi-to-neokapi.sh [--dry-run]
#
# With --dry-run it only prints what it would do, without making changes.
#
set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DRY_RUN=false

if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    echo "=== DRY RUN MODE — no changes will be made ==="
    echo
fi

# Directories/paths to skip (relative to repo root, used as grep -v patterns)
SKIP_PATTERNS=(
    "^\.git/"
    "^\.claude/"
    "^node_modules/"
    "^vendor/"
    "^\.gokapi\.db"
    "^\.neokapi\.db"
    "^coverage/"
    "^bin/"
    # Skip this script itself to avoid self-modification during execution
    "^scripts/rename-gokapi-to-neokapi\.sh"
)

# Binary file extensions to skip for content replacement (but still rename)
BINARY_EXTENSIONS="png|jpg|jpeg|gif|ico|webp|woff|woff2|ttf|eot|db|sqlite|jar|zip|tar|gz|exe|dll|so|dylib|wasm|pdf"

# ── Helpers ──────────────────────────────────────────────────────────────────

log()  { echo "  $*"; }
info() { echo "▸ $*"; }
warn() { echo "⚠ $*" >&2; }

should_skip() {
    local path="$1"
    for pattern in "${SKIP_PATTERNS[@]}"; do
        if echo "$path" | grep -qE "$pattern"; then
            return 0
        fi
    done
    return 1
}

is_binary_file() {
    local path="$1"
    if echo "$path" | grep -qiE "\.(${BINARY_EXTENSIONS})$"; then
        return 0
    fi
    return 1
}

# Platform-compatible sed in-place
sed_inplace() {
    if [[ "$(uname)" == "Darwin" ]]; then
        sed -i '' "$@"
    else
        sed -i "$@"
    fi
}

# ── Phase 1: Content replacement in text files ──────────────────────────────

phase_content_replace() {
    info "Phase 1: Replacing file contents"
    echo

    cd "$REPO_ROOT"

    # Find all files containing gokapi/Gokapi/GOKAPI (case-insensitive)
    local files
    files=$(grep -rlI --include='*' -i 'gokapi' . \
        | sed 's|^\./||' \
        | sort)

    local count=0
    local skipped=0

    while IFS= read -r file; do
        [[ -z "$file" ]] && continue

        # Skip excluded paths
        if should_skip "$file"; then
            ((skipped++))
            continue
        fi

        # Skip binary files for content replacement
        if is_binary_file "$file"; then
            continue
        fi

        # Skip if file doesn't exist (symlink target missing, etc.)
        if [[ ! -f "$file" ]]; then
            continue
        fi

        if $DRY_RUN; then
            log "[content] $file"
        else
            # Order matters: do the most specific (module path) replacements first,
            # then general replacements. Using separate sed passes for clarity.

            # 1. GitHub module path: github.com/gokapi/gokapi → github.com/neokapi/neokapi
            sed_inplace 's|github\.com/gokapi/gokapi|github.com/neokapi/neokapi|g' "$file"

            # 2. GitHub org references: github.com/gokapi/ → github.com/neokapi/
            #    (catches any remaining like github.com/gokapi/other-repo)
            sed_inplace 's|github\.com/gokapi/|github.com/neokapi/|g' "$file"

            # 3. Container registry: ghcr.io/gokapi/ → ghcr.io/neokapi/
            sed_inplace 's|ghcr\.io/gokapi/|ghcr.io/neokapi/|g' "$file"

            # 4. Case-sensitive general replacements (order: UPPER, Title, lower)
            #    Using word-ish boundaries to be safe, but also catching compounds
            sed_inplace 's/GOKAPI/NEOKAPI/g' "$file"
            sed_inplace 's/Gokapi/Neokapi/g' "$file"
            sed_inplace 's/gokapi/neokapi/g' "$file"
        fi

        ((count++))
    done <<< "$files"

    echo
    info "Phase 1 complete: $count files updated, $skipped skipped"
    echo
}

# ── Phase 2: Rename files that have "gokapi" in their name ──────────────────

phase_rename_files() {
    info "Phase 2: Renaming files with 'gokapi' in their name"
    echo

    cd "$REPO_ROOT"

    # Find files (not dirs) with gokapi in the name, deepest first for safe renaming
    local files
    files=$(find . -name "*gokapi*" -not -path "./.git/*" -not -path "./.claude/*" \
        -not -path "*/node_modules/*" -type f \
        | sed 's|^\./||' \
        | sort -r)

    local count=0

    while IFS= read -r file; do
        [[ -z "$file" ]] && continue

        if should_skip "$file"; then
            continue
        fi

        local dir
        dir=$(dirname "$file")
        local base
        base=$(basename "$file")

        # Case-preserving rename in the filename
        local newbase
        newbase=$(echo "$base" | sed 's/GOKAPI/NEOKAPI/g; s/Gokapi/Neokapi/g; s/gokapi/neokapi/g')

        if [[ "$base" != "$newbase" ]]; then
            local newfile="$dir/$newbase"
            if $DRY_RUN; then
                log "[rename file] $file → $newfile"
            else
                mv "$file" "$newfile"
                log "[renamed] $file → $newfile"
            fi
            ((count++))
        fi
    done <<< "$files"

    echo
    info "Phase 2 complete: $count files renamed"
    echo
}

# ── Phase 3: Rename directories that have "gokapi" in their name ────────────

phase_rename_dirs() {
    info "Phase 3: Renaming directories with 'gokapi' in their name"
    echo

    cd "$REPO_ROOT"

    # Find directories with gokapi in name, deepest first for safe renaming
    local dirs
    dirs=$(find . -name "*gokapi*" -not -path "./.git/*" -not -path "./.claude/*" \
        -not -path "*/node_modules/*" -type d \
        | sed 's|^\./||' \
        | sort -r)

    local count=0

    while IFS= read -r dir; do
        [[ -z "$dir" ]] && continue

        if should_skip "$dir"; then
            continue
        fi

        local parent
        parent=$(dirname "$dir")
        local base
        base=$(basename "$dir")

        local newbase
        newbase=$(echo "$base" | sed 's/GOKAPI/NEOKAPI/g; s/Gokapi/Neokapi/g; s/gokapi/neokapi/g')

        if [[ "$base" != "$newbase" ]]; then
            local newdir="$parent/$newbase"
            if $DRY_RUN; then
                log "[rename dir] $dir → $newdir"
            else
                mv "$dir" "$newdir"
                log "[renamed] $dir → $newdir"
            fi
            ((count++))
        fi
    done <<< "$dirs"

    echo
    info "Phase 3 complete: $count directories renamed"
    echo
}

# ── Phase 4: Rename special root-level files ────────────────────────────────

phase_rename_dotfiles() {
    info "Phase 4: Renaming special dotfiles"
    echo

    cd "$REPO_ROOT"

    local count=0

    # .gokapi.db → .neokapi.db
    if [[ -f ".gokapi.db" ]]; then
        if $DRY_RUN; then
            log "[rename] .gokapi.db → .neokapi.db"
        else
            mv ".gokapi.db" ".neokapi.db"
            log "[renamed] .gokapi.db → .neokapi.db"
        fi
        ((count++))
    fi

    # .gitignore: update references
    if [[ -f ".gitignore" ]] && grep -q "gokapi" ".gitignore"; then
        if $DRY_RUN; then
            log "[content] .gitignore"
        else
            sed_inplace 's/GOKAPI/NEOKAPI/g; s/Gokapi/Neokapi/g; s/gokapi/neokapi/g' ".gitignore"
            log "[updated] .gitignore"
        fi
        ((count++))
    fi

    echo
    info "Phase 4 complete: $count items processed"
    echo
}

# ── Phase 5: Update go.work.sum and go.sum (these need special handling) ─────

phase_update_go_sums() {
    info "Phase 5: Updating go.sum / go.work.sum module paths"
    echo

    cd "$REPO_ROOT"

    local count=0

    # go.work.sum at root
    if [[ -f "go.work.sum" ]]; then
        if $DRY_RUN; then
            log "[content] go.work.sum"
        else
            sed_inplace 's|github\.com/gokapi/gokapi|github.com/neokapi/neokapi|g' "go.work.sum"
            log "[updated] go.work.sum"
        fi
        ((count++))
    fi

    # All go.sum files
    while IFS= read -r sumfile; do
        [[ -z "$sumfile" ]] && continue
        if should_skip "$sumfile"; then continue; fi

        if grep -q "github.com/gokapi" "$sumfile" 2>/dev/null; then
            if $DRY_RUN; then
                log "[content] $sumfile"
            else
                sed_inplace 's|github\.com/gokapi/gokapi|github.com/neokapi/neokapi|g' "$sumfile"
                log "[updated] $sumfile"
            fi
            ((count++))
        fi
    done < <(find . -name "go.sum" -not -path "./.git/*" -not -path "./.claude/*" \
        -not -path "*/node_modules/*" | sed 's|^\./||')

    echo
    info "Phase 5 complete: $count sum files updated"
    echo
}

# ── Phase 6: Summary & next steps ───────────────────────────────────────────

phase_summary() {
    info "Phase 6: Post-rename verification hints"
    echo
    echo "  After running this script, you should:"
    echo
    echo "  1. Verify Go modules resolve:"
    echo "     cd $REPO_ROOT && go work sync"
    echo
    echo "  2. Run tests:"
    echo "     make test"
    echo
    echo "  3. Regenerate protobuf files:"
    echo "     make proto"
    echo
    echo "  4. Check for any remaining references:"
    echo "     grep -rI --include='*' -i 'gokapi' . \\"
    echo "       --exclude-dir=.git --exclude-dir=.claude \\"
    echo "       --exclude-dir=node_modules --exclude='*.db' \\"
    echo "       --exclude='rename-gokapi-to-neokapi.sh'"
    echo
    echo "  5. Update your git remote:"
    echo "     git remote set-url origin git@github.com:neokapi/neokapi.git"
    echo
    echo "  6. Consider renaming the parent directory:"
    echo "     mv $REPO_ROOT $(dirname $REPO_ROOT)/neokapi"
    echo
}

# ── Main ─────────────────────────────────────────────────────────────────────

main() {
    echo "╔══════════════════════════════════════════════════════════════╗"
    echo "║  Rename: gokapi → neokapi (case-preserving)                ║"
    echo "║  Repo:   github.com/gokapi/gokapi → neokapi/neokapi       ║"
    echo "╚══════════════════════════════════════════════════════════════╝"
    echo
    info "Repository root: $REPO_ROOT"
    echo

    # Run phases in order: content first, then renames (so grep still finds files)
    phase_content_replace
    phase_rename_files
    phase_rename_dirs
    phase_rename_dotfiles
    phase_update_go_sums
    phase_summary

    if $DRY_RUN; then
        echo "=== DRY RUN COMPLETE — no changes were made ==="
    else
        echo "=== RENAME COMPLETE ==="
    fi
}

main "$@"
