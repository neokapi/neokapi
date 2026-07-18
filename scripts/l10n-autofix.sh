#!/usr/bin/env bash
# l10n-autofix.sh — self-healing wrapper around the deterministic l10n regen
# targets that back the `make *-l10n-verify` CI gates.
#
# Usage:
#   scripts/l10n-autofix.sh [--dry-run] <make-target>... -- <pathspec>...
#
# Runs `make <make-target>...` and compares the owned <pathspec>s against the
# committed state — the same paths the corresponding *-l10n-verify target
# diffs. Then:
#
#   * no drift                 → exit 0 quietly.
#   * drift on a same-repo PR  → commit exactly the owned paths as
#     github-actions[bot], push to the PR head branch, annotate, exit 0.
#     Regeneration is deterministic, so the next run of this gate (on the bot
#     commit) finds no drift — the loop terminates after one commit. The push
#     uses GITHUB_TOKEN, which does NOT trigger new workflow runs; the checks
#     on the current run's SHA are authoritative.
#   * drift anywhere else      → print the drift and fail, exactly like
#     `make *-l10n-verify` today (fork PRs have no push rights; pushes to
#     main keep the strict safety net; local runs stay strict for humans).
#
# Context is passed in via environment (wired in the workflows):
#   GITHUB_EVENT_NAME   "pull_request" enables the auto-fix path
#   AUTOFIX_SAME_REPO   "true" iff the PR head repo == the base repo
#   AUTOFIX_HEAD_REF    the PR head branch to push to
#   GITHUB_WORKFLOW     names the workflow in the bot commit body
#
# --dry-run performs the regen + stage + commit but skips the actual push
# (prints the push command instead) — used for local testing of the wiring.
#
# The Makefile *-l10n-verify targets are untouched: `make X-l10n-verify`
# keeps working locally as the strict gate for humans.
set -euo pipefail

dry_run=0
targets=()

usage() {
  echo "usage: $0 [--dry-run] <make-target>... -- <pathspec>..." >&2
  exit 2
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)
      dry_run=1
      shift
      ;;
    --)
      shift
      break
      ;;
    -*)
      usage
      ;;
    *)
      targets+=("$1")
      shift
      ;;
  esac
done
paths=("$@")

if [[ ${#targets[@]} -eq 0 || ${#paths[@]} -eq 0 ]]; then
  usage
fi

# make targets and the gate pathspecs are repo-root relative.
repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"

echo "Regenerating l10n outputs: make ${targets[*]}"
make "${targets[@]}"

# Drift = a tracked file under the owned paths changed (what the verify
# targets diff), or regeneration produced a new non-ignored file there.
drift=0
if ! git diff --quiet -- "${paths[@]}"; then
  drift=1
fi
untracked=$(git ls-files --others --exclude-standard -- "${paths[@]}")
if [[ -n "$untracked" ]]; then
  drift=1
fi

if [[ $drift -eq 0 ]]; then
  echo "l10n outputs are fresh; nothing to do."
  exit 0
fi

event="${GITHUB_EVENT_NAME:-}"
same_repo="${AUTOFIX_SAME_REPO:-false}"
head_ref="${AUTOFIX_HEAD_REF:-}"

if [[ "$event" == "pull_request" && "$same_repo" == "true" && -n "$head_ref" ]]; then
  # Same-repo PR: self-heal. Stage ONLY the owned paths — never the rest of
  # the working tree — so an unexpected unrelated change can never ride along
  # in the bot commit.
  git add -- "${paths[@]}"
  if git diff --cached --quiet; then
    echo "Drift was not stageable under the owned paths; treating as fresh."
    exit 0
  fi
  echo "Committing regenerated files:"
  git diff --cached --name-status
  git -c user.name="github-actions[bot]" \
    -c user.email="41898282+github-actions[bot]@users.noreply.github.com" \
    commit --quiet \
    -m "chore(l10n): regenerate generated catalogs" \
    -m "Auto-committed by the ${GITHUB_WORKFLOW:-l10n-autofix} workflow (make ${targets[*]})."
  if [[ $dry_run -eq 1 ]]; then
    echo "[dry-run] skipping push: git push origin HEAD:refs/heads/${head_ref}"
  else
    git push origin "HEAD:refs/heads/${head_ref}"
  fi
  echo "::notice::Stale l10n catalogs regenerated and committed to '${head_ref}'. The bot commit was pushed with GITHUB_TOKEN and does not re-trigger workflows — the checks on this run's SHA are authoritative."
  exit 0
fi

# Fork PR, push to main, or a local run: strict gate (today's behavior).
echo "l10n drift detected under the gated paths:"
git --no-pager diff --stat -- "${paths[@]}"
if [[ -n "$untracked" ]]; then
  echo "New untracked generated files:"
  printf '  %s\n' "$untracked"
fi
echo "::error::Generated l10n catalogs are stale. Run 'make ${targets[*]}' locally and commit the regenerated files. (Auto-fix only runs on same-repo pull requests; this context has no push rights or is a push to main, where the gate stays strict.)"
exit 1
