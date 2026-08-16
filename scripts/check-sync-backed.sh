#!/usr/bin/env bash
#
# Guard: a convergence run commits derived artifacts only when the committed
# context moved with them.
#
# `kapi up` writes target-language artifacts — catalogs, narration sidecars,
# runtime dictionaries — out of the project store. A checkout with no server
# seeds that store from `.kapi/`, the committed context graph, so an artifact
# that reached the tree without a matching `.kapi/` change is wording git holds
# in exactly one place: the artifact. The next convergence that runs from a
# colder store writes over it from a context that never learned it. The wording
# is gone, the run that produced it was green, and nothing said so.
#
# This gate is what says so. It classifies everything the run left in the
# working tree, using git for all path matching:
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
# It refuses — non-zero, naming every file — when derived changes have no
# backing change, and when anything foreign changed. Foreign is refused even
# with backing present: backing explains target wording, not a source edit.
#
# A derived change is backed by the run as a whole, not file by file: no
# committed mapping ties a seed or a shard to the artifact it feeds (seed
# filenames are deliberately independent of collection names), so a per-file
# correspondence would be invented rather than read. The honest question this
# can answer is whether the context moved at all.
#
# Usage:
#     ./scripts/check-sync-backed.sh                 # gate the working tree
#     ./scripts/check-sync-backed.sh --repo DIR      # gate another checkout
#     ./scripts/check-sync-backed.sh --derived 'a b' # override the derived set
#     ./scripts/check-sync-backed.sh --self-test     # prove the gate both ways
#
# Wired into .github/workflows/dogfood-sync.yml between `kapi up` and the
# delivery step, and self-tested in the repo-guards job of ci.yml.
set -euo pipefail

# Under .kapi/ minus the derived projection of it. .kapi/work/ is gitignored, so
# the exclusion is belt and braces: a work tree that stopped being ignored must
# not read as the context graph moving.
readonly BACKING_SPECS=('.kapi' ':(exclude).kapi/work')

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

# ── the gate ─────────────────────────────────────────────────────────────────
#
# $1 is the repository to inspect; $2 the whitespace-separated derived pathspecs.
# Returns 0 when the tree is committable, 1 when it refuses.
gate() {
  local repo="$1" derived_spec="$2"
  local rec path
  local all_entries=() all_paths=() derived_paths=() backing_paths=()
  local refused_unbacked=() refused_foreign=() backing_entries=() derived_entries=()

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

  if [ "${#backing_entries[@]}" -eq 0 ]; then
    refused_unbacked=(${derived_entries[@]+"${derived_entries[@]}"})
  fi

  if [ "${#refused_unbacked[@]}" -eq 0 ] && [ "${#refused_foreign[@]}" -eq 0 ]; then
    outputs "derived=${#derived_entries[@]}" "backing=${#backing_entries[@]}"
    if [ "${#derived_entries[@]}" -eq 0 ] && [ "${#backing_entries[@]}" -eq 0 ]; then
      echo "check-sync-backed: the run left nothing to commit"
      summary "### Sync backing gate" "" "The run left nothing to commit."
      return 0
    fi
    echo "check-sync-backed: ${#derived_entries[@]} derived change(s), backed by ${#backing_entries[@]} context change(s)"
    if [ "${#backing_entries[@]}" -gt 0 ]; then
      echo "context (.kapi/):"
      report "${backing_entries[@]}"
    fi
    if [ "${#derived_entries[@]}" -gt 0 ]; then
      echo "derived:"
      report "${derived_entries[@]}"
    fi
    summary "### Sync backing gate" "" \
      "${#derived_entries[@]} derived change(s), backed by ${#backing_entries[@]} change(s) under \`.kapi/\`."
    return 0
  fi

  echo "check-sync-backed: REFUSED — this run must not be committed" >&2

  if [ "${#refused_unbacked[@]}" -gt 0 ]; then
    cat >&2 <<'EOF'

The run rewrote artifacts the loop owns while the committed context under
.kapi/ did not move. Regeneration rebuilds these files from .kapi/, so
committing them delivers wording that the next regeneration erases.

Refused — derived, nothing behind it:
EOF
    report "${refused_unbacked[@]}" >&2
    cat >&2 <<'EOF'

What backs a derived change: a decision shard under .kapi/state/, a change to
.kapi/terms.json, a memory seed under .kapi/memory/, .kapi/voice.yaml, or a
profile under .kapi/profiles/. Carry the run's decisions home (the return legs
of the loop write the shards) and the same artifacts become committable.

