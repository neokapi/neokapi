#!/usr/bin/env bash
# l10n-autofix.sh: regenerate the build-derived multilingual artifacts and, on a
# same-repo pull request, commit the result instead of failing the build.
#
# Usage: scripts/l10n-autofix.sh [--dry-run]
#
# --dry-run performs the regen, stage and commit but prints the push command
# instead of running it, for local testing. `make l10n-build` regenerates the
# tier and `make l10n-derived-paths` names what it owns, so the gated set has
# one definition.

# The walk has no convergence in it: extraction and the qps probe read
# committed source, and the compilers read what those wrote. Every gated path is
# a deterministic function of committed source, so:
#
#   * no drift                -> exit 0 quietly.
#   * drift on a same-repo PR -> commit exactly the owned paths as
#     github-actions[bot], push to the PR head branch, annotate, exit 0. A
#     rerun on the bot commit finds no drift.
#   * drift anywhere else     -> print it and fail, like `make l10n-verify`.
#     Fork PRs have no push rights; main and local runs stay strict.

# The push uses GITHUB_TOKEN, so GitHub starts no workflow runs for the bot
# commit, or starts them as `action_required`. Branch protection evaluates the
# checks of the PR's new head, the bot commit, so the PR cannot merge until CI
# runs on it: someone pushes another commit or closes and reopens the PR, which
# costs a full CI cycle. The app qps dictionaries are build output for that
# reason (`make app-qps`), which leaves this tier to change rarely.

# The target-language tier must not be added here. `kapi up` writes it from the
# project store, which holds wording git alone cannot reproduce, so a bot that
# regenerated and committed it would overwrite approved wording while every
# check passed. Its standing is reported instead: `make l10n-report`.

# Context comes from the environment (wired in .github/workflows/l10n.yml):
#   GITHUB_EVENT_NAME   "pull_request" enables the auto-fix path
#   AUTOFIX_SAME_REPO   "true" iff the PR head repo == the base repo
#   AUTOFIX_HEAD_REF    the PR head branch to push to
#   GITHUB_WORKFLOW     names the workflow in the bot commit body
set -euo pipefail

dry_run=0
case "${1:-}" in
  --dry-run) dry_run=1 ;;
  "") ;;
  *) echo "usage: $0 [--dry-run]" >&2; exit 2 ;;
esac

# make targets and the gate pathspecs are repo-root relative.
repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

echo "Regenerating the build-derived multilingual artifacts: make l10n-build"
make l10n-build

# The pathspec list comes from the Makefile so there is one source of truth.
# Entries may carry git pathspec magic (":(glob)…"); those survive word
# splitting because no file matches them, so the shell leaves the word alone.
read -ra paths < <(make -s l10n-derived-paths)

# Drift = a tracked file under the owned paths changed, or regeneration
# produced a new non-ignored file there.
drift=0
if ! git diff --quiet -- "${paths[@]}"; then
  drift=1
fi
untracked=$(git ls-files --others --exclude-standard -- "${paths[@]}")
if [[ -n "$untracked" ]]; then
  drift=1
fi

if [[ $drift -eq 0 ]]; then
  echo "Build-derived multilingual artifacts are fresh; nothing to do."
  exit 0
fi

event="${GITHUB_EVENT_NAME:-}"
same_repo="${AUTOFIX_SAME_REPO:-false}"
head_ref="${AUTOFIX_HEAD_REF:-}"

if [[ "$event" == "pull_request" && "$same_repo" == "true" && -n "$head_ref" ]]; then
  # Same-repo PR: self-heal. Stage ONLY the owned paths, never the rest of
  # the working tree, so an unexpected unrelated change can never ride along
  # in the bot commit. `git add` (unlike diff and ls-files) fatals on a
  # pathspec that matches nothing, so it gets the resolved file lists rather
  # than the raw pathspecs.
  { git diff --name-only -z -- "${paths[@]}"
    git ls-files --others --exclude-standard -z -- "${paths[@]}"
  } | git add --pathspec-from-file=- --pathspec-file-nul
  if git diff --cached --quiet; then
    echo "Drift was not stageable under the owned paths; treating as fresh."
    exit 0
  fi
  echo "Committing regenerated files:"
  git diff --cached --name-status
  git -c user.name="github-actions[bot]" \
    -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
    commit --quiet \
    -m "chore(l10n): regenerate the build-derived artifacts" \
    -m "Auto-committed by the ${GITHUB_WORKFLOW:-l10n} workflow (make l10n-build)."
  if [[ $dry_run -eq 1 ]]; then
    echo "[dry-run] skipping push: git push origin HEAD:refs/heads/${head_ref}"
  else
    git push origin "HEAD:refs/heads/${head_ref}"
  fi
  echo "::warning::Stale build-derived artifacts regenerated and committed to '${head_ref}'. The bot commit was pushed with GITHUB_TOKEN, so GitHub runs no checks on it, and branch protection holds the pull request until checks pass on that new head. Close and reopen the pull request, or push another commit, to run CI on it. Running 'make l10n-build' before pushing avoids this."
  exit 0
fi

# Fork PR, push to main, or a local run: strict gate.
echo "Drift detected under the gated paths:"
git --no-pager diff --stat -- "${paths[@]}"
if [[ -n "$untracked" ]]; then
  echo "New untracked generated files:"
  printf '  %s\n' "$untracked"
fi
echo "::error::Build-derived artifacts are stale. Run 'make l10n-build' locally and commit the regenerated files. (Auto-fix only runs on same-repo pull requests; this context has no push rights or is a push to main, where the gate stays strict.)"
exit 1
