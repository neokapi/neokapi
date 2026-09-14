#!/usr/bin/env bash
#
# Guard: every tracked .go file must be gofmt-clean, with -s simplifications.
#
# Why this guard exists
#
#   `make fmt` (and `make -C bowrain fmt`) run `gofmt -w -s .` — they are
#   FIXERS, not CHECKERS. They rewrite the working tree and exit 0. Nothing in
#   this repository ever ran `gofmt -l`, so format drift could not fail
#   anything: not a PR, not `make check`, not `make pre-push`.
#
#   golangci-lint does not cover the gap. Under the v2 config in .golangci.yml
#   the `formatters:` block declares only exclusions — no formatter is enabled —
#   so `make lint` and the CI Lint jobs say nothing about formatting.
#
#   The result was silent accumulation. Nine files across the bowrain module
#   (bowrain/billing/, bowrain/connector/, bowrain/core/event/, bowrain/server/,
#   bowrain/store/) were gofmt-dirty on main as of 4c13e26af. They were then
#   cleared by ACCIDENT: the unrelated .klf→.kbf rename sweep (#1444) ran a
#   tree-wide `gofmt -w -s` and incidentally reformatted them. No one fixed
#   them, and nothing would have stopped them re-accumulating.
#
#   Two structural reasons the bowrain module drifted hardest:
#
#     1. `make check-bowrain` → `make -C bowrain check` → `fmt vet lint`, whose
#        `fmt` step SILENTLY REWRITES your files and passes. If you have already
#        committed, the fix lands as an unstaged edit that never reaches the
#        commit — the developer sees a green check and a dirty tree.
#     2. scripts/pre-push-check.sh gated that bowrain check on `^bowrain/core/`,
#        `^bowrain/plugin/` and `^bowrain/go.(mod|sum)$` only — never on the main
#        bowrain module. Eight of the nine drifted files live in exactly the
#        directories no local gate covered.
#
#   So this guard is deliberately a CHECK that fails, ungated by path: format
#   drift can be introduced by any commit touching any Go file, and the cost of
#   looking is ~1s for the whole tree.
#
# What it checks
#
#   Tracked .go files only (`git ls-files`), so vendored trees, build output and
#   untracked scratch files are out of scope by construction — the same
#   tracked-files convention scripts/check-abs-paths.sh uses.
#
#   `-s` is included because `make fmt` includes it. Dropping it here would let
#   `make fmt` produce a tree this guard considers clean but a later `make fmt`
#   would change again, which is the drift this exists to prevent. `-s`
#   simplifications (composite literals, slice expressions, range clauses) are
#   semantically neutral rewrites, not whitespace.
#
#   There is no allowlist. As of this writing all 2733 tracked .go files are
#   clean, including generated protobuf output, so there is nothing to carve
#   out. Keep it that way: fix the file rather than exempting it.
#
# Remediation when this fails:
#     make fmt          # gofmt -w -s over the tree, then commit the result
#
# Usage:
#     ./scripts/check-gofmt.sh              # check tracked .go files
#     ./scripts/check-gofmt.sh --self-test  # prove it both ways

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

GOFMT="${GOFMT:-gofmt}"

if ! command -v "$GOFMT" >/dev/null 2>&1; then
  echo "error: $GOFMT not found on PATH — install Go or set GOFMT=" >&2
  exit 1
fi