Do not hand-edit a seed to clear this gate. Seeds are read-only accelerants;
wording is decided in the ledger, and a seed edited to match an artifact
records a decision nobody made.
EOF
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

  local n=$((${#refused_unbacked[@]} + ${#refused_foreign[@]}))
  echo "" >&2
  echo "check-sync-backed: refused ${n} file(s)" >&2
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    echo "::error title=Sync refused::${n} file(s) the committed context cannot explain — see the step log"
  fi
  summary "### Sync backing gate — REFUSED" "" \
    "\`${n}\` file(s) refused: the run produced changes the committed context under \`.kapi/\` does not explain." "" \
    '```' "$(report ${refused_unbacked[@]+"${refused_unbacked[@]}"} ${refused_foreign[@]+"${refused_foreign[@]}"})" '```'
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

# ── self-test ────────────────────────────────────────────────────────────────
#
# Every case runs the real gate against a real git repository, because what is
# being tested is a classification git performs: pathspec matching, untracked
# listing, and the ignore rules. A fixture that faked git output would prove
# nothing about any of them.

SELFTEST_STATUS=0

# planted_repo builds a scratch checkout shaped like this one — a committed
# context, one derived catalog, one demo whose narration sidecar does not exist
# yet, and one source file — and prints its path.
planted_repo() {
  local dir="$1"
  mkdir -p "$dir/.kapi/memory" "$dir/.kapi/state" "$dir/core/i18n/catalogs" \
    "$dir/harness/demos/demo-a" "$dir/core/flow"
  printf 'work/\n' >"$dir/.kapi/.gitignore"
  printf '{"entries":[]}\n' >"$dir/.kapi/memory/docs-nb.memory.json"
  printf '{"concepts":[]}\n' >"$dir/.kapi/terms.json"
  printf '{"greeting":"Hei"}\n' >"$dir/core/i18n/catalogs/nb.json"
  printf 'narration: hello\n' >"$dir/harness/demos/demo-a/demo.yaml"
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
  out="$(gate "$repo" "$SELFTEST_DERIVED" 2>&1)" || rc=$?
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

  printf '{"greeting":"Hallo"}\n' >"$repo/core/i18n/catalogs/nb.json"
  expect "a catalog rewritten with no context change is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json" "erases"

  printf '{"entries":[{"t":"Hallo"}]}\n' >"$repo/.kapi/memory/docs-nb.memory.json"
  expect "the same catalog passes once a seed moved with it" "$repo" 0 \
    "core/i18n/catalogs/nb.json" ".kapi/memory/docs-nb.memory.json"

  git -C "$repo" checkout -q -- .
  printf 'narration: hei\n' >"$repo/harness/demos/demo-a/demo.nb.yaml"
  expect "a first narration sidecar is refused when untracked and unbacked" "$repo" 1 \
    "harness/demos/demo-a/demo.nb.yaml"

  printf '{"unit":"a","variant":"nb"}\n' >"$repo/.kapi/state/demo-a.jsonl"
  expect "the sidecar passes once a decision shard backs it" "$repo" 0 \
    "harness/demos/demo-a/demo.nb.yaml" ".kapi/state/demo-a.jsonl"

  rm -f "$repo/.kapi/state/demo-a.jsonl" "$repo/harness/demos/demo-a/demo.nb.yaml"
  rm -f "$repo/core/i18n/catalogs/nb.json"
  expect "deleting a catalog with no context change is refused" "$repo" 1 \
    "core/i18n/catalogs/nb.json"

  git -C "$repo" checkout -q -- .
  printf 'package flow // edited\n' >"$repo/core/flow/executor.go"
  printf '{"concepts":[{"id":"a"}]}\n' >"$repo/.kapi/terms.json"
  expect "a source edit is refused even with the context moved" "$repo" 1 \
    "core/flow/executor.go" "outside the loop's scope"

  git -C "$repo" checkout -q -- .
  printf '{"concepts":[{"id":"a"}]}\n' >"$repo/.kapi/terms.json"
  expect "context moving on its own commits" "$repo" 0 ".kapi/terms.json"

  git -C "$repo" checkout -q -- .
  mkdir -p "$repo/.kapi/work"
  printf 'store\n' >"$repo/.kapi/work/store.db"
  expect "the gitignored work tree is invisible" "$repo" 0

  git -C "$repo" checkout -q -- .
  rm -rf "$repo/.kapi/work"

  printf '{"greeting":"Hallo"}\n' >"$repo/core/i18n/catalogs/nb.json"
  local out rc=0
  out="$(gate "$repo" '   ' 2>&1)" || rc=$?
  if [ "$rc" -eq 2 ] && printf '%s\n' "$out" | grep -qF "derived set is empty"; then
    echo "✓ self-test: an empty derived set is refused, not read as approval"
  else
    echo "✖ self-test: an empty derived set did not stop the gate (exit ${rc}):"
    printf '%s\n' "$out" | sed 's/^/    /'
    SELFTEST_STATUS=1
  fi
  git -C "$repo" checkout -q -- .

  # What the delivery step tells a reviewer the run produced comes from here,
  # so a run that classified two derived changes behind one context change must
  # say so in its outputs and not merely in its log.
  : >"$outfile"
  printf '{"greeting":"Hallo"}\n' >"$repo/core/i18n/catalogs/nb.json"
  printf 'narration: hei\n' >"$repo/harness/demos/demo-a/demo.nb.yaml"
  printf '{"concepts":[{"id":"a"}]}\n' >"$repo/.kapi/terms.json"
  expect "two derived changes behind one context change commit" "$repo" 0
  if grep -qx 'derived=2' "$outfile" && grep -qx 'backing=1' "$outfile"; then
    echo "✓ self-test: the counts are published as step outputs"
  else
    echo "✖ self-test: the step outputs do not carry the counts:"
    sed 's/^/    /' "$outfile"
    SELFTEST_STATUS=1
  fi
  rm -f "$repo/harness/demos/demo-a/demo.nb.yaml"
  git -C "$repo" checkout -q -- .

  # The gate arms itself in this repository: the derived set comes from the
  # Makefile, and a rename there must fail loudly rather than pass everything.
  cd "$start"
  local root spec
  root="$(cd "$(dirname "$0")/.." && pwd)"
  if spec="$(resolve_derived "$root")" && [ -n "${spec//[[:space:]]/}" ]; then
    echo "✓ self-test: the derived set resolves from the Makefile"
  else
    echo "✖ self-test: the derived set does not resolve in ${root}"
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
  local repo="" derived="" mode="gate"

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

  gate "$repo" "$derived"
}

main "$@"
