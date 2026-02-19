#!/bin/bash
# Sets up a clean git repo copy of the example project for walkthrough tapes.
# Called by generate.sh before running walkthrough tapes.
# Sets WALKTHROUGH_DIR to the temp directory path.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
EXAMPLE_DIR="$SCRIPT_DIR/../../../examples/docusaurus-e2e"

WALKTHROUGH_DIR="$(mktemp -d)/docusaurus-e2e"
cp -r "$EXAMPLE_DIR" "$WALKTHROUGH_DIR"
cd "$WALKTHROUGH_DIR"

git init -q
git add -A
git commit -q -m "Initial commit" \
  --author="Alex Developer <alex@example.com>"

export WALKTHROUGH_DIR
echo "$WALKTHROUGH_DIR"
