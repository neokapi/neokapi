#!/usr/bin/env bash
#
# Guard: a convergence run commits derived artifacts only when what it wrote in
# them is sound.
#
# `kapi up` writes target-language artifacts — catalogs, narration sidecars,
# runtime dictionaries — out of the project store. This gate classifies
# everything the run left in the working tree, using git for all path matching:
#
#   backing  — under .kapi/: a decision shard, terms, a memory seed, the voice
#              profile, a profile. The context graph moved.
#   derived  — every committed artifact the pipeline owns (`make
#              l10n-owned-paths`): the target-language tier `kapi up` writes and
#              the build tier the extractors and compilers write. Both are
#              legitimate output of a convergence run; the byte gate covers only
#              the second, which is why this gate reads the union rather than
#              `make l10n-derived-paths`.
#   foreign  — everything else. A convergence run has no business writing it, so
#              its presence is a symptom of the run rather than content to
#              deliver, whatever the delivery step stages.
#
# It then reads the derived artifacts themselves
# (`scripts/check-derived-content.mjs`) and refuses — non-zero, naming every
# defect — a run whose output does not parse, dropped a placeholder its source
# carries, or translated a machine identifier the recipe does not declare
# translatable. Foreign changes are refused too, and separately: a convergence
# run does not author source.
#
# Content is the authority here because the file-shaped question is not one.
# Classifying which files moved cannot see a run that wrote the wrong content to
# the right files, and twice it did not: a return leg wrote translated scene ids
# into 16 narration sidecars, and 275 committed strings shipped with the hole
# where their placeholder had been. Both runs moved exactly the files a
# convergence run moves.
#
# Backing is reported rather than required. A night that converged and brought
# nothing home to `.kapi/` is the ordinary state of a repository whose source
# moves daily and whose reviewers approve in batches, and refusing it made the
# nightly red on every such night. What it is worth saying about backing is what
# it says: `--backing` compares each changed context file against HEAD after
# canonicalizing it, so a rewrite that re-sorted an array and decided nothing is
# reported as the normalization it is rather than as a decision.
#
# Usage:
#     ./scripts/check-sync-backed.sh                 # gate the working tree
#     ./scripts/check-sync-backed.sh --repo DIR      # gate another checkout
#     ./scripts/check-sync-backed.sh --derived 'a b' # override the derived set
#     ./scripts/check-sync-backed.sh --pairs 'L a:b' # override the content pairs
#     ./scripts/check-sync-backed.sh --self-test     # prove the gate both ways
#
# Wired into .github/workflows/dogfood-sync.yml between `kapi up` and the
# delivery step, and self-tested in the repo-guards job of ci.yml.
set -euo pipefail

# Under .kapi/ minus the derived projection of it. .kapi/work/ is gitignored, so
# the exclusion is belt and braces: a work tree that stopped being ignored must
# not read as the context graph moving.
readonly BACKING_SPECS=('.kapi' ':(exclude).kapi/work')

# The reader that answers what was written. Kept beside this script so a
# checkout that has one has the other.
CONTENT_READER="$(cd "$(dirname "$0")" && pwd)/check-derived-content.mjs"
readonly CONTENT_READER

usage() {
  sed -n '/^# Usage:/,/^#     .*--self-test/p' "$0" | sed 's/^# \{0,1\}//'
}

# porcelain prints one NUL-terminated "XY path" record per change, index and
# working tree together. --no-renames keeps every record to a single path, so a
# rename reads as a delete plus an add rather than a two-record entry; -uall
# lists untracked files individually, which is how a first narration sidecar or
# a first catalog for a new locale shows up at all.
porcelain() {
  git status --porcelain=v1 -z --untracked-files=all --no-renames -- "$@"
}

# in_list returns 0 when the first argument appears in the rest. Compared as
# whole strings rather than matched, so a path is never a prefix of another.
in_list() {
  local needle="$1" item
  shift
  for item in "$@"; do
    [ "$item" = "$needle" ] && return 0
  done
  return 1
}

# report prints "  XY path" for each entry it is given.
report() {
  local entry
  for entry in "$@"; do
    printf '  %s\n' "$entry"
  done
}

