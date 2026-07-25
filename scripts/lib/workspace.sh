#!/usr/bin/env bash
#
# Shared resolution of the locations outside this repository. Source it, then
# call neokapi_init_workspace <repo-root>; it sets, without clobbering anything
# already in the environment:
#
#   NEOKAPI_WORKSPACE_DIR   the multi-repo workspace (this repo's siblings)
#   NEOKAPI_CHECKOUTS_DIR   where unrelated reference checkouts live
#   NEOKAPI_OKAPI_DIR       the upstream Okapi Framework (Java) clone
#   NEOKAPI_DOCLANG_DIR     the DocLang specification checkout
#
# The workspace is the parent of the MAIN checkout, not of the caller's tree:
# inside a linked git worktree (.claude/worktrees/<name>/) the parent is
# .claude/worktrees. git's common dir points at the main checkout's .git in
# both cases, so derive from that and fall back to <root>/.. outside git.
#
# Mirrors the defaults in the root Makefile — keep the two in step.
# See docs/internals/workspace-paths.md.

neokapi_init_workspace() {
  local root="${1:?neokapi_init_workspace: repo root required}"
  local common

  if [ -z "${NEOKAPI_WORKSPACE_DIR:-}" ]; then
    common="$(git -C "$root" rev-parse --path-format=absolute --git-common-dir 2>/dev/null || true)"
    if [ -n "$common" ] && [ -d "$common" ]; then
      NEOKAPI_WORKSPACE_DIR="$(cd "$common/../.." && pwd)"
    else
      NEOKAPI_WORKSPACE_DIR="$(cd "$root/.." && pwd)"
    fi
  fi

  : "${NEOKAPI_CHECKOUTS_DIR:=$(cd "$NEOKAPI_WORKSPACE_DIR/.." && pwd)}"
  : "${NEOKAPI_OKAPI_DIR:=$NEOKAPI_CHECKOUTS_DIR/okapi/Okapi}"
  : "${NEOKAPI_DOCLANG_DIR:=$NEOKAPI_CHECKOUTS_DIR/doclang-project/doclang}"

  export NEOKAPI_WORKSPACE_DIR NEOKAPI_CHECKOUTS_DIR NEOKAPI_OKAPI_DIR NEOKAPI_DOCLANG_DIR
}
