#!/usr/bin/env bash
# Publish a kapi/neokapi Agent Skill into the neokapi/agent-skills collection repo
# so `npx skills add neokapi/agent-skills` installs it into any SKILL.md-aware tool
# — GitHub Copilot, Claude Code, Cursor, Windsurf, and the rest of the Agent Skills
# ecosystem.
#
# This is a RELEASE step, not a build step. The source of truth is the skill
# directory in the monorepo (e.g. cli/skills/data/kapi), in lockstep with the CLI
# surface; this mirrors that directory into a same-named subdir of the collection
# repo (e.g. agent-skills/kapi/), the layout the skills CLI expects for a
# multi-skill repo. The repo's root README (the collection index) is left alone,
# so independent per-skill publishes don't clobber each other.
#
# Usage: scripts/publish-skill.sh <skill-src-dir> <owner/repo>
#   e.g. scripts/publish-skill.sh cli/skills/data/kapi neokapi/agent-skills
#
# Requires push access to the target repo (the gh credential helper locally, or a
# PAT/deploy key in CI). Pass STAMP=<iso8601> for a reproducible commit.
set -euo pipefail

SRC="${1:?usage: publish-skill.sh <skill-src-dir> <owner/repo>}"
REPO="${2:?usage: publish-skill.sh <skill-src-dir> <owner/repo>}"
STAMP="${STAMP:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
NAME="$(basename "$SRC")"

if [ ! -f "$SRC/SKILL.md" ]; then
  echo "error: $SRC/SKILL.md missing" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone --depth 1 "https://github.com/$REPO.git" "$WORK" 2>/dev/null

# Mirror the whole skill directory into <repo>/<name>/. --delete scopes deletions
# to this skill's subdir only, so other skills and the root README are untouched.
mkdir -p "$WORK/$NAME"
rsync -a --delete "$SRC/" "$WORK/$NAME/"

cd "$WORK"
git add -A
if git diff --cached --quiet; then
  echo "skill '$NAME' in $REPO already up to date"
  exit 0
fi
git commit -q -m "Publish $NAME skill ($STAMP)"
git push -q
echo "published $NAME skill → $REPO"
