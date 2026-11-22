#!/usr/bin/env bash
#
# Guard: comments describe current behavior and constraints.
#
# The matcher flags explicit change narration: first-person change reports,
# PR numbers used as timestamps, and rename notes. It does not broadly flag
# "previously", "no longer" or "used to be": these can describe runtime state,
# such as a decision whose basis no longer matches its source.
#
# Preserve root-cause and constraint explanations when editing comments.
# For example, a containment check needs to explain that an absolute source
# path could otherwise expose arbitrary readable files. Lead with the current
# invariant and retain the failure condition it prevents.
#
# Scope: tracked comment lines in Go, TypeScript, Makefiles, shell, Python and
# the workflow and action YAML, excluding generated code and this file, whose
# own documentation quotes the markers.
# Usage:
#     ./scripts/check-comment-history.sh
#     ./scripts/check-comment-history.sh --self-test
#
# Runs through make check-comment-history, make lint and make pre-push.
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
#   as of #123 / since #123 / until #123 / from #123 until / as of PR 123
#                     A PR number used as a timestamp. Name the behaviour; git
#                     blame holds the PR. A bare reference ("see #123", "the
#                     #804 link") names a record and passes.
#   renamed from      The old name is not a property of the current code.
#   used to be called Same.
#   formerly          Always introduces a superseded name or state.
readonly NARRATION_RE='\bwe (now|used to|previously|no longer)\b|\b(as of|since|until) (pr[[:space:]#]*[0-9]+|#[0-9]+)|\bfrom (pr[[:space:]#]*[0-9]+|#[0-9]+) until\b|\brenamed from\b|\bused to be called\b|\bformerly\b'

# Only comment lines. A string literal that happens to contain one of these
# phrases is content — test fixtures quote user-facing prose, and a seeded
# definition reading "A word we no longer write." is data, not a comment.
# Go and TypeScript comment with // and /* */; the rest of the scope with #.
readonly COMMENT_PREFIX_RE='^[[:space:]]*(//|\*|/\*)'
readonly HASH_COMMENT_PREFIX_RE='^[[:space:]]*#'

readonly SCAN_SURFACES=(
  core host cli kapi memory terms providers packages apps plugins bowrain
  scripts .github Makefile
)

# list_files prints NUL-separated candidate paths. A surface is a directory or
# a single file.
#
# git ls-files so the scan sees exactly what the repo owns: node_modules, build
# output and vendored trees are ignored and therefore skipped. An absolute path
# is the self-test's temp directory, outside any work tree, so it walks instead.
list_files() {
  local p
  for p in "$@"; do
    [ -e "$p" ] || continue
    case "$p" in
      /*) find "$p" -type f \( -name '*.go' -o -name '*.ts' -o -name '*.tsx' \
            -o -name Makefile -o -name '*.mk' -o -name '*.sh' -o -name '*.py' \
            -o -name '*.yml' -o -name '*.yaml' \) -print0 ;;
      *)
        if [ -f "$p" ]; then
          git ls-files -z --cached --others --exclude-standard -- "$p"
          continue
        fi
        # A pathspec `*` matches across `/`, so "$p/*.go" reaches every depth.
        # "$p/**/*.go" would need a directory between, and skip the files at
        # the top of the surface.
        git ls-files -z --cached --others --exclude-standard \
          -- "$p/*.go" "$p/*.ts" "$p/*.tsx" \
          "$p/*Makefile" "$p/*.mk" "$p/*.sh" "$p/*.py" \
          "$p/*.yml" "$p/*.yaml" ;;
    esac
  done
}

# scan_paths prints "file:line:match" for every comment line under $@ that
# carries a narration marker. Returns 1 if it printed anything.
scan_paths() {
  local hits
  hits="$(list_files "$@" | RE="$NARRATION_RE" PREFIX="$COMMENT_PREFIX_RE" HASH="$HASH_COMMENT_PREFIX_RE" perl -0 -ne '
    BEGIN {
      $re     = qr/$ENV{RE}/i;
      $cstyle = qr/$ENV{PREFIX}/;
      $hash   = qr/$ENV{HASH}/;
    }
    chomp;
    my $f = $_;
    next unless -f $f;
    next if $f =~ m{(^|/)scripts/check-comment-history\.sh$};
    my $prefix = $f =~ /\.(go|ts|tsx)$/ ? $cstyle : $hash;
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

  # A tracked surface, read through git as the real run reads it: a file at
  # the top of the surface, a file a level down, and the #-comment files.
  mkdir -p "$tmp/repo/surface/sub"
  printf '// The reader has accepted it since #1599.\npackage x\n' >"$tmp/repo/surface/top.go"
  printf '// The writer rejects it until #12 lands.\npackage x\n' >"$tmp/repo/surface/sub/deep.go"
  printf 'all:\n\t# Two tests failed on main from #3668 until this job existed.\n\ttrue\n' >"$tmp/repo/surface/Makefile"
  printf '#!/bin/sh\n# we no longer run it here\nurl="since #1"\n' >"$tmp/repo/surface/run.sh"
  printf 'on: push\n# as of #77 this step is gone\njobs: {}\n' >"$tmp/repo/surface/ci.yml"
  printf '# See #804 for the link.\nx = "we no longer"\n' >"$tmp/repo/surface/ref.py"
  out="$(cd "$tmp/repo" && git init -q . && git add -A && scan_paths surface || true)"
  local want got
  want="$(printf '%s\n' surface/Makefile surface/ci.yml surface/run.sh surface/sub/deep.go surface/top.go)"
  got="$(printf '%s\n' "$out" | cut -d: -f1 | LC_ALL=C sort -u)"
  if [ "$got" = "$want" ]; then
    echo "✓ self-test: a git surface is read at every depth and in #-comment files"
  else
    echo "✖ self-test: expected hits in exactly these files:"
    printf '%s\n' "$want"
    echo "  got:"
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
  as of / since #123     → name the behaviour; git blame holds the PR.
  renamed from           → the old name is not a property of the current code.
  formerly               → likewise for a superseded name or state.

Rewrite to state the invariant. If the comment explains WHY the code is shaped
as it is — a past failure that a branch, a guard or a regression test exists to
prevent — keep that cause and lead with the invariant. Never delete load-bearing
rationale just to satisfy this grep.

MSG
exit 1
