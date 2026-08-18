#!/usr/bin/env bash
#
# Guard: no hand-rolled run walk. A Run sequence is projected through a
# declared RunSpec (packages/kapi-format/src/run-projection.ts), never through
# a loop that tests the discriminator itself.
#
# The loop this catches is the one that reads as "concatenate the text" and
# behaves as "silently delete every placeholder, every paired code, every
# plural":
#
#     for (const r of runs) if (typeof r.text === "string") out += r.text;
#
# It shipped three times: a review pane whose source read "Your credits reset
# on ." beside a target showing the variable; a document preview that drew a
# plural block as an empty line; a lab that measured segmentation over text no
# reader saw. Each was written by someone who only needed the text, and each
# silently dropped everything else — including, later, kinds of run that did not
# exist when the loop was written.
#
# A RunSpec cannot do that: its type is a mapped type over RUN_KINDS, so every
# kind must be answered, and a kind added to the model is a compile error in
# every projection until each has said what it does with it.
#
# What fails: a source file testing a run's discriminator outside the sanctioned
# modules —
#
#     typeof x.text === "string"      r.text !== undefined
#     "ph" in run                     run.pcOpen ? … : …
#
# What passes, deliberately:
#
#   1. PROJECTION_FILES — the modules that implement the contract, plus the two
#      structural bridges (coded text ↔ runs) that must map every kind
#      one-to-one by hand and are covered by round-trip tests.
#   2. Test files — a test may construct and inspect runs freely.
#
# Usage:
#     ./scripts/check-run-projection.sh              # scan tracked files
#     ./scripts/check-run-projection.sh --self-test  # prove the matcher both ways
#
# Wired into `make check-run-projection` (part of `make lint`), `make pre-push`,
# and the repo-guards job in .github/workflows/ci.yml.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"

# ── the matcher ──────────────────────────────────────────────────────────────

# A run discriminator being tested by hand. The keys are the model's (RFC 0001).
readonly RUN_KEYS='text|ph|pcOpen|pcClose|sub|plural|select'
readonly WALK_RE="(typeof [A-Za-z_.]+\.(${RUN_KEYS}) ===|[A-Za-z_.]+\.(${RUN_KEYS}) !== undefined|\"(${RUN_KEYS})\" in [A-Za-z_]+)"

# Modules allowed to discriminate a run directly. Each is here because it does
# something a projection cannot express, and none of them can lose content
# silently — that is the bar for being added.
readonly PROJECTION_FILES=(
  # The contract itself, and the kit's discriminator helper it mirrors.
  packages/kapi-format/src/run-projection.ts
  packages/ui/src/components/preview/types.ts

  # Structural bridges: every kind maps one-to-one to a chip/span and back, and
  # the round trip is asserted by tests. The mapping is the product here, not a
  # reading of it.
  packages/ui/src/components/editor/runsCodedBridge.ts
  packages/ui/src/components/editor/codedText.ts
  packages/kapi-format/src/runs-validate.ts
  packages/kapi-format/src/icu-parse.ts
  packages/kapi-format/src/target-plural.ts

  # Canonicalisation for the KBF content hash. Maps every kind one-to-one and
  # *throws* on a discriminator it does not know — a run cannot pass through it
  # silently, which is the same guarantee by a stricter route.
  packages/kapi-format/src/kbf.ts

  # Anchor resolution: navigates a path to one run and asks whether it is the
  # kind the anchor claims. It projects nothing; a mismatch is returned as
  # `path-wrong-kind`, never skipped.
  packages/kapi-format/src/annotation.ts

  # Construction, not projection — the extractor builds runs from JSX source.
  packages/i18n-react/src/extract/runs.ts
  # Likewise the parse direction back from a translator's textarea.
  packages/ui/src/components/plural/runs-text.ts

  # Anatomy views: one element per kind, exhaustively, ending in a visible "?"
  # for anything unrecognised. Showing the discriminators *is* the feature.
  packages/ui/src/components/preview/RunSequence.tsx
  packages/kapi-lab/src/KbfExplorer.tsx
  packages/kapi-lab/src/KbfConformance.tsx
  web/src/components/Lab/kbfAnatomyDoc.ts
)

