#!/usr/bin/env bash
#
# PostToolUse hook for Write and Edit: format the file that was just written.
#
# The formatters are the ones CI holds the tree to (`gofmt -s` through
# scripts/check-gofmt.sh, `vp fmt` through make check-fmt-fixed-point), so a
# file formatted here does not cost a CI round trip later. The hook never
# fails the edit: a formatter that is missing, or that refuses the file, leaves
# it as written for the guards to report.
#
# Only files of this repository are formatted: the project directory and its
# worktrees, which share one git directory. A formatter reads configuration
# from the tree it runs in, and `vp fmt` loads that configuration as code, so
# a file edited in some other checkout is left alone.
set -uo pipefail

file="$(jq -r '.tool_response.filePath // .tool_input.file_path // empty' 2>/dev/null)"
[ -n "$file" ] && [ -f "$file" ] || exit 0

# git_dir prints the git directory a checkout shares with its worktrees.
git_dir() {
  git -C "$1" rev-parse --path-format=absolute --git-common-dir 2>/dev/null
}

project="${CLAUDE_PROJECT_DIR:-}"
[ -n "$project" ] || exit 0
ours="$(git_dir "$project")"
theirs="$(git_dir "$(dirname "$file")")"
[ -n "$ours" ] && [ "$ours" = "$theirs" ] || exit 0

case "$file" in
  *.go)
    command -v gofmt >/dev/null 2>&1 && gofmt -s -w "$file" 2>/dev/null
    ;;
  *.ts | *.tsx | *.mts | *.cts | *.js | *.jsx | *.mjs | *.cjs | *.css | *.json)
    # Run from the checkout the file is in. A session started in the primary
    # checkout may be editing a worktree under .claude/worktrees/, and each has
    # its own node_modules and formatter configuration. vp fmt skips a file
    # its configuration ignores, such as generated bindings.
    root="$(git -C "$(dirname "$file")" rev-parse --show-toplevel 2>/dev/null)" || exit 0
    command -v vp >/dev/null 2>&1 && (cd "$root" && vp fmt "$file" >/dev/null 2>&1)
    ;;
esac
exit 0