# summary appends a markdown section to the job summary when running in Actions.
summary() {
  [ -n "${GITHUB_STEP_SUMMARY:-}" ] || return 0
  printf '%s\n' "$@" >>"$GITHUB_STEP_SUMMARY"
}

# outputs publishes the classification as step outputs when running in Actions,
# so the step that delivers the run can report what the run produced rather than
# what a delivery is shaped like. Counts only: the paths are in the log and the
# job summary, and the pull request carries the diff itself.
outputs() {
  [ -n "${GITHUB_OUTPUT:-}" ] || return 0
  printf '%s\n' "$@" >>"$GITHUB_OUTPUT"
}

# content_defects prints the content reader's report for the given derived paths
# and returns its exit code. One invocation per locale line, each restricted to
# what this run wrote: the gate's question is about the run, not about the
# standing of the committed tier.
#
# A run that produced no derived change asks nothing. A checkout whose reader is
# missing is fatal rather than permissive, for the same reason an unreadable
# derived set is: a gate that cannot read what was written would pass everything
# while looking armed.
content_defects() {
  local pairs="$1"
  shift
  local paths=("$@")
  [ "${#paths[@]}" -gt 0 ] || return 0

  if [ ! -f "$CONTENT_READER" ]; then
    echo "check-sync-backed: cannot read derived content — ${CONTENT_READER} is missing" >&2
    return 2
  fi

  local only=()
  local p
  for p in "${paths[@]}"; do
    only+=(--only "$p")
  done

  local status=0 line args out rc
  while IFS= read -r line; do
    [ -n "${line//[[:space:]]/}" ] || continue
    set -f
    # shellcheck disable=SC2206  # deliberate word splitting, globbing disabled
    args=($line)
    set +f
    rc=0
    out="$(node "$CONTENT_READER" "${args[@]}" "${only[@]}" 2>&1)" || rc=$?
    if [ "$rc" -ne 0 ]; then
      printf '%s\n' "$out" >&2
      status="$rc"
    fi
  done <<<"$pairs"
  return "$status"
}

