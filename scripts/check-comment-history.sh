#!/usr/bin/env bash
#
# Guard: a comment describes what the code IS, not what it was changed from.
#
# Change narration rots. "Previously this returned nil" is true on the day it is
# written and misleading a year later, when the reader has no way to tell whether
# the comment describes the code in front of them or a state three refactors ago.
# The code's history is in git, where it stays accurate for free.
#
# ── What this guard does NOT match, and why ──────────────────────────────────
#
# This is the narrow half of a rule whose wide half is a judgement call, and the
# split is deliberate.
#
# "previously", "no longer", "used to be" and "formerly" are NOT scanned as a
# class, because in this codebase they are overwhelmingly DOMAIN vocabulary
# rather than change narration. Staleness and drift are what the product is
# about, so a comment reading
#
#     a decision whose basis no longer matches its source is stale
#     the reservation records that `slug` was previously held by `workspaceID`
#     cachedDocument streams a previously-recorded document from disk
#
# describes RUNTIME STATE — data that changed, not code that changed — and is
# exactly right. A sweep in 2026-08 judged ~140 such sites one by one: roughly
# two thirds were domain usage or load-bearing root-cause documentation, and
# only a third were narration worth rewriting. A grep cannot tell those apart,
# and a guard that flagged all of them would need a ~95-entry allowlist — which
# is a tax on every future correct use, not a check.
#
# So the patterns below are only the markers that have no legitimate
# current-state reading at all: a first-person report of a change ("we now
# buffer"), a PR number offered as a timestamp ("as of #852"), and an explicit
# rename note. Those are always narration.
#
# ── The rule the guard cannot enforce ────────────────────────────────────────
#
# ROOT-CAUSE AND CONSTRAINT COMMENTS STAY, even when they describe a past
# failure. A comment explaining why a branch exists —
#
#     An absolute source_path used to be honoured outright, which let item
#     metadata name any readable file on the host
#
# — is the reason the containment check may not be simplified away, and deleting
# it to satisfy a grep loses the only record of why the code is shaped as it is.
# When rewriting narration, lead with the invariant and keep the cause; delete
# only when nothing about the present remains.
#
# Scope: tracked Go and TypeScript sources, comment lines only. Generated files
# (*.pb.go, Wails bindings) are excluded — their comments come from a generator.
#
# Usage:
#     ./scripts/check-comment-history.sh              # scan
#     ./scripts/check-comment-history.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-comment-history` (part of `make lint`) and `make
# pre-push`.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────
#
# Case-insensitive. Each alternative is change narration in every context:
#
#   we now / we used to / we previously / we no longer
#                     A first-person report that the code changed. The
#                     present-tense fact ("nothing is buffered") says the same
#                     thing and stays true.
#   as of #123 / as of PR 123
#                     A PR number used as a timestamp. Name the behaviour; git
#                     blame holds the PR.
#   renamed from      The old name is not a property of the current code.
#   used to be called Same.
#   formerly          Always introduces a superseded name or state.
readonly NARRATION_RE='\bwe (now|used to|previously|no longer)\b|\bas of (pr[[:space:]#]*[0-9]+|#[0-9]+)|\brenamed from\b|\bused to be called\b|\bformerly\b'

# Only comment lines. A string literal that happens to contain one of these
# phrases is content — test fixtures quote user-facing prose, and a seeded
# definition reading "A word we no longer write." is data, not a comment.
readonly COMMENT_PREFIX_RE='^[[:space:]]*(//|\*|/\*)'

readonly SCAN_SURFACES=(
  core host cli kapi memory terms providers packages apps plugins bowrain
)

# list_files prints NUL-separated candidate paths.
#
# git ls-files so the scan sees exactly what the repo owns: node_modules, build
# output and vendored trees are ignored and therefore skipped. An absolute path
# is the self-test's temp directory, outside any work tree, so it walks instead.
list_files() {
  local p
  for p in "$@"; do
    [ -e "$p" ] || continue
    case "$p" in
      /*) find "$p" -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \) -print0 ;;
      *) git ls-files -z --cached --others --exclude-standard \
           -- "$p/**/*.go" "$p/**/*.ts" "$p/**/*.tsx" ;;
    esac
  done
}

