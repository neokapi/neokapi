#!/bin/bash
# Sets up a clean git repo copy of the example project for walkthrough tapes.
# Called by generate.sh before running walkthrough tapes.
# Sets WALKTHROUGH_DIR to the temp directory path.

set -e

WALKTHROUGH_DIR="$(mktemp -d)/bowrain-example-docusaurus"

git clone -q https://github.com/gokapi/bowrain-example-docusaurus.git "$WALKTHROUGH_DIR"
cd "$WALKTHROUGH_DIR"
git config user.email "alex@example.com"
git config user.name "Alex Developer"

export WALKTHROUGH_DIR
echo "$WALKTHROUGH_DIR"
