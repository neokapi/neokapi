#!/usr/bin/env bash
#
# Guard: the written record describes context as living in the store.
#
# A project's terms, voice profiles, content memory and unit decisions live in
# the per-user workspace store. One command reads a checkout's context files
# into it, `kapi context import`, and a person runs it. Nothing else opens one.
#
# Prose that tells a reader to commit their context, that names `.kapi/terms.json`
# as the thing a gate reads, or that names the retired `kapi commit`, describes a
# product that no longer exists. It also teaches the habit the change exists to
# end: a branch deciding what a gate enforces.
#
# What fails: any of these spellings in a tracked prose file under the swept
# trees —
#
#     kapi commit                 the retired verb
#     .kapi/terms.json            the artifact, named as a source
#     .kapi/voice.yaml            the same
#     .kapi/memory/               the same
#     .kapi/state/                the same
#     .kapi/profiles/             the same
#     commit .kapi                the retired instruction
#     committed context           the retired framing
#     context files               fine when it names the artifact, caught here so
#                                 each use is stated on the allowlist
#
# What passes: a line on the allowlist below, each with the reason it is there.
# The bar for joining it is that the passage describes the ARTIFACT role (what a
# snapshot writes, what an import reads, what an export carries) or the
# first-meeting notice, rather than telling a reader that their context lives in
# a file.
#
# Usage:
#     ./scripts/check-docs-store-only.sh              # scan tracked files
#     ./scripts/check-docs-store-only.sh --self-test  # prove the matcher both ways
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# The trees this sweeps: the published docs, the shipped agent skill, the repo's
# own internals, the front-door READMEs and the platform's CLI docs.
TREES=(
  web/docs
  cli/skills
  docs/internals
  README.md
  CONTRIBUTING.md
  .kapi/README.md
  bowrain/web/docs
)

# Generated output. `web/docs/reference/commands/**` is written from the CLI's
# own help by `make generate-reference-pages`; fixing a phrase there means fixing
# the command's help.
EXCLUDE_DIRS=(
  web/docs/reference/commands
)

PATTERN='kapi commit|\.kapi/(terms\.json|voice\.yaml|memory/|state/|profiles/)|commit \.kapi|committed context|context files'

# Each entry is a file, under a comment saying what it names those paths for. A
# file whose last such passage goes comes off the list, and the self-test below
# proves the matcher still catches a spelling nothing has listed.
ALLOWLIST=(
  # The four verbs' own page: it documents the artifact layout each one writes
  # and reads, which is the whole subject.
  "web/docs/kapi/context-portability.mdx"
  # The store page and the context page name the artifact once each, pointing
  # at the import.
  "web/docs/kapi/project-store.mdx"
  "web/docs/kapi/context.mdx"
  # The recipe and serialization references document the file shapes.
  "web/docs/reference/project-file.mdx"
  "web/docs/reference/serialization/terms.mdx"
  "web/docs/reference/serialization/project-state.mdx"
  "web/docs/reference/serialization/project-archive.mdx"
  "web/docs/reference/serialization/overview.mdx"
  "web/docs/reference/serialization/choosing.mdx"
  # The ADs state where each artifact sits in the layout and which command
  # writes or reads it.
  "web/docs/contribute/architecture/context/c-01-project-model.md"
  "web/docs/contribute/architecture/context/c-02-coordinates-and-governance.md"
  "web/docs/contribute/architecture/context/c-04-unit-state-and-decisions.md"
  "web/docs/contribute/architecture/context/c-08-terms.md"
  "web/docs/contribute/architecture/context/c-09-content-memory.md"
  "web/docs/contribute/architecture/context/c-11-context-operations.md"
  "web/docs/contribute/architecture/multilingual/m-06-content-packages.md"
  "web/docs/contribute/implementation/context/kapi-project-file.md"
  # The CI pages name the one import step a runner needs before a gate.
  "web/docs/kapi/recipes/ship-gates-and-ci.mdx"
  "web/docs/kapi/recipes/machine-ship-strategy.mdx"
  # The storage recipe walks import, snapshot, export and restore.
  "web/docs/kapi/recipes/memory-and-terms-storage.mdx"
  # SKILL.md tells an agent that reading a checkout's context files is the
  # person's to do.
  "cli/skills/data/kapi/SKILL.md"
  # The dogfood loop's own documents. They describe the export this repository
  # keeps in git, the import that reads it into the store one step before
  # `kapi up`, and the snapshot that writes it back out.
  "docs/internals/l10n-ci.md"
  "docs/internals/brand-communication.md"
  ".kapi/README.md"
  # The platform's CLI docs describe the same artifact layout for the same
  # reason the framework's reference does.
  "bowrain/web/docs/docs/cli/overview.md"
  "bowrain/web/docs/docs/cli/project-model.md"
  "bowrain/web/docs/docs/cli/commands/init.md"
)

allowed() {
  local file=$1
  local entry
  for entry in "${ALLOWLIST[@]}"; do
    if [[ $file == "$entry" ]]; then
      return 0
    fi
  done
  return 1
}

excluded() {
  local file=$1
  local dir
  for dir in "${EXCLUDE_DIRS[@]}"; do
    if [[ $file == "$dir"/* ]]; then
      return 0
    fi
  done
  return 1
}

self_test() {
  local tmp
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' RETURN
  printf 'Run kapi commit and commit the shards.\n' >"$tmp/bad.md"
  printf 'The context lives in the project store.\n' >"$tmp/good.md"
  if ! grep -nEq "$PATTERN" "$tmp/bad.md"; then
    echo "self-test: the matcher missed a retired spelling" >&2
    return 1
  fi
  if grep -nEq "$PATTERN" "$tmp/good.md"; then
    echo "self-test: the matcher flagged store-only prose" >&2
    return 1
  fi
  echo "self-test: the matcher catches the retired spelling and passes the current one"
}

if [[ ${1:-} == --self-test ]]; then
  self_test
  exit $?
fi

failed=0
while IFS= read -r file; do
  case "$file" in
    *.md | *.mdx) ;;
    *) continue ;;
  esac
  if excluded "$file" || allowed "$file"; then
    continue
  fi
  if hits=$(grep -nE "$PATTERN" "$file"); then
    if [[ $failed -eq 0 ]]; then
      echo "A project's context lives in its store. These passages describe it as a file:"
      echo
    fi
    failed=1
    while IFS= read -r hit; do
      echo "  $file:$hit"
    done <<<"$hits"
  fi
done < <(git ls-files -- "${TREES[@]}")

if [[ $failed -eq 1 ]]; then
  cat >&2 <<'EOF'

Say where the thing lives: the project's terms store, voice store, content
memory or decision ledger, all in the user's workspace. Where the passage is
genuinely about the artifact a snapshot writes and an import reads, add the file
to the allowlist in this script with a line saying why.
EOF
  exit 1
fi

echo "check-docs-store-only: the written record describes context as living in the store"
