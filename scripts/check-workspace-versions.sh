#!/usr/bin/env bash
#
# Guard: a module's go.mod and the workspace agree on what it compiles against.
#
# go.work unifies version selection across every module it lists, so a bump in
# one module's go.mod raises that dependency for all of them. The other
# modules' go.mod files stay as they were, which makes them misleading: the
# framework may declare etree v1.6.0 and build against v1.8.0 under the
# workspace, while a GOWORK=off build (module isolation, downstream consumers,
# `go install`) still uses v1.6.0. The two builds then parse XML with different
# libraries and nothing in the diff says so.
#
# For each workspace module this script diffs the versions `go list -m all`
# resolves under the workspace against the ones it resolves with GOWORK=off,
# and prints every dependency whose version differs, with both versions.
#
# What fails:
#     a DIRECT dependency of the module (a require without `// indirect`)
#     resolves to a different version under the workspace. The module's own
#     go.mod is wrong about what its code imports; raise the require (and tidy)
#     so the isolated build and the workspace build agree.
#
# What only reports (a warning, exit 0):
#     an INDIRECT dependency differs. The module does not import it; the
#     difference is a transitive choice, and the isolated tidy run that
#     audit-modules already performs keeps the go.sum honest for it.
#
# Modules the workspace replaces (the sibling modules of this repository) and
# the workspace members themselves are not compared: they carry no version.
#
# Only the modules with an isolated build are compared: the scripts/* members
# resolve this repository's modules through go.work alone (no replace), so
# they have no GOWORK=off answer to compare against.
#
# Usage:
#     ./scripts/check-workspace-versions.sh [dir ...]   # default: the isolated modules
#     ./scripts/check-workspace-versions.sh --self-test # prove the comparison both ways
#
# Wired into `make audit-modules` and `make check-module-boundaries` (the
# module-boundaries job in .github/workflows/ci.yml).
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
GO="${GO:-go}"

# ── the listings ─────────────────────────────────────────────────────────────

# list_format prints one "path version direct|indirect" line per dependency
# with a version of its own, skipping workspace members (.Main) and replaced
# modules (.Replace), which have none to compare.
readonly LIST_FORMAT='{{if and (not .Main) (not .Replace) .Version}}{{.Path}} {{.Version}} {{if .Indirect}}indirect{{else}}direct{{end}}{{end}}'

# workspace_versions prints the versions the workspace resolves. They are the
# same from every member directory, so this runs once, from the repo root.
workspace_versions() {
  (cd "$root" && "$GO" list -m -f "$LIST_FORMAT" all)
}

# module_versions prints the versions one module resolves on its own.
module_versions() {
  (cd "$root/$1" && GOWORK=off "$GO" list -m -f "$LIST_FORMAT" all)
}

# ── the comparison ───────────────────────────────────────────────────────────

