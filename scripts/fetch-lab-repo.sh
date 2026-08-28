#!/usr/bin/env bash
#
# fetch-lab-repo.sh — Clone the subject repository the authoring lab reads.
#
# Usage:
#   ./scripts/fetch-lab-repo.sh
#
# Clones ripgrep at a pinned tag into ./lab-repo/<name>-<tag>/ at the repository
# root, gitignored. The versioned directory makes updates idempotent: bumping
# LAB_REPO_TAG picks up a new revision without FORCE_FETCH.
#
# Pinned, and shallow at that tag rather than at a branch head, because the lab
# publishes documents an agent wrote from this source. A floating clone would
# make two runs of the same eval read different code, and the difference would
# be reported as a difference between models.
#
# Environment:
#   LAB_REPO_URL   — Override the repository (default: ripgrep).
#   LAB_REPO_NAME  — Override the local directory name (default: ripgrep).
#   LAB_REPO_TAG   — Override the tag (default: 14.1.1).
#   FORCE_FETCH    — Re-clone even when the versioned directory exists.

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

REPO_URL="${LAB_REPO_URL:-https://github.com/BurntSushi/ripgrep.git}"
REPO_NAME="${LAB_REPO_NAME:-ripgrep}"
REPO_TAG="${LAB_REPO_TAG:-14.1.1}"

dest="lab-repo/${REPO_NAME}-${REPO_TAG}"

if [ -d "$dest" ] && [ -z "${FORCE_FETCH:-}" ]; then
  echo "✓ $dest already present (FORCE_FETCH=1 to re-clone)"
  exit 0
fi

rm -rf "$dest"
mkdir -p "$(dirname "$dest")"

echo "→ cloning $REPO_URL at $REPO_TAG → ${dest}…"
git clone --depth 1 --branch "$REPO_TAG" --quiet "$REPO_URL" "$dest"

# The .git directory is 90% of the download and the lab never reads it: the
# agent is given a source tree, not a history.
rm -rf "$dest/.git"

files=$(find "$dest" -type f | wc -l | tr -d ' ')
bytes=$(du -sh "$dest" | cut -f1)
echo "✓ $REPO_NAME $REPO_TAG → $dest ($files files, $bytes)"
