#!/usr/bin/env bash
#
# Guard: no em dash (U+2014) in the text this project ships.
#
# CLAUDE.md caps user-facing prose at one em dash per thousand words and calls
# it the strongest single signal that a page was written by a model. The cap was
# stated for docs, but the same prose reaches a reader through three tiers, and
# only one of them was ever checked. This checks all three.
#
#   --part go        Every string literal in a non-test .go file, across every
#                    module. Comments are exempt, so the matcher parses the file
#                    (scripts/emdashcheck, go/ast) rather than grepping it. Two
#                    exact literals are allowlisted, both cases where the
#                    character is a character: "—" alone is an empty table cell,
#                    and "—-( \t" is a TrimLeft cutset in scripts/contract-audit.
#
#   --part catalogs  The English source text of every extracted source catalog:
#                    the two committed inventories (host/i18n/commands.json,
#                    core/i18n/builtins/metadata.json) and the per-file
#                    .kbf.json under each surface `make l10n-extract-globs`
#                    names. Target-locale trees (i18n-nb/, i18n-qps/) are
#                    separate directories and are never read, and a `targets`
#                    key inside a catalog is skipped: a translation catches up
#                    on the next convergence and must never gate a source
#                    change.
#
#                    The UI catalogs are gitignored build artefacts, produced by
#                    `make l10n-extract` from JSX. So this part scans what has
#                    been extracted rather than the TSX behind it: reading the
#                    catalog is reading exactly what ships to a translator,
#                    where a second extractor written here would drift from the
#                    real one. A surface that has not been extracted is reported
#                    as unscanned; pass --require-extracted (as the l10n
#                    workflow does, where `make l10n-build` has just run) to
#                    make that a failure instead.
#
#                    Findings in this tier are reported without failing for now,
#                    which CATALOGS_HARD_FAIL below explains.
#
#   --part ui        The TS/TSX that renders text no catalog covers, today
#                    packages/kapi-lab/src. It is a dependency of both the docs
#                    site and the desktop app, and it sits in no extract glob,
#                    so its strings reach a reader without passing through a
#                    catalog the part above could read. An em dash outside a
#                    comment is reported: in JS/TSX the character can only
#                    otherwise sit in a string, a template or JSX text. The
#                    durable fix is to add the directory to an extract glob, at
#                    which point this list shrinks to nothing.
#
#   --part docs      Every .md/.mdx under web/docs, docs/internals and
#                    cli/skills/data. Fenced code blocks hold CLI output and
#                    config, where a dash is data, so they are skipped for both
#                    the count and the word total behind the allowance. Each
#                    file may hold floor(words/1000), CLAUDE.md's ceiling.
#
#                    web/walkthroughs/ is deliberately outside this list. Those
#                    files are the authored unit the demo pipeline records and
#                    narrates from, so they are swept with the demos rather than
#                    with the docs. Add them here when that sweep lands.
#
# With no --part, all three run and the script exits non-zero if any failed.
#
# Usage:
#     ./scripts/check-em-dashes.sh                     # every part
#     ./scripts/check-em-dashes.sh --part go
#     ./scripts/check-em-dashes.sh --part catalogs --require-extracted
#     ./scripts/check-em-dashes.sh --part ui
#     ./scripts/check-em-dashes.sh --self-test         # prove the matchers
#
# Wired into `make check-em-dashes` (part of `make lint`), `make pre-push`, the
# repo-guards job in .github/workflows/ci.yml (go, ui and docs, last in the job
# so a failure hides no other guard), and the catalogs job in
# .github/workflows/l10n.yml, where the extract has just run.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

PART=all
REQUIRE_EXTRACTED=0
SELF_TEST_ONLY=0

# Findings in the catalogs tier are reported without failing, because five
# strings in two review dialogs (MarkSourceTermDialog, ProposeSourceChangeDialog)
# are still being rewritten. Set this to 1 in the platform review PR that removes
# them, which is where the tier reaches zero.
readonly CATALOGS_HARD_FAIL=0

while [ $# -gt 0 ]; do
  case "$1" in
    --part)
      PART="${2:-}"
      shift 2
      ;;
    --part=*)
      PART="${1#--part=}"
      shift
      ;;
    --require-extracted)
      REQUIRE_EXTRACTED=1
      shift
      ;;
    --self-test)
      SELF_TEST_ONLY=1
      shift
      ;;
    *)
      echo "check-em-dashes.sh: unknown argument $1" >&2
      exit 2
      ;;
  esac
done

case "$PART" in
  all|go|catalogs|docs|ui) ;;
  *)
    echo "check-em-dashes.sh: unknown part $PART (want go, catalogs, docs or ui)" >&2
    exit 2
    ;;
esac

# The matcher is built once, from its own module with GOWORK=off. It imports
# nothing but the standard library, so building it this way needs Go and no
# module download at all, which is what lets the repo-guards job run it.
MATCHER="$(mktemp -d)/emdashcheck"
# shellcheck disable=SC2064  # expand the path now, not at trap time
trap "rm -rf '$(dirname "$MATCHER")'" EXIT
(cd scripts/emdashcheck && GOWORK=off go build -o "$MATCHER" .)

