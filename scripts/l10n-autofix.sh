#!/usr/bin/env bash
# l10n-autofix.sh — regenerate the build-derived multilingual artifacts and, on a
# same-repo pull request, commit the result instead of failing the build.
#
# Usage: scripts/l10n-autofix.sh [--dry-run]
#
# There is nothing to configure. `make l10n-build` regenerates the tier and
# `make l10n-derived-paths` names exactly what it owns, so this script and the
# Makefile cannot disagree about the gated set — which is the whole reason the
# path list is not repeated here.
#
# The walk it runs has no convergence in it. Extraction and the qps probe read
# committed source, and the compilers read what those two wrote; nothing here
# opens the project store, so nothing here can rewrite a target-language
# artifact. That is the property the auto-commit rests on: every path under the
# gate is a deterministic function of committed source, so:
#
#   * no drift                → exit 0 quietly.
#   * drift on a same-repo PR → commit exactly the owned paths as
#     github-actions[bot], push to the PR head branch, annotate, exit 0.
#     Regeneration is deterministic, so the next run of this gate (on the bot
#     commit) finds no drift — the loop terminates after one commit. The push
#     uses GITHUB_TOKEN, which does NOT trigger new workflow runs; the checks
#     on the current run's SHA are authoritative.
#   * drift anywhere else     → print it and fail, exactly like `make
#     l10n-verify` (fork PRs have no push rights; pushes to main keep the
#     strict safety net; local runs stay strict for humans).
#
# The target-language tier is not here and must not be added. It is written by
# `kapi up` out of the project store, which holds wording no checkout can
# reproduce from git alone, so a bot that regenerated and committed it would
# overwrite approved wording with whatever the seeds happen to hold — green,
# every time, and silent. Its standing is reported instead: `make l10n-report`.
#
# Context is passed in via environment (wired in .github/workflows/l10n.yml):
#   GITHUB_EVENT_NAME   "pull_request" enables the auto-fix path
#   AUTOFIX_SAME_REPO   "true" iff the PR head repo == the base repo
#   AUTOFIX_HEAD_REF    the PR head branch to push to
#   GITHUB_WORKFLOW     names the workflow in the bot commit body
#
# --dry-run performs the regen + stage + commit but skips the actual push
# (prints the push command instead) — used for local testing of the wiring.
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
  # Same-repo PR: self-heal. Stage ONLY the owned paths — never the rest of
  # the working tree — so an unexpected unrelated change can never ride along
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
  echo "::notice::Stale build-derived artifacts regenerated and committed to '${head_ref}'. The bot commit was pushed with GITHUB_TOKEN and does not re-trigger workflows — the checks on this run's SHA are authoritative."
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
