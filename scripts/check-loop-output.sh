#!/usr/bin/env bash
#
# Guard: a convergence run commits derived artifacts only when what it wrote in
# them is sound.
#
# `kapi up` writes target-language artifacts (catalogs, narration sidecars,
# runtime dictionaries) out of the project store. This gate classifies
# everything the run left in the working tree, using git for all path matching:
#
#   derived  — every committed artifact the pipeline owns (`make
#              l10n-owned-paths`): the target-language tier `kapi up` writes and
#              the build tier the extractors and compilers write. Both are
#              legitimate output of a convergence run; the byte gate covers only
#              the second, which is why this gate reads the union rather than
#              `make l10n-derived-paths`.
#   foreign  — every other change git reports. A convergence run has no business
#              writing it, so its presence is a symptom of the run rather than
#              content to deliver, whatever the delivery step stages.
#
# The project store under .kapi/ is gitignored as a whole, so nothing a run
# leaves there reaches either list. The context a run brought home travels on
# refs/kapi/context, and the workflow's `kapi context push` step reports it.
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
# `--hold-back` changes what a defect costs. Without it one bad leaf refuses the
# whole run, so a night of sound work is thrown away over a single string; with
# it the defective leaf is removed from the artifact and named, and the rest of
# the run is delivered. Every runtime that reads this tier falls back to the
# source string for a key it cannot find, so the withheld string reads as the
# pending work it is rather than as a hole in a sentence. The nightly passes it;
# a bare run is a reading and leaves the tree alone.
#
# What it never does is turn a defect into a pass. A leaf whose removal would
# empty its artifact, an artifact that does not parse, and every narration
# sidecar defect refuse the run exactly as before: a sidecar overlays its master
# scene for scene, so there is no leaf to take out of one.
#
# A removal carries no content to read, so what excuses one is the context: an
# owned artifact that disappeared is delivered only when the run brought
# decisions home. `--decisions N` passes that count, which the nightly takes
# from the operations its `kapi context push` step shared on refs/kapi/context.
# With the default of 0, a catalog or a sidecar the run removed is an erasure
# and refuses the run. A sidecar that became identical to its source is dropped
# by design, and it is dropped because a decision changed its wording.
#
# Usage:
#     ./scripts/check-loop-output.sh                 # gate the working tree
#     ./scripts/check-loop-output.sh --repo DIR      # gate another checkout
#     ./scripts/check-loop-output.sh --derived 'a b' # override the derived set
#     ./scripts/check-loop-output.sh --pairs 'L a:b' # override the content pairs
#     ./scripts/check-loop-output.sh --hold-back     # withhold defective leaves
#     ./scripts/check-loop-output.sh --decisions N   # decisions the run brought home
#     ./scripts/check-loop-output.sh --self-test     # prove the gate both ways
#
# Wired into .github/workflows/dogfood-sync.yml between `kapi up` and the
# delivery step, and self-tested in the repo-guards job of ci.yml.
set -euo pipefail

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
# Everything the reader said is printed, on a sound run as well as a defective
# one, because with hold-back on there is something to say either way: the
# records naming what left the artifacts come back the same way the defects do.
# The caller separates them.
#
# A run that produced no derived change asks nothing. A checkout whose reader is
# missing is fatal rather than permissive, for the same reason an unreadable
# derived set is: a gate that cannot read what was written would pass everything
# while looking armed.
content_defects() {
  local pairs="$1" hold="$2"
  shift 2
  local paths=("$@")
  [ "${#paths[@]}" -gt 0 ] || return 0

  if [ ! -f "$CONTENT_READER" ]; then
    echo "check-loop-output: cannot read derived content — ${CONTENT_READER} is missing" >&2
    return 2
  fi
  if ! command -v node >/dev/null 2>&1; then
    echo "check-loop-output: cannot read derived content — node is not on PATH" >&2
    return 2
  fi

  local only=()
  local p
  for p in "${paths[@]}"; do
    only+=(--only "$p")
  done

  local hold_args=()
  [ -n "$hold" ] && hold_args=(--hold-back)

  local status=0 line args out rc
  while IFS= read -r line; do
    [ -n "${line//[[:space:]]/}" ] || continue
    set -f
    # shellcheck disable=SC2206  # deliberate word splitting, globbing disabled
    args=($line)
    set +f
    rc=0
    out="$(node "$CONTENT_READER" "${args[@]}" ${hold_args[@]+"${hold_args[@]}"} "${only[@]}" 2>&1)" ||
      rc=$?
    printf '%s\n' "$out"
    if [ "$rc" -ne 0 ]; then
      status="$rc"
    fi
  done <<<"$pairs"
  return "$status"
}