# scan_paths prints "file:line:match" for every comment line under $@ that
# carries a narration marker. Returns 1 if it printed anything.
scan_paths() {
  local hits
  hits="$(list_files "$@" | RE="$NARRATION_RE" PREFIX="$COMMENT_PREFIX_RE" perl -0 -ne '
    BEGIN {
      $re     = qr/$ENV{RE}/i;
      $prefix = qr/$ENV{PREFIX}/;
    }
    chomp;
    my $f = $_;
    next unless -f $f;
    # Generated sources: the comments come from a generator, not from an
    # author, and rewriting them would be undone on the next regeneration.
    next if $f =~ /\.pb\.go$|\/bindings\/|_templ\.go$|\.gen\.go$/;
    # Slurped, then split on newlines by hand: -0 sets $/ to NUL for the
    # file LIST on stdin, so a readline here would swallow the whole file.
    my $text = do { local $/; open(my $fh, "<", $f) or next; <$fh> };
    next unless defined $text;
    my @lines = split /\n/, $text, -1;
    for my $i (0 .. $#lines) {
      my $line = $lines[$i];
      next unless $line =~ $prefix;
      while ($line =~ /$re/g) { print "$f:", $i + 1, ":$&\n" }
    }
  ')"

  [ -z "$hits" ] && return 0
  printf '%s\n' "$hits"
  return 1
}

# ── self-test ────────────────────────────────────────────────────────────────
#
# A matcher that silently stops matching is worse than no check at all.
self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  mkdir -p "$tmp/surface"
  cat >"$tmp/surface/narrated.go" <<'EOF'
package x

// skelFlush is a no-op: we no longer buffer each write.
func skelFlush() {}

// The parity knowledge moved to spec.yaml as of #852.
// This field was renamed from its ts-prefixed spelling.
// The stamp (formerly the source-check tool) promotes a status.
// We now write the toggle form on every run.
func other() {}
EOF

  # The domain and root-cause uses this guard deliberately lets pass.
  cat >"$tmp/surface/clean.go" <<'EOF'
package x

// A decision whose basis no longer matches its source is stale.
// ReserveSlug records that `slug` was previously held by `workspaceID`.
// cachedDocument streams a previously-recorded document from disk.
//
// An absolute source_path used to be honoured outright, which let item
// metadata name any readable file on the host; the connector has no business
// reading outside the root it was pointed at.
func x() {}
EOF

  # A string literal is content, not a comment.
  cat >"$tmp/surface/data.ts" <<'EOF'
export const seed = { definition: "A word we no longer write." };
EOF

  local out n
  if out=$(scan_paths "$tmp/surface/narrated.go"); then
    echo "✖ self-test: the matcher did NOT flag planted narration"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "✖ self-test: expected 5 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags all five narration markers (5 hits)"
    fi
  fi

  if out=$(scan_paths "$tmp/surface/clean.go"); then
    echo "✓ self-test: domain usage and root-cause rationale pass"
  else
    echo "✖ self-test: the matcher flagged a comment it must leave alone:"
    printf '%s\n' "$out"
    status=1
  fi

  if out=$(scan_paths "$tmp/surface/data.ts"); then
    echo "✓ self-test: a string literal is not scanned"
  else
    echo "✖ self-test: the matcher flagged a string literal:"
    printf '%s\n' "$out"
    status=1
  fi

  return "$status"
}

# ── main ─────────────────────────────────────────────────────────────────────

cd "$root"

if [ "${1:-}" = "--self-test" ]; then
  self_test
  exit $?
fi

if ! self_test >/dev/null; then
  echo "✖ check-comment-history.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

if hits=$(scan_paths "${SCAN_SURFACES[@]}"); then
  echo "✓ no change narration in comments"
  exit 0
fi

echo "✖ change narration found in comments:"
printf '%s\n' "$hits"
cat <<'MSG'

A comment describes what the code IS. These markers describe what it was
changed from, which git already records — and which stops being true the next
time the code moves:

  we now / we used to    → state the behaviour in the present tense.
  as of #123             → name the behaviour; git blame holds the PR.
  renamed from           → the old name is not a property of the current code.
  formerly               → likewise for a superseded name or state.

Rewrite to state the invariant. If the comment explains WHY the code is shaped
as it is — a past failure that a branch, a guard or a regression test exists to
prevent — keep that cause and lead with the invariant. Never delete load-bearing
rationale just to satisfy this grep.

MSG
exit 1
