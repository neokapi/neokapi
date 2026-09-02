#!/usr/bin/env bash
#
# Guard: no em dash (U+2014) in the text this project ships.
#
# CLAUDE.md caps user-facing prose at one em dash per thousand words and calls
# it the strongest single signal that a page was written by a model. The cap was
# stated for docs, but the same prose reaches a reader through several tiers,
# and only one of them was ever checked. This checks each of them, and every
# part fails on a finding.
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
#   --part dossier   The authored reference prose and the dataset built from it:
#                    the sidecars under scripts/gen-refs/nativedocs, the plugin
#                    manifests and format schemas, the model catalog, and the
#                    generated packages/reference-data/data JSON. Allowance
#                    zero. A `config:` block inside a sidecar holds a worked
#                    recipe snippet and is read past as the code it is, and a
#                    fenced block in the tree's README likewise.
#
#                    The dataset is scanned alongside its inputs because a
#                    plugin that is absent at generation time contributes no
#                    entry, so the inputs alone would leave a gap.
#
#   --part docs      Every .md/.mdx under web/docs, docs/internals and
#                    cli/skills/data, plus the sample READMEs and every
#                    harness/demos/*/demo.yaml. Fenced code blocks hold CLI
#                    output and config, where a dash is data, so they are
#                    skipped for both the count and the word total behind the
#                    allowance. Each page may hold floor(words/1000),
#                    CLAUDE.md's ceiling; a manifest is a set of authored fields
#                    rather than a document and is allowed none however long its
#                    narration runs.
#
#                    The demo manifests are here because their title, subtitle,
#                    tagline and captions are typeset into a video and their
#                    narration is read aloud, so they reach a viewer the way a
#                    page reaches a reader, and nothing else checked them. Only
#                    demo.yaml is read: demo.<locale>.yaml sidecars are
#                    generated target-language artefacts, which never gate a
#                    source change. demos/_retired/ is excluded because the
#                    harness itself skips it, so nothing there is recorded,
#                    typeset or spoken.
#
#                    web/walkthroughs/ is deliberately outside this list. Those
#                    files are the authored unit the demo pipeline records and
#                    narrates from, so they are swept with the demos rather than
#                    with the docs. Add them here when that sweep lands.
#
# With no --part, every part runs and the script exits non-zero if any failed.
#
# Usage:
#     ./scripts/check-em-dashes.sh                     # every part
#     ./scripts/check-em-dashes.sh --part go
#     ./scripts/check-em-dashes.sh --part catalogs --require-extracted
#     ./scripts/check-em-dashes.sh --part ui
#     ./scripts/check-em-dashes.sh --part dossier
#     ./scripts/check-em-dashes.sh --self-test         # prove the matchers
#
# Wired into `make check-em-dashes` (part of `make lint`), `make pre-push`, the
# repo-guards job in .github/workflows/ci.yml (go, ui, dossier and docs, last
# in the job so a failure hides no other guard), and the catalogs job in
# .github/workflows/l10n.yml, where the extract has just run.
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

PART=all
REQUIRE_EXTRACTED=0
SELF_TEST_ONLY=0

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
  all|go|catalogs|docs|ui|dossier) ;;
  *)
    echo "check-em-dashes.sh: unknown part $PART (want go, catalogs, docs, ui or dossier)" >&2
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

# The committed source catalogs. The first two are generated from the Go
# registries and the cobra command tree by `make kapi-i18n-generate` /
# `make kapi-cli-i18n-generate`, so a finding there is fixed in the Go source
# and the inventory regenerated. The third is hand-written email subject copy,
# whose target locales sit beside it as separate files this never reads.
readonly GO_CATALOGS=(
  host/i18n/commands.json
  core/i18n/builtins/metadata.json
  bowrain/mailer/subjects/en.json
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
  else
    echo "✖ catalogs: fix the source string, then re-extract that surface."
    printf '%s\n' "$out"
    rc=1
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

# ── dossier ──────────────────────────────────────────────────────────────────

check_dossier() {
  local out
  if out=$(git ls-files -- \
      'scripts/gen-refs/nativedocs' \
      'packages/reference-data/data/*.json' \
      'providers/ai/models.json' \
      'plugins/*/manifest.json' \
      'plugins/*/formats/*/schema.json' |
      "$MATCHER" -part dossier 2>&1); then
    echo "✓ dossier: no em dash in the reference prose or the dataset it produces"
    return 0
  fi
  echo "✖ dossier: fix the sidecar, manifest or catalog, then run 'make generate-reference-docs'."
  printf '%s\n' "$out"
  return 1
}

# ── docs ─────────────────────────────────────────────────────────────────────

# The sample READMEs are documentation about the repo and are read here. The
# Markdown under samples/<name>/docs/ is not: it is the corpus each sample
# translates, and rewriting a sentence there orphans the translations and the
# committed decisions the sample ships to demonstrate the loop.
check_docs() {
  local out
  if out=$(git ls-files -- \
      'web/docs/**/*.md' 'web/docs/**/*.mdx' \
      'docs/internals/**/*.md' 'docs/internals/**/*.mdx' \
      'cli/skills/data/**/*.md' 'cli/skills/data/**/*.mdx' \
      'web/blog/**/*.md' 'web/blog/**/*.mdx' \
      'samples/README.md' 'samples/*/README.md' \
      'harness/demos/*/demo.yaml' ':!harness/demos/_retired/*' |
      "$MATCHER" -part docs 2>&1); then
    echo "✓ docs: every page is within its em-dash allowance"
    return 0
  fi
  echo "✖ docs: each page is allowed floor(words/1000), a manifest none; target zero."
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
if [ "$PART" = all ] || [ "$PART" = dossier ]; then
  check_dossier || status=1
fi
if [ "$PART" = all ] || [ "$PART" = docs ]; then
  check_docs || status=1
fi

exit "$status"