# render_withheld turns the reader's withheld records into the lines a reviewer
# reads: which artifact, which leaf, what was wrong with it.
render_withheld() {
  local target key kind detail
  while IFS=$'\t' read -r _ target key kind detail; do
    [ -n "$target" ] || continue
    printf '  %s %s: %s (%s)\n' "$target" "$key" "$detail" "$kind"
  done <<<"$1"
}

# held_back_block is what the log and the job summary say about the leaves the
# run did not deliver.
held_back_block() {
  printf 'held back (%s): withheld from this run, so the surface falls back to\n' "$1"
  printf 'its source for these strings until the translation is sound.\n'
  render_withheld "$2"
}

# withheld_output publishes the withheld records as a multi-line step output, so
# the delivery step can name each one in the pull request it opens. A count
# alone would tell a reviewer that something is missing and not what.
withheld_output() {
  [ -n "${GITHUB_OUTPUT:-}" ] || return 0
  {
    echo 'withheld_detail<<CHECK_LOOP_OUTPUT_WITHHELD'
    if [ -n "$1" ]; then render_withheld "$1"; fi
    echo 'CHECK_LOOP_OUTPUT_WITHHELD'
  } >>"$GITHUB_OUTPUT"
}

# ── the gate ─────────────────────────────────────────────────────────────────
#
# $1 is the repository to inspect; $2 the whitespace-separated derived pathspecs;
# $3 the content pairs, one locale per line; $4 non-empty to hold defective
# leaves back; $5 the number of decisions the run brought home. Returns 0 when
# the tree is committable, 1 when it refuses.
gate() {
  local repo="$1" derived_spec="$2" content_pairs="${3:-}" hold="${4:-}"
  local decisions="${5:-0}"
  local rec path
  local all_entries=() all_paths=() derived_paths=()
  local refused_foreign=() derived_entries=()

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
    echo "check-loop-output: the derived set is empty — refusing to classify anything as derived" >&2
    return 2
  fi

  while IFS= read -r -d '' rec; do
    all_entries+=("$rec")
    all_paths+=("${rec:3}")
  done < <(porcelain)

  while IFS= read -r -d '' rec; do
    derived_paths+=("${rec:3}")
  done < <(porcelain "${derived_specs[@]}")

  local i=0
  while [ "$i" -lt "${#all_entries[@]}" ]; do
    rec="${all_entries[$i]}"
    path="${all_paths[$i]}"
    i=$((i + 1))
    if in_list "$path" ${derived_paths[@]+"${derived_paths[@]}"}; then
      derived_entries+=("$rec")
    else
      refused_foreign+=("$rec")
    fi
  done

  # A run's own output is judged by what is in it. What a run *removed* has no
  # content to read, so the decisions the run brought home are the authority
  # there: a catalog or a sidecar that disappeared in a run that decided
  # nothing is an erasure.
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
  local content_out="" withheld_out="" defect_out="" withheld_count=0
  if [ "${#derived_entries[@]}" -gt 0 ]; then
    local derived_only=()
    for path in "${all_paths[@]}"; do
      in_list "$path" ${derived_paths[@]+"${derived_paths[@]}"} && derived_only+=("$path")
    done
    content_out="$(content_defects "$content_pairs" "$hold" ${derived_only[@]+"${derived_only[@]}"} 2>&1)" ||
      content_status=$?
    if [ "$content_status" -eq 2 ]; then
      printf '%s\n' "$content_out" >&2
      return 2
    fi
    # The reader answers on one stream: a tab-separated record for every leaf it
    # withheld, and the report a human reads for everything else.
    withheld_out="$(printf '%s\n' "$content_out" | grep $'^withheld\t' || true)"
    defect_out="$(printf '%s\n' "$content_out" | grep -v $'^withheld\t' || true)"
    if [ -n "$withheld_out" ]; then
      withheld_count="$(printf '%s\n' "$withheld_out" | wc -l | tr -d ' ')"
    fi
  fi

  if [ "$content_status" -eq 0 ] && [ "${#refused_foreign[@]}" -eq 0 ] &&
    [ "${#refused_deleted[@]}" -eq 0 ]; then
    outputs "derived=${#derived_entries[@]}" "withheld=${withheld_count}"
    withheld_output "$withheld_out"
    if [ "${#derived_entries[@]}" -eq 0 ]; then
      echo "check-loop-output: the run left nothing to commit"
      summary "### Loop output gate" "" "The run left nothing to commit."
      return 0
    fi
    echo "check-loop-output: ${#derived_entries[@]} derived change(s) carry sound content;" \
      "${decisions} decision(s) brought home"
    if [ -n "$withheld_out" ]; then
      held_back_block "$withheld_count" "$withheld_out"
    fi
    echo "derived:"
    report "${derived_entries[@]}"
    summary "### Loop output gate" "" \
      "${#derived_entries[@]} derived change(s) carry sound content. Decisions brought home: \`${decisions}\`."
    if [ -n "$withheld_out" ]; then
      summary "" "\`${withheld_count}\` string(s) held back, so those surfaces fall back to their source:" \
        "" '```' "$(render_withheld "$withheld_out")" '```'
    fi
    return 0
  fi

  echo "check-loop-output: REFUSED — this run must not be committed" >&2

  if [ "$content_status" -ne 0 ]; then
    cat >&2 <<'EOF'

The run wrote artifacts the loop owns, and what it wrote in them is not sound.
Regeneration re-materializes these files rather than repairing them, so
committing them ships the defect and the next run reproduces it.
EOF
    printf '%s\n' "$defect_out" >&2
    cat >&2 <<'EOF'
Nothing here is coverage: a string the target does not carry falls back to its
source, which is the pending state this loop absorbs. These are strings the
target does carry and gets wrong.
EOF
  fi

  # A run can give up some leaves and still be refused over one it cannot. What
  # it did give up is reported either way, because those artifacts were edited.
  if [ -n "$withheld_out" ]; then
    echo "" >&2
    held_back_block "$withheld_count" "$withheld_out" >&2
  fi

  if [ "${#refused_deleted[@]}" -gt 0 ]; then
    cat >&2 <<'EOF'

The run removed artifacts the loop owns and brought no decisions home. A
removal carries no content to read, so with nothing decided behind it, it is an
erasure. The nightly passes the count of operations `kapi context push` shared
as --decisions; a run that shared none has nothing to account for a removal.

Refused, removed with no decision behind it:
EOF
    report "${refused_deleted[@]}" >&2
  fi

  if [ "${#refused_foreign[@]}" -gt 0 ]; then
    cat >&2 <<'EOF'

The run also changed files outside the artifacts the loop owns. A convergence
run does not author these, so a run that wrote them is not a run to deliver
from:

Refused, outside the loop's scope:
EOF
    report "${refused_foreign[@]}" >&2
  fi

  local n=$((${#refused_foreign[@]} + ${#refused_deleted[@]}))
  echo "" >&2
  echo "check-loop-output: refused ${n} file(s) the run may not deliver" >&2
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::error title=Sync refused::the run produced content that must not be committed — see the step log"
  fi
  summary "### Loop output gate: REFUSED" "" \
    "The run produced derived content that must not be committed." "" \
    '```' "$(printf '%s\n' "$defect_out"
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
    echo "check-loop-output: cannot read the derived set from 'make l10n-owned-paths' in ${repo}" >&2
    echo "check-loop-output: pass --derived '<pathspecs>' for a checkout without the Makefile" >&2
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
    echo "check-loop-output: cannot read the content pairs from 'make l10n-content-pairs' in ${repo}" >&2
    echo "check-loop-output: pass --pairs '<lang> <artifact>:<reference>...' for a checkout without the Makefile" >&2
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

# planted_repo builds a scratch checkout shaped like this one and prints its
# path. It ignores the project store the way this repository does, and holds a
# recipe naming which leaves are prose, one derived catalog beside the
# inventory it derives from, one demo whose narration sidecar does not exist
# yet, and one source file.
planted_repo() {
  local dir="$1"
  mkdir -p "$dir/core/i18n/catalogs" "$dir/core/i18n/builtins" \
    "$dir/harness/demos/demo-a" "$dir/core/flow"
  printf '/.kapi/\n' >"$dir/.gitignore"
  printf '%s\n' "$SELFTEST_SOURCE" >"$dir/core/i18n/builtins/metadata.json"
  printf '%s\n' "$SELFTEST_TARGET" >"$dir/core/i18n/catalogs/nb.json"
  printf '%s' "$SELFTEST_MASTER" >"$dir/harness/demos/demo-a/demo.yaml"
  printf 'package flow\n' >"$dir/core/flow/executor.go"
  cat >"$dir/kapi.yaml" <<'EOF'
version: v1
name: planted
defaults:
  source_language: en
  target_languages: [nb]
  formats:
    yaml:
      config:
        keyPathPatterns:
          - narration.*.text
collections:
  - name: engine
    base: core/i18n
    content:
      - path: builtins/metadata.json
        format:
          name: json
          config:
            extractAllPairs: false
            extractionRules: '(displayName|description)$'
        target: catalogs/{lang}.json
  - name: demos
    base: harness/demos
    content:
      - path: "*/demo.yaml"
        target: "{dir}/demo.{lang}.yaml"
EOF
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
  out="$(gate "$repo" "$SELFTEST_DERIVED" "$SELFTEST_PAIRS" "${HOLD:-}" "${DECISIONS:-0}" 2>&1)" || rc=$?
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
  expect "a rewritten catalog commits when its content is sound" \
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
  expect "deleting a catalog in a run that decided nothing is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erasure"

  DECISIONS=0
  expect "deleting a catalog with --decisions 0 is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erasure"

  # Decisions the run brought home account for the removal. They do not buy
  # anything else past the gate: what the run wrote is still read.
  DECISIONS=3
  expect "deleting a catalog with --decisions 3 commits" "$repo" 0 \
    "core/i18n/catalogs/nb.json" "3 decision(s)"

  printf '%s' "${SELFTEST_SIDECAR//id: discover/id: oppdag}" \
    >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "decisions do not excuse unsound content beside a removal" "$repo" 1 \
    "harness/demos/demo-a/demo.nb.yaml" "oppdag" "identifier"
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"

  printf 'package flow // edited\n' >"$repo/core/flow/executor.go"
  expect "decisions do not excuse a source edit" "$repo" 1 \
    "core/flow/executor.go" "outside the loop's scope"
  DECISIONS=""
  git -C "$repo" checkout -q -- .
  rm -f "$repo/core/i18n/catalogs/nb.json"

  printf 'package flow // edited\n' >"$repo/core/flow/executor.go"
  expect "a source edit is refused" "$repo" 1 \
    "core/flow/executor.go" "outside the loop's scope"

  # What a run leaves in the project store is ignored by git, so it is neither
  # derived nor foreign: the store's work tree, its database and a decision
  # shard all stay out of the classification.
  git -C "$repo" checkout -q -- .
  mkdir -p "$repo/.kapi/work" "$repo/.kapi/state"
  printf 'store\n' >"$repo/.kapi/work/store.db"
  printf 'store\n' >"$repo/.kapi/store.db"
  printf '{"op":"approve"}\n' >"$repo/.kapi/state/decisions.jsonl"
  local store_out store_rc=0
  store_out="$(gate "$repo" "$SELFTEST_DERIVED" "$SELFTEST_PAIRS" 2>&1)" || store_rc=$?
  if [ "$store_rc" -eq 0 ] && ! printf '%s\n' "$store_out" | grep -qF ".kapi/"; then
    echo "✓ self-test: files the run left under .kapi/ are ignored, not refused as foreign"
  else
    echo "✖ self-test: the project store reached the classification (exit ${store_rc}):"
    printf '%s\n' "$store_out" | sed 's/^/    /'
    SELFTEST_STATUS=1
  fi

  # A store write beside a removal does not make the removal deliverable.
  rm -f "$repo/core/i18n/catalogs/nb.json"
  expect "a removal is refused with the project store written beside it" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erasure"

  git -C "$repo" checkout -q -- .
  rm -rf "$repo/.kapi"

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
  # merely in its log.
  : >"$outfile"
  printf '%s\n' "${SELFTEST_TARGET/Kontrollerte/Sjekket}" >"$repo/core/i18n/catalogs/nb.json"
  printf '%s' "$SELFTEST_SIDECAR" >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "two sound derived changes commit" "$repo" 0
  if grep -qx 'derived=2' "$outfile" && grep -qx 'withheld=0' "$outfile"; then
    echo "✓ self-test: the counts are published as step outputs"
  else
    echo "✖ self-test: the step outputs do not carry the counts:"
    sed 's/^/    /' "$outfile"
    SELFTEST_STATUS=1
  fi
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"
  git -C "$repo" checkout -q -- .

  # Holding a leaf back. With --hold-back the gate removes the defective leaves
  # and delivers the rest of what the run wrote, which is what the nightly asks
  # for: one bad translation used to discard a whole night's work. Every runtime
  # that reads this tier falls back to the source string for a key it cannot
  # find, so a withheld leaf reads as the pending work it is.
  HOLD=hold

  printf '{"tools":{"qa":{"displayName":"Kvalitet","category":"quality","description":"Kontrollerte blokk(er)"}}}\n' \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "a defective leaf is held back and the rest of the run is delivered" "$repo" 0 \
    "tools.qa.description" "held back" "missing {count}"
  if grep -q 'Kvalitet' "$repo/core/i18n/catalogs/nb.json" &&
    ! grep -q 'description' "$repo/core/i18n/catalogs/nb.json"; then
    echo "✓ self-test: the withheld leaf left the artifact and the sound ones stayed"
  else
    echo "✖ self-test: the artifact does not carry what the run delivered:"
    sed 's/^/    /' "$repo/core/i18n/catalogs/nb.json"
    SELFTEST_STATUS=1
  fi

  # The counts a reviewer reads come from the gate, so a run that withheld a
  # leaf says so in its outputs and not only in its log.
  : >"$outfile"
  git -C "$repo" checkout -q -- .
  printf '{"tools":{"qa":{"displayName":"Kvalitet","category":"quality","description":"Kontrollerte blokk(er)"}}}\n' \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "a held-back run is still a delivered run" "$repo" 0 "held back"
  if grep -qx 'withheld=1' "$outfile"; then
    echo "✓ self-test: the withheld count is published as a step output"
  else
    echo "✖ self-test: the step outputs do not carry the withheld count:"
    sed 's/^/    /' "$outfile"
    SELFTEST_STATUS=1
  fi

  # The artifact may not be emptied to hold a leaf back: a catalog that came
  # back with nothing in it is an erasure, which is what l10n-collapse-check
  # refuses. A target carrying only the defective leaf is refused instead.
  # The defective leaf has a sibling that is not a string, so the excision
  # itself would succeed and leave a document with no translations in it. A
  # target whose only leaf sits alone in its object is refused by the shape of
  # the edit rather than by the rule, which is not the rule being tested here.
  git -C "$repo" checkout -q -- .
  printf '{"tools":{"qa":{"description":"Kontrollerte blokk(er)","weight":3}}}\n' \
    >"$repo/core/i18n/catalogs/nb.json"
  expect "the artifact's only leaf is refused rather than emptied" "$repo" 1 \
    "tools.qa.description" "placeholder"

  # A defect with no leaf to remove is refused with --hold-back exactly as
  # without it. Zero coverage is never a pass.
  git -C "$repo" checkout -q -- .
  printf 'not json\n' >"$repo/core/i18n/catalogs/nb.json"
  expect "an artifact that does not parse is refused even with hold-back on" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "unparseable"

  # A narration sidecar overlays its master scene for scene, so there is no leaf
  # to take out of one: removing a scalar breaks the structure check by
  # construction. A sidecar's defect refuses the run.
  git -C "$repo" checkout -q -- .
  printf '%s' "${SELFTEST_SIDECAR//id: discover/id: oppdag}" \
    >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a narration sidecar's defect is not held back" "$repo" 1 \
    "harness/demos/demo-a/demo.nb.yaml" "oppdag" "identifier"
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"

  HOLD=""
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

  local count
  for count in "" "three" "-1"; do
    rc=0
    out="$("$0" --repo "$repo" --decisions "$count" 2>&1)" || rc=$?
    if [ "$rc" -eq 2 ] && printf '%s\n' "$out" | grep -qF "non-negative integer"; then
      echo "✓ self-test: --decisions '${count}' is refused, not read as a count"
    else
      echo "✖ self-test: --decisions '${count}' was accepted (exit ${rc}):"
      printf '%s\n' "$out" | sed 's/^/    /'
      SELFTEST_STATUS=1
    fi
  done

  return "$SELFTEST_STATUS"
}

# ── entry point ──────────────────────────────────────────────────────────────

main() {
  local repo="" derived="" pairs="" mode="gate" hold="" decisions=0

  while [ $# -gt 0 ]; do
    case "$1" in
      --decisions)
        decisions="${2:-}"
        # A count that is not one is refused rather than read as zero or as
        # many: either reading would decide a removal on a typo.
        case "$decisions" in
          '' | *[!0-9]*)
            echo "check-loop-output: --decisions takes a non-negative integer, got '${decisions}'" >&2
            return 2
            ;;
        esac
        shift 2
        ;;
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
      --hold-back)
        hold=hold
        shift
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
        echo "check-loop-output: unknown argument: $1" >&2
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

  gate "$repo" "$derived" "$pairs" "$hold" "$decisions"
}

main "$@"