# ── the gate ─────────────────────────────────────────────────────────────────
#
# $1 is the repository to inspect; $2 the whitespace-separated derived pathspecs;
# $3 the content pairs, one locale per line. Returns 0 when the tree is
# committable, 1 when it refuses.
gate() {
  local repo="$1" derived_spec="$2" content_pairs="${3:-}"
  local rec path
  local all_entries=() all_paths=() derived_paths=() backing_paths=()
  local refused_foreign=() backing_entries=() derived_entries=()

  cd "$repo"

  # Pathspecs carry glob magic (the narration sidecars), so split them without
  # letting the shell expand any of it against the working tree.
  local derived_specs=()
  set -f
  # shellcheck disable=SC2206  # deliberate word splitting, globbing disabled
  derived_specs=($derived_spec)
  set +f

  # An empty set is refused rather than tolerated: `git status -- ` with no
  # pathspec after it lists the whole tree, which would classify every change as
  # derived and turn this gate into an approval.
  if [ "${#derived_specs[@]}" -eq 0 ]; then
    echo "check-sync-backed: the derived set is empty — refusing to classify anything as derived" >&2
    return 2
  fi

  while IFS= read -r -d '' rec; do
    all_entries+=("$rec")
    all_paths+=("${rec:3}")
  done < <(porcelain)

  while IFS= read -r -d '' rec; do
    derived_paths+=("${rec:3}")
  done < <(porcelain "${derived_specs[@]}")

  while IFS= read -r -d '' rec; do
    backing_paths+=("${rec:3}")
  done < <(porcelain "${BACKING_SPECS[@]}")

  local i=0
  while [ "$i" -lt "${#all_entries[@]}" ]; do
    rec="${all_entries[$i]}"
    path="${all_paths[$i]}"
    i=$((i + 1))
    if in_list "$path" ${backing_paths[@]+"${backing_paths[@]}"}; then
      backing_entries+=("$rec")
    elif in_list "$path" ${derived_paths[@]+"${derived_paths[@]}"}; then
      derived_entries+=("$rec")
    else
      refused_foreign+=("$rec")
    fi
  done

  # What the context change says, rather than that it happened. A file whose
  # canonical form is what HEAD already held decided nothing, whatever its bytes
  # did — the reordering that was reported as backing 48 derived files for three
  # nights running.
  local decisions=0 normalizations=0 kind
  if [ "${#backing_paths[@]}" -gt 0 ] && [ -f "$CONTENT_READER" ]; then
    while IFS=$'\t' read -r kind path; do
      [ -n "$kind" ] || continue
      if [ "$kind" = "normalization" ]; then
        normalizations=$((normalizations + 1))
      else
        decisions=$((decisions + 1))
      fi
    done < <(node "$CONTENT_READER" --backing "${backing_paths[@]}" 2>/dev/null || true)
  else
    decisions="${#backing_paths[@]}"
  fi

  # A run's own output is judged by what is in it. What a run *removed* has no
  # content to read, so the committed context stays the authority there: a
  # catalog or a sidecar that disappeared is an erasure unless something under
  # .kapi/ decided it, and a re-serialization decides nothing.
  local refused_deleted=()
  if [ "$decisions" -eq 0 ]; then
    local j=0
    while [ "$j" -lt "${#derived_entries[@]}" ]; do
      rec="${derived_entries[$j]}"
      j=$((j + 1))
      case "${rec:0:2}" in
        *D*) refused_deleted+=("$rec") ;;
      esac
    done
  fi

  local content_status=0
  local content_out=""
  if [ "${#derived_entries[@]}" -gt 0 ]; then
    local derived_only=()
    for path in "${all_paths[@]}"; do
      in_list "$path" ${derived_paths[@]+"${derived_paths[@]}"} && derived_only+=("$path")
    done
    content_out="$(content_defects "$content_pairs" ${derived_only[@]+"${derived_only[@]}"} 2>&1)" ||
      content_status=$?
    if [ "$content_status" -eq 2 ]; then
      printf '%s\n' "$content_out" >&2
      return 2
    fi
  fi

  if [ "$content_status" -eq 0 ] && [ "${#refused_foreign[@]}" -eq 0 ] &&
    [ "${#refused_deleted[@]}" -eq 0 ]; then
    outputs "derived=${#derived_entries[@]}" "backing=${#backing_entries[@]}" \
      "decisions=${decisions}"
    if [ "${#derived_entries[@]}" -eq 0 ] && [ "${#backing_entries[@]}" -eq 0 ]; then
      echo "check-sync-backed: the run left nothing to commit"
      summary "### Sync content gate" "" "The run left nothing to commit."
      return 0
    fi
    echo "check-sync-backed: ${#derived_entries[@]} derived change(s) carry sound content;" \
      "${decisions} context decision(s), ${normalizations} normalization(s)"
    if [ "${#backing_entries[@]}" -gt 0 ]; then
      echo "context (.kapi/):"
      report "${backing_entries[@]}"
    fi
    if [ "${#derived_entries[@]}" -gt 0 ]; then
      echo "derived:"
      report "${derived_entries[@]}"
    fi
    summary "### Sync content gate" "" \
      "${#derived_entries[@]} derived change(s) carry sound content. Context: \`${decisions}\` decision(s), \`${normalizations}\` normalization(s)."
    return 0
  fi

  echo "check-sync-backed: REFUSED — this run must not be committed" >&2

  if [ "$content_status" -ne 0 ]; then
    cat >&2 <<'EOF'

The run wrote artifacts the loop owns, and what it wrote in them is not sound.
Regeneration re-materializes these files rather than repairing them, so
committing them ships the defect and the next run reproduces it.
EOF
    printf '%s\n' "$content_out" >&2
    cat >&2 <<'EOF'
Nothing here is coverage: a string the target does not carry falls back to its
source, which is the pending state this loop absorbs. These are strings the
target does carry and gets wrong.
EOF
  fi

  if [ "${#refused_deleted[@]}" -gt 0 ]; then
    cat >&2 <<'EOF'

The run removed artifacts the loop owns while the committed context decided
nothing. A removal carries no content to read, so it is an erasure until
something under .kapi/ explains it — a decision shard under .kapi/state/, a
change to .kapi/terms.json, a memory seed, .kapi/voice.yaml, or a profile. A
context file that only re-serialized what was already there is not that.