# compare_listings reads the workspace listing from $1 and a module's own
# listing from $2 and prints one line per differing version:
#     DIRECT    <path>  go.mod <v>  workspace <v>
#     indirect  <path>  go.mod <v>  workspace <v>
# Returns 1 when a DIRECT line was printed, 0 otherwise (clean, or indirect
# differences only).
compare_listings() {
  local out
  out=$(awk '
    NR == FNR { ws[$1] = $2; next }
    ($1 in ws) && ws[$1] != $2 {
      kind = ($3 == "direct") ? "DIRECT  " : "indirect"
      printf "  %s  %s  go.mod %s  workspace %s\n", kind, $1, $2, ws[$1]
    }' "$1" "$2" | sort -k1,1r -k2,2)
  [ -z "$out" ] && return 0
  printf '%s\n' "$out"
  if printf '%s\n' "$out" | grep -q '^  DIRECT'; then
    return 1
  fi
  return 0
}

# ── the scan ─────────────────────────────────────────────────────────────────

# The modules audit-modules builds in isolation (AUDIT_MODULES in the
# Makefile, which passes the same list explicitly).
readonly DEFAULT_MODULES=(. host cli bowrain/core kapi apps/kapi-desktop bowrain/plugin bowrain)

scan() {
  local tmp status=0 dir
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  workspace_versions >"$tmp/workspace.txt"

  for dir in "$@"; do
    echo ">> workspace versions: $dir"
    if ! module_versions "$dir" >"$tmp/module.txt"; then
      echo "ERROR: $dir: GOWORK=off go list -m all failed (see above); the module does not resolve on its own"
      status=1
      continue
    fi
    if ! compare_listings "$tmp/workspace.txt" "$tmp/module.txt"; then
      echo "ERROR: $dir: a direct dependency resolves to a different version under go.work than under its own go.mod"
      echo "  (raise the require in $dir/go.mod to the workspace version and run make ci-tidy; the isolated build must compile against what the workspace builds against)"
      status=1
    fi
  done

  if [ "$status" -eq 0 ]; then
    echo "check-workspace-versions: every direct dependency resolves to the same version under go.work and GOWORK=off (indirect differences, if any, are listed above as warnings)"
  fi
  return "$status"
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  cat >"$tmp/workspace.txt" <<'EOF'
github.com/beevik/etree v1.8.0 direct
golang.org/x/net v0.59.0 indirect
golang.org/x/text v0.42.0 direct
github.com/google/uuid v1.6.0 direct
EOF

  # A direct dependency behind the workspace: must fail.
  cat >"$tmp/direct.txt" <<'EOF'
github.com/beevik/etree v1.6.0 direct
golang.org/x/net v0.58.0 indirect
github.com/google/uuid v1.6.0 direct
EOF
  local out
  if out=$(compare_listings "$tmp/workspace.txt" "$tmp/direct.txt"); then
    echo "✖ self-test: a direct dependency behind the workspace did NOT fail"
    status=1
  elif [ "$(printf '%s\n' "$out" | grep -c .)" -ne 2 ] ||
    ! printf '%s\n' "$out" | grep -q '^  DIRECT    github.com/beevik/etree  go.mod v1.6.0  workspace v1.8.0$' ||
    ! printf '%s\n' "$out" | grep -q '^  indirect  golang.org/x/net  go.mod v0.58.0  workspace v0.59.0$'; then
    echo "✖ self-test: unexpected report for a direct drift:"
    printf '%s\n' "$out"
    status=1
  else
    echo "✓ self-test: a direct dependency behind the workspace fails, with both versions"
  fi

  # Only an indirect dependency differs: reported, exit 0.
  cat >"$tmp/indirect.txt" <<'EOF'
github.com/beevik/etree v1.8.0 direct
golang.org/x/net v0.58.0 indirect
EOF
  if ! out=$(compare_listings "$tmp/workspace.txt" "$tmp/indirect.txt"); then
    echo "✖ self-test: an indirect-only difference failed instead of warning"
    status=1
  elif [ "$out" != "  indirect  golang.org/x/net  go.mod v0.58.0  workspace v0.59.0" ]; then
    echo "✖ self-test: unexpected report for an indirect drift:"
    printf '%s\n' "$out"
    status=1
  else
    echo "✓ self-test: an indirect difference is reported and does not fail"
  fi

  # Same versions, and a dependency the workspace does not know: clean.
  cat >"$tmp/clean.txt" <<'EOF'
github.com/beevik/etree v1.8.0 direct
golang.org/x/text v0.42.0 direct
example.com/only-here v0.1.0 direct
EOF
  if ! out=$(compare_listings "$tmp/workspace.txt" "$tmp/clean.txt"); then
    echo "✖ self-test: matching versions failed"
    status=1
  elif [ -n "$out" ]; then
    echo "✖ self-test: matching versions printed a report:"
    printf '%s\n' "$out"
    status=1
  else
    echo "✓ self-test: matching versions print nothing and pass"
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

case "${1:-}" in
  --self-test)
    self_test
    ;;
  --help | -h)
    awk 'NR > 1 && /^set -euo/ { exit } NR > 1 { sub(/^# ?/, ""); print }' "$0"
    ;;
  *)
    if [ "$#" -eq 0 ]; then
      scan "${DEFAULT_MODULES[@]}"
    else
      scan "$@"
    fi
    ;;
esac
