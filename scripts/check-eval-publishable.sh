#!/usr/bin/env bash
#
# check-eval-publishable.sh — Refuse to publish a sweep that carries a secret.
#
# Usage:
#   ./scripts/check-eval-publishable.sh [dir]     # default: web/static/skill-eval
#
# The skill eval's transcripts are published to a public CDN and are no longer
# capped: every tool result is recorded whole. The agent that produced them ran
# with bypassPermissions, a shell, and $HOME — so it *could* read a credential
# file and print it, and the result would be served to anyone.
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
  echo "✖ credential-shaped strings in files about to be published:"
  printf '  %s\n' $hits
  fail=1
fi

if hits=$(grep -rIlE "$ASSIGN_RE" "$dir" 2>/dev/null); then
  echo "✖ what looks like an exported credential in files about to be published:"
  printf '  %s\n' $hits
  fail=1
fi

# Absolute home paths. scripts/check-abs-paths.sh sweeps tracked files and these
# are deliberately untracked, so nothing else would look.
if hits=$(grep -rIlE '/(Users|home)/[a-z][a-z0-9_-]*/' "$dir" 2>/dev/null); then
  echo "✖ absolute home paths in files about to be published:"
  printf '  %s\n' $hits
  echo "  scrubPaths in scripts/skilleval/transcript.go should have removed these."
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo
  echo "Nothing was uploaded. These files go to a public CDN; fix the run that"
  echo "produced them rather than editing the artefacts."
  exit 1
fi

n=$(find "$dir" -type f | wc -l | tr -d ' ')
echo "✓ $n file(s) under $dir carry no credential shape and no home path"
