#!/bin/bash
# Sets up a clean git repo copy of the example project for walkthrough tapes.
# Called by generate.sh before running walkthrough tapes.
# Sets WALKTHROUGH_DIR to the temp directory path.

set -e

WALKTHROUGH_DIR="$(mktemp -d)/docusaurus-e2e"

# Clone the example project from GitHub (or use local copy as fallback).
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LOCAL_EXAMPLE="$SCRIPT_DIR/../../../../examples/docusaurus-e2e"

if [ -d "$LOCAL_EXAMPLE" ]; then
  cp -r "$LOCAL_EXAMPLE" "$WALKTHROUGH_DIR"
  cd "$WALKTHROUGH_DIR"
  git init -q
  git config user.email "alex@example.com"
  git config user.name "Alex Developer"
  git add -A
  git commit -q -m "Initial commit"
else
  git clone -q https://github.com/gokapi/docusaurus-e2e.git "$WALKTHROUGH_DIR"
  cd "$WALKTHROUGH_DIR"
  git config user.email "alex@example.com"
  git config user.name "Alex Developer"
fi

export WALKTHROUGH_DIR
echo "$WALKTHROUGH_DIR"
