#!/usr/bin/env bash
#
# Guard: Kapi Desktop's own surface names no interchange format. The app shows
# a project's content memory and terms; moving their contents in and out as
# files is the CLI's job (`kapi memory`, `kapi terms`).
#
# The buttons went in #2258. This keeps them from coming back the way they
# arrived: one adapter method, one header action, and the stores are dressed as
# file-exchange endpoints again.
#
# What fails: TMX, XLIFF, "termbase" or "translation memory" appearing anywhere
# in the desktop app's own source — a label, a dialog filter, a hint, a comment.
#
# What passes, deliberately:
#
#   1. Storybook. XLIFF and TMX are formats the engine reads and writes, and
#      the stories are where they are documented — a reference page for a
#      format is not the app offering to exchange one. Storybook is held to the
#      retired-vocabulary rule by scripts/check-vocabulary.sh, which sweeps
#      apps/kapi-desktop/frontend/src/stories already.
#   2. Test files — several exist precisely to assert the absence this guard
#      also asserts, and would otherwise flag themselves.
#   3. BOUNDARY_RE — spellings frozen by something outside this repo's control:
#      the persisted `"termbases"` view id, and file-extension globs.
#
# This is narrower than scripts/check-vocabulary.sh, which sweeps the retired
# vocabulary across every product surface but deliberately leaves TMX and TBX
# alone as external standards we do not own. Here the question is not whether
# the word is ours; it is whether this app offers the exchange.
#
# Usage:
#     ./scripts/check-desktop-interchange.sh              # scan tracked files
#     ./scripts/check-desktop-interchange.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-desktop-interchange` (part of `make lint`),
# `make pre-push`, and the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────

readonly INTERCHANGE_RE='tmx|xliff|termbase|translation memor'

# Deleted from a line before it is matched. These spellings are fixed by
# something this repo does not get to rename.
#
#   "termbases"  — a persisted view id (types/api.ts documents why it keeps its
#                  historical spelling); renaming it strands saved navigation.
#   *.tmx, .tmx  — a file-extension glob naming a format, not an offer to read
#                  one. `okf_*` are the engine's format ids for the same reason.
readonly BOUNDARY_RE='"termbases"|\*\.tmx|\.tmx\b|okf_[a-z0-9_]+'

# Paths outside the shipped surface: the Storybook design record (which
# documents the formats the engine supports, and which check-vocabulary.sh
# already sweeps) and the tests, some of which assert this same absence.
readonly OUT_OF_SCOPE_RE='(^apps/kapi-desktop/frontend/src/stories/|\.test\.tsx?$|/__tests__/)'

# Reads NUL-separated paths on stdin, prints file:line:match for every offending
# line, and returns 1 when it printed anything.
scan_files() {
  local hits
  hits=$(xargs -0 grep -HniIE "$INTERCHANGE_RE" -- 2>/dev/null || true)
  [ -z "$hits" ] && return 0

  # Drop the frozen spellings, then re-test what is left of each line.
  hits=$(printf '%s\n' "$hits" |
    sed -E "s/(${BOUNDARY_RE})//gI" |
    grep -iE "$INTERCHANGE_RE" || true)
  [ -z "$hits" ] && return 0

  printf '%s\n' "$hits"
  return 1
}

# ── self-test ────────────────────────────────────────────────────────────────

self_test() {
  local tmp status=0
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "rm -rf '$tmp'" EXIT

  cat >"$tmp/dirty.tsx" <<'EOF'
<Button onClick={handleImport}>Import TMX</Button>
<Button onClick={handleExport}>Export TMX</Button>
const hint = "Import an XLIFF file to get started.";
// the project termbase is opened here
<p>Your translation memory is up to date.</p>
EOF

  cat >"$tmp/clean.tsx" <<'EOF'
// The "termbases" view id keeps its historical spelling.
const view = "termbases";
/** Space-delimited glob list, e.g. "*.tmx" or "*.html *.htm". */
const formats = ["okf_xliff", "okf_po"];
<PageHeader title="Project Content Memory" />
<p>Terms are bound at the point the content sits.</p>
EOF

  local out n
  if out=$(printf '%s\0' "$tmp/dirty.tsx" | scan_files); then
    echo "✖ self-test: the matcher did NOT flag planted interchange chrome"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 5 ]; then
      echo "✖ self-test: expected 5 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags TMX, XLIFF, termbase and translation-memory copy (5 hits)"
    fi
  fi

  if out=$(printf '%s\0' "$tmp/clean.tsx" | scan_files); then
    echo "✓ self-test: the persisted view id and format globs pass"
  else
    echo "✖ self-test: the matcher flagged a frozen spelling:"
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

# A matcher that silently stops matching is worse than no check at all.
if ! self_test >/dev/null; then
  echo "✖ check-desktop-interchange.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

if hits=$(git ls-files -z -- 'apps/kapi-desktop/frontend/src' |
  tr '\0' '\n' |
  grep -vE "$OUT_OF_SCOPE_RE" |
  tr '\n' '\0' |
  scan_files); then
  echo "✓ the desktop surface names no interchange format"
  exit 0
fi

echo "✖ interchange vocabulary in the Kapi Desktop surface:"
printf '%s\n' "$hits"
echo ""
echo "The desktop shows a project's stores; it does not move their contents in"
echo "and out as files. That is 'kapi memory' and 'kapi terms', which keep the"
echo "locale and domain arguments the formats need — the desktop's own CSV"
echo "import shipped passing (\"\", \"\", \"\") for all three."
echo ""
echo "Reference material about a format belongs in Storybook, which documents"
echo "what the engine reads and writes and is out of this guard's scope."
exit 1