Refused — removed, nothing behind it:
EOF
    report "${refused_deleted[@]}" >&2
  fi

  if [ "${#refused_foreign[@]}" -gt 0 ]; then
    cat >&2 <<'EOF'

The run also changed files outside both the context graph and the artifacts the
loop owns. A convergence run does not author these, so a run that wrote them is
not a run to deliver from:

Refused — outside the loop's scope:
EOF
    report "${refused_foreign[@]}" >&2
  fi

  local n=$((${#refused_foreign[@]} + ${#refused_deleted[@]}))
  echo "" >&2
  echo "check-sync-backed: refused ${n} file(s) the run may not deliver" >&2
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::error title=Sync refused::the run produced content that must not be committed — see the step log"
  fi
  summary "### Sync content gate — REFUSED" "" \
    "The run produced derived content that must not be committed." "" \
    '```' "$(printf '%s\n' "$content_out"
      report ${refused_deleted[@]+"${refused_deleted[@]}"} ${refused_foreign[@]+"${refused_foreign[@]}"})" '```'
  return 1
}

# resolve_derived prints every committed artifact the pipeline owns, read from
# the Makefile so this gate and the delivery step can never disagree about what
# a convergence run is allowed to have written. An empty or failed read is fatal
# rather than permissive: a gate that cannot name the derived set would pass
# everything while looking armed.
resolve_derived() {
  local repo="$1" spec
  if ! spec="$(make -C "$repo" -s l10n-owned-paths 2>/dev/null)" || [ -z "${spec//[[:space:]]/}" ]; then
    echo "check-sync-backed: cannot read the derived set from 'make l10n-owned-paths' in ${repo}" >&2
    echo "check-sync-backed: pass --derived '<pathspecs>' for a checkout without the Makefile" >&2
    return 1
  fi
  printf '%s\n' "$spec"
}

# resolve_pairs prints the artifact:reference pairs the content reader measures
# against, one locale per line, read from the same Makefile for the same reason:
# what a derived artifact derives from is recipe knowledge, and two lists of it
# would drift.
resolve_pairs() {
  local repo="$1" pairs
  if ! pairs="$(make -C "$repo" -s l10n-content-pairs 2>/dev/null)" || [ -z "${pairs//[[:space:]]/}" ]; then
    echo "check-sync-backed: cannot read the content pairs from 'make l10n-content-pairs' in ${repo}" >&2
    echo "check-sync-backed: pass --pairs '<lang> <artifact>:<reference>...' for a checkout without the Makefile" >&2
    return 1
  fi
  printf '%s\n' "$pairs"
}

# ── self-test ────────────────────────────────────────────────────────────────
#
# Every case runs the real gate against a real git repository, because what is
# being tested is a classification git performs: pathspec matching, untracked
# listing, and the ignore rules. A fixture that faked git output would prove
# nothing about any of them.

SELFTEST_STATUS=0

# The scratch repository's own shapes, so the content reader has real documents
# to measure against: an inventory whose `category` leaf is a machine identifier
# and whose `description` carries a placeholder, and a demo master with two
# narration scenes.
readonly SELFTEST_SOURCE='{"tools":{"qa":{"displayName":"Quality","category":"quality","description":"Checked {count} block(s)"}}}'
readonly SELFTEST_TARGET='{"tools":{"qa":{"displayName":"Kvalitet","category":"quality","description":"Kontrollerte {count} blokk(er)"}}}'
readonly SELFTEST_MASTER='id: demo-a
kind: use-case
narration:
  - id: discover
    kind: title
    text: A company repository.
  - id: correct
    kind: terminal
    text: Fix it where it is read.
'
readonly SELFTEST_SIDECAR='id: demo-a
kind: use-case
narration:
  - id: discover
    kind: title
    text: Et bedriftsrepository.
  - id: correct
    kind: terminal
    text: Rett det der det leses.
'

# planted_repo builds a scratch checkout shaped like this one — a committed
# context, one derived catalog beside the inventory it derives from, one demo
# whose narration sidecar does not exist yet, and one source file — and prints
# its path.
planted_repo() {
  local dir="$1"
  mkdir -p "$dir/.kapi/memory" "$dir/.kapi/state" "$dir/core/i18n/catalogs" \
    "$dir/core/i18n/builtins" "$dir/harness/demos/demo-a" "$dir/core/flow"
  printf 'work/\n' >"$dir/.kapi/.gitignore"
  printf '{"entries":[]}\n' >"$dir/.kapi/memory/docs-nb.memory.json"
  printf '{"concepts":[{"id":"a","terms":[{"locale":"en","text":"berth"},{"locale":"nb","text":"kaiplass"}]}]}\n' \
    >"$dir/.kapi/terms.json"
  printf '%s\n' "$SELFTEST_SOURCE" >"$dir/core/i18n/builtins/metadata.json"
  printf '%s\n' "$SELFTEST_TARGET" >"$dir/core/i18n/catalogs/nb.json"
  printf '%s' "$SELFTEST_MASTER" >"$dir/harness/demos/demo-a/demo.yaml"
  printf 'package flow\n' >"$dir/core/flow/executor.go"
  git -C "$dir" -c init.defaultBranch=main init -q
  git -C "$dir" -c user.email=gate@example.invalid -c user.name=gate add -A
  git -C "$dir" -c user.email=gate@example.invalid -c user.name=gate \
    -c commit.gpgsign=false commit -qm "planted"
}

# expect runs the gate on the scratch repo and checks the exit code, then that
# every named path appears in the output.
expect() {
  local label="$1" repo="$2" want="$3"
  shift 3
  local out rc=0 path
  out="$(gate "$repo" "$SELFTEST_DERIVED" "$SELFTEST_PAIRS" 2>&1)" || rc=$?
  if [ "$rc" -ne "$want" ]; then
    echo "✖ self-test: ${label} — expected exit ${want}, got ${rc}:"
    printf '%s\n' "$out" | sed 's/^/    /'
    SELFTEST_STATUS=1
    return
  fi
  for path in "$@"; do
    if ! printf '%s\n' "$out" | grep -qF -- "$path"; then
      echo "✖ self-test: ${label} — output does not name ${path}:"
      printf '%s\n' "$out" | sed 's/^/    /'
      SELFTEST_STATUS=1
      return
    fi
  done
  echo "✓ self-test: ${label}"
}

readonly SELFTEST_DERIVED='core/i18n/catalogs :(glob)harness/demos/*/demo.*.yaml'
readonly SELFTEST_PAIRS='nb core/i18n/catalogs/nb.json:core/i18n/builtins/metadata.json'

self_test() {
  local tmp start
  start="$PWD"
  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand $tmp now, not at trap time
  trap "cd '$start'; rm -rf '$tmp'" EXIT

  # The self-test runs the gate against scratch repositories inside a real job.
  # Its Actions side effects belong to those runs, not to the job hosting it, so
  # the step summary and the step outputs are redirected to the scratch dir —
  # the last case reads the redirected file to prove the counts are published.
  local outfile="$tmp/step-output"
  export GITHUB_STEP_SUMMARY="$tmp/step-summary"
  export GITHUB_OUTPUT="$outfile"

  local repo="$tmp/repo"
  mkdir -p "$repo"
  planted_repo "$repo"

  expect "a clean tree commits nothing" "$repo" 0

  # #1882: a night that converged and approved nothing is a night like any
  # other. What it wrote is what decides whether it may be committed.
  printf '%s\n' "${SELFTEST_TARGET/Kontrollerte/Sjekket}" \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "a catalog rewritten with no context change commits when its content is sound" \
    "$repo" 0 "core/i18n/catalogs/nb.json"

  # #2031: the residue class. The translation keeps its markers but loses the
  # parameter, so the reader gets the sentence with the count missing from it.
  printf '{"tools":{"qa":{"displayName":"Kvalitet","category":"quality","description":"Kontrollerte blokk(er)"}}}\n' \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "a translation that dropped its placeholder is refused" "$repo" 1 \
    "tools.qa.description" "missing {count}" "placeholder"

  # #1937 on the JSON path: a leaf the recipe's extraction rule does not select
  # is a machine identifier, and a group-by splits on the two spellings.
  printf '{"tools":{"qa":{"displayName":"Kvalitet","category":"kvalitet","description":"Kontrollerte {count} blokk(er)"}}}\n' \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "a translated machine identifier is refused" "$repo" 1 \
    "tools.qa.category" "identifier"

  printf 'not json\n' >"$repo/core/i18n/catalogs/nb.json"
  expect "a catalog that no longer parses is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "unparseable"

  # A context change beside it does not buy content past the gate.
  printf '{"entries":[{"t":"Hallo"}]}\n' >"$repo/.kapi/memory/docs-nb.memory.json"
  expect "backing does not excuse unsound content" "$repo" 1 "unparseable"

  git -C "$repo" checkout -q -- .
  printf '%s' "$SELFTEST_SIDECAR" >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a first narration sidecar commits when it overlays its master" "$repo" 0 \
    "harness/demos/demo-a/demo.nb.yaml"

  # #2032: the return leg translated the scene ids and kinds the harness matches
  # an overlay by, and every file-shaped gate agreed the run looked normal.
  printf '%s' "${SELFTEST_SIDECAR//id: discover/id: oppdag}" \
    >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a sidecar with a translated scene id is refused" "$repo" 1 \
    "harness/demos/demo-a/demo.nb.yaml" "oppdag" "identifier"

  printf '%s' "${SELFTEST_SIDECAR//kind: use-case/kind: brukstilfelle}" \
    >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a sidecar with a translated scene kind is refused" "$repo" 1 \
    "brukstilfelle" "identifier"

  printf 'id: demo-a\nkind: use-case\nnarration:\n  - id: discover\n    kind: title\n    text: Et bedriftsrepository.\n' \
    >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a sidecar that dropped a scene is refused" "$repo" 1 \
    "harness/demos/demo-a/demo.nb.yaml" "structure"

  git -C "$repo" checkout -q -- .
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"
  rm -f "$repo/core/i18n/catalogs/nb.json"
  expect "deleting a catalog with no context decision is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erasure"

  # #2018: the reordering that backed the loop's first delivered night. The
  # bytes moved and the file says exactly what it said before, so it explains
  # nothing that the run removed.
  printf '{"concepts":[{"id":"a","terms":[{"locale":"nb","text":"kaiplass"},{"locale":"en","text":"berth"}]}]}\n' \
    >"$repo/.kapi/terms.json"
  expect "a re-serialized context file does not explain a removal" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erasure"

  printf '{"concepts":[{"id":"a","terms":[{"locale":"en","text":"berth"},{"locale":"nb","text":"quay"}]}]}\n' \
    >"$repo/.kapi/terms.json"
  expect "a context decision explains the same removal" "$repo" 0 \
    "core/i18n/catalogs/nb.json" ".kapi/terms.json"

  printf 'package flow // edited\n' >"$repo/core/flow/executor.go"
  printf '{"concepts":[{"id":"b"}]}\n' >"$repo/.kapi/terms.json"
  expect "a source edit is refused even with the context moved" "$repo" 1 \
    "core/flow/executor.go" "outside the loop's scope"

  git -C "$repo" checkout -q -- .
  printf '{"concepts":[{"id":"b"}]}\n' >"$repo/.kapi/terms.json"
  expect "context moving on its own commits" "$repo" 0 ".kapi/terms.json"

  git -C "$repo" checkout -q -- .
  mkdir -p "$repo/.kapi/work"
  printf 'store\n' >"$repo/.kapi/work/store.db"
  expect "the gitignored work tree is invisible" "$repo" 0

  git -C "$repo" checkout -q -- .
  rm -rf "$repo/.kapi/work"

  printf '%s\n' "${SELFTEST_TARGET/Kontrollerte/Sjekket}" >"$repo/core/i18n/catalogs/nb.json"
  local out rc=0
  out="$(gate "$repo" '   ' "$SELFTEST_PAIRS" 2>&1)" || rc=$?
  if [ "$rc" -eq 2 ] && printf '%s\n' "$out" | grep -qF "derived set is empty"; then
    echo "✓ self-test: an empty derived set is refused, not read as approval"
  else
    echo "✖ self-test: an empty derived set did not stop the gate (exit ${rc}):"
    printf '%s\n' "$out" | sed 's/^/    /'
    SELFTEST_STATUS=1
  fi
  git -C "$repo" checkout -q -- .

  # What the delivery step tells a reviewer the run produced comes from here, so
  # a run that classified two derived changes must say so in its outputs and not
  # merely in its log — and it must say how much of the context change was a
  # decision rather than a re-serialization.
  : >"$outfile"
  printf '%s\n' "${SELFTEST_TARGET/Kontrollerte/Sjekket}" >"$repo/core/i18n/catalogs/nb.json"
  printf '%s' "$SELFTEST_SIDECAR" >"$repo/harness/demos/demo-a/demo.nb.yaml"
  printf '{"concepts":[{"id":"a","terms":[{"locale":"nb","text":"kaiplass"},{"locale":"en","text":"berth"}]}]}\n' \
    >"$repo/.kapi/terms.json"
  expect "two sound derived changes behind a re-serialized context commit" "$repo" 0
  if grep -qx 'derived=2' "$outfile" && grep -qx 'backing=1' "$outfile" &&
    grep -qx 'decisions=0' "$outfile"; then
    echo "✓ self-test: the counts are published as step outputs, normalization apart from decision"
  else
    echo "✖ self-test: the step outputs do not carry the counts:"
    sed 's/^/    /' "$outfile"
    SELFTEST_STATUS=1
  fi
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"
  git -C "$repo" checkout -q -- .

  # The gate arms itself in this repository: both lists come from the Makefile,
  # and a rename there must fail loudly rather than pass everything.
  cd "$start"
  local root spec
  root="$(cd "$(dirname "$0")/.." && pwd)"
  if spec="$(resolve_derived "$root")" && [ -n "${spec//[[:space:]]/}" ]; then
    echo "✓ self-test: the derived set resolves from the Makefile"
  else
    echo "✖ self-test: the derived set does not resolve in ${root}"
    SELFTEST_STATUS=1
  fi

  if spec="$(resolve_pairs "$root")" && [ -n "${spec//[[:space:]]/}" ]; then
    echo "✓ self-test: the content pairs resolve from the Makefile"
  else
    echo "✖ self-test: the content pairs do not resolve in ${root}"
    SELFTEST_STATUS=1
  fi

  if [ -f "$CONTENT_READER" ] && node "$CONTENT_READER" --self-test >/dev/null 2>&1; then
    echo "✓ self-test: the content reader proves its own cases"
  else
    echo "✖ self-test: the content reader does not pass its own cases"
    SELFTEST_STATUS=1
  fi

  if (resolve_derived "$tmp" >/dev/null 2>&1); then
    echo "✖ self-test: a checkout with no Makefile resolved a derived set anyway"
    SELFTEST_STATUS=1
  else
    echo "✓ self-test: a checkout that cannot name its derived set is fatal, not permissive"
  fi

  return "$SELFTEST_STATUS"
}

# ── entry point ──────────────────────────────────────────────────────────────

main() {
  local repo="" derived="" pairs="" mode="gate"

  while [ $# -gt 0 ]; do
    case "$1" in
      --repo)
        repo="${2:-}"
        shift 2
        ;;
      --derived)
        derived="${2:-}"
        shift 2
        ;;
      --pairs)
        pairs="${2:-}"
        shift 2
        ;;
      --self-test)
        mode="self-test"
        shift
        ;;
      -h | --help)
        usage
        return 0
        ;;
      *)
        echo "check-sync-backed: unknown argument: $1" >&2
        usage >&2
        return 2
        ;;
    esac
  done

  if [ "$mode" = "self-test" ]; then
    self_test
    return
  fi

  if [ -z "$repo" ]; then
    repo="$(git rev-parse --show-toplevel)"
  else
    repo="$(cd "$repo" && git rev-parse --show-toplevel)"
  fi

  if [ -z "$derived" ]; then
    derived="$(resolve_derived "$repo")" || return 2
  fi
  if [ -z "$pairs" ]; then
    pairs="$(resolve_pairs "$repo")" || return 2
  fi

  gate "$repo" "$derived" "$pairs"
}

main "$@"
