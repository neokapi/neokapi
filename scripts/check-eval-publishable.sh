#!/usr/bin/env bash
#
# check-eval-publishable.sh — Refuse to publish a sweep that carries a secret.
#
# Usage:
#   ./scripts/check-eval-publishable.sh [dir]     # default: web/static/skill-eval
#
# Eval transcripts are no longer capped: every tool result is recorded whole. The
# agents that produce them run with bypassPermissions, a shell, and $HOME — so
# one *could* read a credential file and print it, and the result would reach
# whoever reads the eval. The skill eval's go to a public CDN; the authoring
# lab's are committed to this repository, which is a shorter path to the same
# place, so both are checked.
#
# The environment allow-list in scripts/skilleval/run.go removes the easy path
# (no key is in the process to begin with). This is the check on what is about
# to leave the machine, which is a different question and the last one asked.
#
# It scans for shapes rather than for known values, because the values are what
# we do not have: a run on someone else's laptop carries their keys, not ours.

set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

dir="${1:-web/static/skill-eval}"
if [ ! -d "$dir" ]; then
  echo "✓ $dir does not exist — nothing to publish, nothing to check"
  exit 0
fi

# Credential shapes. Each is a prefix a vendor documents, so a match is a real
# key rather than a word that resembles one.
readonly SECRET_RE='sk-ant-[A-Za-z0-9_-]{16}|sk-[A-Za-z0-9]{32}|ghp_[A-Za-z0-9]{20}|gho_[A-Za-z0-9]{20}|github_pat_[A-Za-z0-9_]{20}|AKIA[0-9A-Z]{12}|ASIA[0-9A-Z]{12}|AIza[0-9A-Za-z_-]{30}|xox[baprs]-[A-Za-z0-9-]{10}|-----BEGIN [A-Z ]*PRIVATE KEY-----'

# An assignment that looks like a key being exported, which is what `env` in a
# transcript would produce.
readonly ASSIGN_RE='(ANTHROPIC_API_KEY|OPENAI_API_KEY|GEMINI_API_KEY|GITHUB_TOKEN|GH_TOKEN|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|NPM_TOKEN)[\"'\'']?[:=][\"'\'' ]?[A-Za-z0-9/+_-]{12}'

fail=0

if hits=$(grep -rIlE "$SECRET_RE" "$dir" 2>/dev/null); then
  echo "✖ credential-shaped strings:"
  printf '  %s\n' $hits
  fail=1
fi

if hits=$(grep -rIlE "$ASSIGN_RE" "$dir" 2>/dev/null); then
  echo "✖ what looks like an exported credential:"
  printf '  %s\n' $hits
  fail=1
fi

# Absolute home paths. The skill eval's artefacts are untracked, so
# check-abs-paths.sh never sees them; the authoring lab's are tracked and it
# does. Checked here for both, so one caller does not depend on the other.
if hits=$(grep -rIlE '/(Users|home)/[a-z][a-z0-9_-]*/' "$dir" 2>/dev/null); then
  echo "✖ absolute home paths:"
  printf '  %s\n' $hits
  echo "  scrubPaths (scripts/skilleval/transcript.go, scripts/authoringlab/agent.go)"
  echo "  should have removed these."
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo
  echo "Stopped. These files reach the public — a CDN for the skill eval's, this"
  echo "repository for the lab's. Fix the run that produced them rather than"
  echo "editing the artefacts."
  exit 1
fi

n=$(find "$dir" -type f | wc -l | tr -d ' ')
echo "✓ $n file(s) under $dir carry no credential shape and no home path"