check_tree() {
  cd "$ROOT_DIR"

  local list count offenders offender_count
  # The file list stays NUL-delimited from `git ls-files -z` through `xargs -0`,
  # so a path containing a space would still be passed as one argument. (`xargs`
  # without -0 splits on whitespace, which would silently truncate such a path
  # into two nonexistent ones.) -0 is in both BSD and GNU xargs.
  list="$(mktemp)"
  # shellcheck disable=SC2064  # expand $list now, not at trap time
  trap "rm -f '$list'" RETURN
  git ls-files -z '*.go' >"$list"
  count="$(tr -cd '\0' <"$list" | wc -c | tr -d '[:space:]')"
  if [[ "$count" -eq 0 ]]; then
    echo "error: no tracked .go files found — is this the repository root?" >&2
    return 1
  fi

  # xargs batches argv so the 2700+ file list cannot overflow ARG_MAX.
  # gofmt exits 2 (and xargs 123) when a file does not parse; distinguish that
  # from "found unformatted files" so the guard explains itself instead of
  # aborting on an unexplained status.
  local rc=0
  offenders="$(xargs -0 "$GOFMT" -l -s <"$list")" || rc=$?
  if [[ "$rc" -ne 0 ]]; then
    echo >&2
    echo "error: $GOFMT could not process every file (exit $rc)." >&2
    echo "The parse errors above are the cause — a file that does not parse" >&2
    echo "cannot be format-checked. Fix the syntax, then re-run." >&2
    return 1
  fi
  offender_count="$(printf '%s' "$offenders" | grep -c . || true)"

  if [[ "$offender_count" -gt 0 ]]; then
    echo "error: $offender_count of $count tracked .go files are not gofmt-clean:" >&2
    echo >&2
    printf '%s\n' "$offenders" | sed 's/^/  /' >&2
    echo >&2
    echo "Fix and commit the result:" >&2
    echo "  make fmt" >&2
    echo >&2
    echo "Note: 'make fmt' rewrites files and exits 0 — it is a fixer, not a" >&2
    echo "check. Re-run this guard afterwards to confirm, and stage the changes." >&2

    # Before sending anyone to `make fmt`, warn if that would silently rewrite
    # doc-comment PROSE rather than just layout. Go's doc-comment canonicalizer
    # reads '' and `` as TeX-style quotes and converts them to ” and “, which
    # is how #1444's tree-wide sweep turned "SetupTrial inserted ''" (an empty
    # SQL string literal) into "SetupTrial inserted ”" — valid Go, meaningless
    # prose, no failure anywhere. Reword instead of letting gofmt decide.
    # Read the offender list line-by-line and quote each path, rather than
    # word-splitting it through xargs (gofmt -l emits newline-separated paths).
    local tex_quotes f
    tex_quotes=""
    while IFS= read -r f; do
      [[ -n "$f" ]] || continue
      tex_quotes+="$(grep -HnE "^[[:space:]]*//.*(''|\`\`)" "$f" 2>/dev/null || true)"$'\n'
    done <<<"$offenders"
    # Drop the blank lines left by files that matched nothing.
    tex_quotes="$(printf '%s' "$tex_quotes" | grep . || true)"
    if [[ -n "$tex_quotes" ]]; then
      echo >&2
      echo "WARNING: these comment lines contain '' or \`\` — gofmt will rewrite" >&2
      echo "them into curly quotes (” and “) and destroy the prose meaning:" >&2
      echo >&2
      printf '%s\n' "$tex_quotes" | sed 's/^/  /' >&2
      echo >&2
      echo "Reword them (e.g. \"an empty string\") instead of running make fmt." >&2
    fi
    return 1
  fi

  echo "gofmt: all $count tracked .go files are clean (gofmt -l -s)"
}

self_test() {
  local tmp rc=0 clean dirty
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  clean="$tmp/clean.go"
  dirty="$tmp/dirty.go"

  # Canonical gofmt output.
  cat >"$clean" <<'EOF'
package p

func f() []string {
	return []string{"a"}
}
EOF

  # Two distinct defects: leading space indentation (plain gofmt) and a
  # redundant type in a composite literal (only -s catches this one), so the
  # self-test proves BOTH halves of `gofmt -l -s` are in force.
  cat >"$dirty" <<'EOF'
package p

type pair struct{ a int }

func f() []pair {
  return []pair{pair{1}}
}
EOF

  echo "self-test: canonical gofmt output must pass"
  if [[ -n "$("$GOFMT" -l -s "$clean")" ]]; then
    echo "  FAIL: flagged a clean file" >&2
    rc=1
  else
    echo "  ok"
  fi

  echo "self-test: misindented + unsimplified source must fail"
  if [[ -z "$("$GOFMT" -l -s "$dirty")" ]]; then
    echo "  FAIL: accepted a dirty file" >&2
    rc=1
  else
    echo "  ok"
  fi

  echo "self-test: -s simplification alone must be enough to fail"
  # Same file with correct tab indentation: only the -s defect remains.
  cat >"$dirty" <<'EOF'
package p

type pair struct{ a int }

func f() []pair {
	return []pair{pair{1}}
}
EOF
  if [[ -z "$("$GOFMT" -l -s "$dirty")" ]]; then
    echo "  FAIL: -s simplification not enforced" >&2
    rc=1
  else
    echo "  ok"
  fi

  [[ "$rc" -eq 0 ]] && echo "self-test passed"
  return "$rc"
}

case "${1:-}" in
  --self-test) self_test ;;
  "") check_tree ;;
  *)
    echo "usage: $0 [--self-test]" >&2
    exit 2
    ;;
esac