# A matcher that silently stops matching is worse than no check at all.
if [ "$SELF_TEST_ONLY" = 1 ]; then
  "$MATCHER" --self-test
  exit $?
fi
if ! "$MATCHER" --self-test >/dev/null; then
  echo "✖ check-em-dashes.sh: self-test failed, the matcher is broken."
  "$MATCHER" --self-test || true
  exit 1
fi

status=0

# ── go ───────────────────────────────────────────────────────────────────────

check_go() {
  local out
  if out=$(git ls-files -- '*.go' | "$MATCHER" -part go 2>&1); then
    echo "✓ go: no em dash in any Go string literal"
    return 0
  fi
  echo "✖ go: rewrite each as two sentences, or with a comma or colon."
  printf '%s\n' "$out"
  return 1
}

# ── catalogs ─────────────────────────────────────────────────────────────────

# The two committed inventories, generated from the Go registries and the cobra
# command tree by `make kapi-i18n-generate` / `make kapi-cli-i18n-generate`. A
# finding here is fixed in the Go source and the inventory regenerated.
readonly GO_CATALOGS=(
  host/i18n/commands.json
  core/i18n/builtins/metadata.json
)

check_catalogs() {
  local files=() missing=() dir out
  local rc=0

  for f in "${GO_CATALOGS[@]}"; do
    if [ ! -f "$f" ]; then
      echo "✖ catalogs: $f is missing; run 'make l10n-extract'"
      return 1
    fi
    files+=("$f")
  done

  # The surfaces stage 1 extracts, read from the pipeline rather than re-derived
  # here, so this checks what `make l10n-extract` actually produces.
  while IFS=$'\t' read -r dir _; do
    [ -n "$dir" ] || continue
    if [ ! -d "$dir/i18n" ]; then
      missing+=("$dir/i18n")
      continue
    fi
    while IFS= read -r f; do
      files+=("$f")
    done < <(find "$dir/i18n" -type f -name '*.kbf.json' | sort)
  done < <(make -s l10n-extract-globs)

  if out=$(printf '%s\n' "${files[@]}" | "$MATCHER" -part catalogs 2>&1); then
    echo "✓ catalogs: no em dash in the English source of ${#files[@]} catalog file(s)"
  elif [ "$CATALOGS_HARD_FAIL" = 1 ]; then
    echo "✖ catalogs: fix the source string, then re-extract that surface."
    printf '%s\n' "$out"
    rc=1
  else
    echo "  catalogs: reported, not gated (see CATALOGS_HARD_FAIL). Fix the source string, then re-extract that surface."
    printf '%s\n' "$out"
  fi

  if [ ${#missing[@]} -gt 0 ]; then
    if [ "$REQUIRE_EXTRACTED" = 1 ]; then
      echo "✖ catalogs: not extracted, so not scanned (run 'make l10n-extract'):"
      printf '    %s\n' "${missing[@]}"
      rc=1
    else
      echo "  catalogs: not extracted, so not scanned (run 'make l10n-extract'):"
      printf '    %s\n' "${missing[@]}"
    fi
  fi

  return "$rc"
}

# ── ui ───────────────────────────────────────────────────────────────────────

# Shipped UI source that no extract glob reaches, so no catalog carries its
# strings. Remove a directory from this list when it joins an extract glob.
readonly UNEXTRACTED_UI_DIRS=(
  packages/kapi-lab/src
)

check_ui() {
  local out
  if out=$(git ls-files -- "${UNEXTRACTED_UI_DIRS[@]}" |
      grep -E '\.tsx?$' |
      "$MATCHER" -part ui 2>&1); then
    echo "✓ ui: no em dash in the UI source no catalog covers"
    return 0
  fi
  echo "✖ ui: rewrite each as two sentences, or with a comma or colon."
  printf '%s\n' "$out"
  return 1
}

# ── docs ─────────────────────────────────────────────────────────────────────

check_docs() {
  local out
  if out=$(git ls-files -- \
      'web/docs/**/*.md' 'web/docs/**/*.mdx' \
      'docs/internals/**/*.md' 'docs/internals/**/*.mdx' \
      'cli/skills/data/**/*.md' 'cli/skills/data/**/*.mdx' \
      'web/blog/**/*.md' 'web/blog/**/*.mdx' |
      "$MATCHER" -part docs 2>&1); then
    echo "✓ docs: every page is within its em-dash allowance"
    return 0
  fi
  echo "✖ docs: each file is allowed floor(words/1000); target zero."
  printf '%s\n' "$out"
  return 1
}

# ── main ─────────────────────────────────────────────────────────────────────

if [ "$PART" = all ] || [ "$PART" = go ]; then
  check_go || status=1
fi
if [ "$PART" = all ] || [ "$PART" = catalogs ]; then
  check_catalogs || status=1
fi
if [ "$PART" = all ] || [ "$PART" = ui ]; then
  check_ui || status=1
fi
if [ "$PART" = all ] || [ "$PART" = docs ]; then
  check_docs || status=1
fi

exit "$status"
