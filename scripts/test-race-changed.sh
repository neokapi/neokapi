#!/usr/bin/env bash
# test-race-changed.sh — race-test only the Go packages a PR touches.
#
# The PR CI profile deliberately drops -race (see the Makefile's event-aware
# GOTEST profile): the full race gate runs on push/nightly. But main is
# unprotected, so a race introduced in a PR can merge green. This script is the
# cheap middle ground: it computes the Go packages changed between BASE_SHA and
# HEAD_SHA and runs `go test -race -short -count=1` over just those, keeping
# the job well under the PR wall's budget.
#
# Scope guard: if the changed set is large (> RACE_CHANGED_CAP packages) or a
# workspace-global file changed (go.work, root go.mod/go.sum, Makefile), the
# run falls back to a fixed core set — ./core/flow ./core/tool for the
# framework plus one root package per changed module — so the job stays fast
# instead of silently becoming a full -race wall. Completeness is not the goal
# here; the push/nightly gate still races everything.
#
# Covered modules: the ones the PR test wall itself runs on plain ubuntu.
# Excluded: apps/kapi-desktop and bowrain/apps/bowrain (need the GTK/WebKit
# toolchain), plugins/sat (native ONNX stack), scripts/* tooling modules, and
# the e2e suites (build binaries; nightly concern).
#
# Env: BASE_SHA (default origin/main), HEAD_SHA (default HEAD),
#      RACE_CHANGED_CAP (default 30), GOTAGS (default fts5).
set -euo pipefail

BASE_SHA="${BASE_SHA:-origin/main}"
HEAD_SHA="${HEAD_SHA:-HEAD}"
CAP="${RACE_CHANGED_CAP:-30}"
GOTAGS="${GOTAGS:-fts5}"

# Modules covered, longest path first so file→module assignment can take the
# first prefix match. Keep in sync with go.work (minus the exclusions above).
MODULES=(
  bowrain/plugin/schema
  bowrain/plugin
  bowrain/core
  bowrain/cli
  bowrain
  host
  cli
  kapi
  .
)

# Fallback root package(s) per module, used when the changed set blows the cap.
fallback_pkgs() {
  case "$1" in
    .) echo "./core/flow ./core/tool" ;;
    host) echo "./" ;;
    cli) echo "./" ;;
    kapi) echo "./preset" ;;
    bowrain) echo "./service" ;;
    bowrain/core) echo "./project" ;;
    bowrain/cli) echo "./cmd/kapi-bowrain" ;;
    bowrain/plugin) echo "./" ;;
    bowrain/plugin/schema) echo "./" ;;
  esac
}

module_of() {
  # Prints the covered module a repo-relative file belongs to, or nothing.
  local f="$1" m
  for m in "${MODULES[@]}"; do
    if [ "$m" = "." ]; then
      # Framework module: any core/sievepen/termbase/providers/bench path not
      # claimed by a longer module prefix above.
      case "$f" in
        core/*|sievepen/*|termbase/*|providers/*|bench/*) echo "."; return ;;
      esac
    elif [[ "$f" == "$m"/* ]]; then
      echo "$m"
      return
    fi
  done
}

changed=$(git diff --name-only "$BASE_SHA"..."$HEAD_SHA" -- '*.go' ':(exclude)**/testdata/**')
if [ -z "$changed" ]; then
  echo "race-changed: no Go files changed — nothing to do"
  exit 0
fi

# Workspace-global changes make "changed packages" meaningless — fall back.
global_changed=$(git diff --name-only "$BASE_SHA"..."$HEAD_SHA" -- go.work 'go.work.sum' go.mod go.sum Makefile)

# pkgs_<module> accumulation via two parallel arrays (bash 3.2 compatible).
mod_names=()
mod_pkgs=()
add_pkg() {
  local mod="$1" pkg="$2" i
  for i in "${!mod_names[@]}"; do
    if [ "${mod_names[$i]}" = "$mod" ]; then
      case " ${mod_pkgs[$i]} " in
        *" $pkg "*) return ;;
      esac
      mod_pkgs[i]="${mod_pkgs[$i]} $pkg"
      return
    fi
  done
  mod_names+=("$mod")
  mod_pkgs+=("$pkg")
}

total=0
while IFS= read -r f; do
  [ -n "$f" ] || continue
  mod=$(module_of "$f")
  [ -n "$mod" ] || continue
  dir=$(dirname "$f")
  # Skip e2e suites (they build binaries; covered nightly).
  case "$dir" in */e2e|*/e2e/*) continue ;; esac
  # Skip deleted/emptied package dirs.
  if ! ls "$dir"/*.go >/dev/null 2>&1; then continue; fi
  if [ "$mod" = "." ]; then
    rel="$dir"
  else
    rel="${dir#"$mod"/}"
    [ "$rel" = "$dir" ] && rel="." # file at the module root
  fi
  before_names=${#mod_names[@]} before_pkgs="${mod_pkgs[*]-}"
  add_pkg "$mod" "./$rel"
  if [ "${#mod_names[@]}" -ne "$before_names" ] || [ "${mod_pkgs[*]-}" != "$before_pkgs" ]; then
    total=$((total + 1))
  fi
done <<<"$changed"

if [ "$total" -eq 0 ]; then
  echo "race-changed: no changed packages in covered modules — nothing to do"
  exit 0
fi

if [ -n "$global_changed" ] || [ "$total" -gt "$CAP" ]; then
  echo "race-changed: capping (${total} changed packages, cap ${CAP}; global changes: ${global_changed:-none})"
  echo "race-changed: falling back to fixed core set per changed module"
  for i in "${!mod_names[@]}"; do
    mod_pkgs[i]=$(fallback_pkgs "${mod_names[$i]}")
  done
  # A global change without any framework package in the diff still races the
  # framework core set — the workspace files affect every module.
  if [ -n "$global_changed" ]; then
    add_pkg "." "$(fallback_pkgs .)" || true
  fi
fi

status=0
for i in "${!mod_names[@]}"; do
  mod="${mod_names[$i]}"
  # shellcheck disable=SC2086 # package lists are intentionally word-split
  set -- ${mod_pkgs[$i]}
  echo "race-changed: (cd $mod && go test -tags $GOTAGS -race -short -count=1 -timeout 480s $*)"
  if ! (cd "$mod" && go test -tags "$GOTAGS" -race -short -count=1 -timeout 480s "$@"); then
    status=1
  fi
done
exit $status
