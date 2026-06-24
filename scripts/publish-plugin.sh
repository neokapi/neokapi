#!/usr/bin/env bash
# Publish the assembled Claude Code plugin bundle to the neokapi-plugins
# marketplace repo.
#
# This is a RELEASE step, not a build step. The kapi binary build never depends
# on the marketplace repo: the skill source of truth lives in cli/skills/data,
# is embedded into the binary, and `kapi skills export` (run by `make
# plugin-bundle`) regenerates the same bytes into <src>/plugins/kapi/skills.
# This script only mirrors that already-assembled bundle into the marketplace
# repo so `/plugin install kapi@neokapi-plugins` serves it.
#
# Usage: scripts/publish-plugin.sh <plugin-src-dir> <owner/repo>
#   e.g. scripts/publish-plugin.sh packages/kapi-claude-plugin neokapi/claude-plugins
#
# Requires push access to the target repo (the gh credential helper locally, or
# a PAT/deploy key in CI). Pass STAMP=<iso8601> to make the commit reproducible.
set -euo pipefail

SRC="${1:?usage: publish-plugin.sh <plugin-src-dir> <owner/repo>}"
REPO="${2:?usage: publish-plugin.sh <plugin-src-dir> <owner/repo>}"
STAMP="${STAMP:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

if [ ! -d "$SRC/plugins/kapi/skills/kapi" ]; then
  echo "error: $SRC/plugins/kapi/skills/kapi missing — run 'make plugin-bundle' first" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git clone --depth 1 "https://github.com/$REPO.git" "$WORK" 2>/dev/null

# Mirror only the bundle paths; never touch the target's .git or .github (CI).
mkdir -p "$WORK/.claude-plugin" "$WORK/plugins"
rsync -a --delete "$SRC/.claude-plugin/" "$WORK/.claude-plugin/"
rsync -a --delete "$SRC/plugins/" "$WORK/plugins/"
cp "$SRC/README.md" "$WORK/README.md"

# Stamp the release version into the published manifests (the monorepo source
# stays at its base version; the version is a publish-time concern). Tracks the
# kapi release so the marketplace skill always matches a shipped CLI surface.
if [ -n "${VERSION:-}" ]; then
  python3 - "$WORK" "$VERSION" <<'PY'
import json, sys
work, ver = sys.argv[1], sys.argv[2]
mp = f"{work}/.claude-plugin/marketplace.json"
pl = f"{work}/plugins/kapi/.claude-plugin/plugin.json"
m = json.load(open(mp)); m.setdefault("metadata", {})["version"] = ver
json.dump(m, open(mp, "w"), indent=2); open(mp, "a").write("\n")
p = json.load(open(pl)); p["version"] = ver
json.dump(p, open(pl, "w"), indent=2); open(pl, "a").write("\n")
PY
fi

cd "$WORK"
git add -A
if git diff --cached --quiet; then
  echo "marketplace $REPO already up to date"
  exit 0
fi
git commit -q -m "Publish kapi plugin bundle ($STAMP)"
git push -q
echo "published kapi plugin bundle → $REPO"