# scan_files reads NUL-separated paths on stdin and prints "file:line:match"
# for every hand-rolled discriminator test. Returns 1 if it printed anything.
#
# A single `x.text !== undefined` is not evidence of a run walk — plenty of
# objects have a `text` field, and flagging them would train everyone to ignore
# this check. A file is judged to be discriminating a *run* when it tests a
# paired-code key (which nothing else has) or two different run keys.
scan_files() {
  local hits keys suspect
  hits=$(xargs -0 grep -HnoIE "$WALK_RE" -- 2>/dev/null || true)
  [ -z "$hits" ] && return 0

  keys=$(printf '%s\n' "$hits" |
    sed -E "s/^([^:]+):[0-9]+:.*(pcOpen|pcClose|plural|select|text|sub|ph).*/\\1	\\2/" |
    sort -u)
  suspect=$(printf '%s\n' "$keys" | awk -F'\t' '
    $2 == "pcOpen" || $2 == "pcClose" { walker[$1] = 1 }
    { seen[$1]++ }
    END { for (f in seen) if (walker[f] || seen[f] >= 2) print f }
  ' | sort -u)
  [ -z "$suspect" ] && return 0

  hits=$(printf '%s\n' "$hits" | grep -E "^($(printf '%s\n' "$suspect" | paste -sd'|' -)):")
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

  cat >"$tmp/walker.ts" <<'EOF'
function runsText(runs: Run[]): string {
  let out = "";
  for (const r of runs) if (typeof r.text === "string") out += r.text;
  return out;
}
function label(run: Run): string {
  if ("ph" in run) return "placeholder";
  return run.text !== undefined ? "text" : "?";
}
EOF

  cat >"$tmp/clean.ts" <<'EOF'
const READING: RunSpec<Run, string> = {
  text: (r) => r.text,
  ph: { dropped: "carried alongside as a chip" },
  pcOpen: { dropped: "carried alongside as a chip" },
  pcClose: { dropped: "carried alongside as a chip" },
  sub: { unsupported: "a subblock has no reading here" },
  plural: { expand: (r) => projectRuns(otherBranch(r.plural.forms), READING) },
  select: { expand: (r) => projectRuns(otherBranch(r.select.cases), READING) },
  fallback: (kind) => `{${kind}}`,
};
export const text = (runs: Run[]) => projectRunsText(runs, READING);
EOF

  local out n
  if out=$(printf '%s\0' "$tmp/walker.ts" | scan_files); then
    echo "✖ self-test: the matcher did NOT flag a planted hand-rolled walk"
    status=1
  else
    n=$(printf '%s\n' "$out" | grep -c . || true)
    if [ "$n" -ne 3 ]; then
      echo "✖ self-test: expected 3 hits in the planted file, got ${n}:"
      printf '%s\n' "$out"
      status=1
    else
      echo "✓ self-test: flags typeof-text, in-operator and !==-undefined walks (3 hits)"
    fi
  fi

  if out=$(printf '%s\0' "$tmp/clean.ts" | scan_files); then
    echo "✓ self-test: a declared RunSpec passes"
  else
    echo "✖ self-test: the matcher flagged a declared RunSpec:"
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
  echo "✖ check-run-projection.sh: self-test failed — the matcher is broken."
  self_test || true
  exit 1
fi

exclude_re="$(printf '%s\n' "${PROJECTION_FILES[@]}" | paste -sd'|' -)"

if hits=$(git ls-files -z -- '*.ts' '*.tsx' |
  tr '\0' '\n' |
  grep -vxE "$exclude_re" |
  grep -vE '(\.test\.tsx?|\.stories\.tsx?|/__tests__/|/e2e/)' |
  tr '\n' '\0' |
  scan_files); then
  echo "✓ every run projection is declared"
  exit 0
fi

echo "✖ hand-rolled run walk(s) found:"
printf '%s\n' "$hits"
echo ""
echo "A Run sequence is projected through a declared RunSpec, not a loop that"
echo "tests the discriminator — a loop answers for the kinds its author"
echo "remembered and silently drops the rest, which is how placeholders and"
echo "plurals have gone missing from three surfaces already."
echo ""
echo "Declare what this projection does with every kind of run:"
echo ""
echo "    import { projectRunsText, otherBranch } from \"@neokapi/kapi-format\";"
echo ""
echo "    const READING: RunSpec<Run, string> = {"
echo "      text: (r) => r.text,"
echo "      ph: { dropped: \"why it contributes nothing here\" },"
echo "      pcOpen: { dropped: \"…\" },  pcClose: { dropped: \"…\" },"
echo "      sub: { unsupported: \"why it must not occur here\" },"
echo "      plural: { expand: (r) => projectRuns(otherBranch(r.plural.forms), READING) },"
echo "      select: { expand: (r) => projectRuns(otherBranch(r.select.cases), READING) },"
echo "      fallback: (kind) => \`{\${kind}}\`,"
echo "    };"
echo ""
echo "See packages/kapi-format/src/run-projection.ts. If this file genuinely"
echo "maps every kind by hand (a structural bridge, with a round-trip test),"
echo "add it to PROJECTION_FILES in scripts/check-run-projection.sh."
exit 1
