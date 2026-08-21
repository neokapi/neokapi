#!/usr/bin/env bash
# Regenerate the WITH/WITHOUT artifacts for this cell. Deterministic, offline.
set -euo pipefail
cd "$(dirname "$0")"
. ../_scripts/lib.sh

PACK=marketing-blog

# WITHOUT: the AI-alone draft, scored as-is.
voice_score_text "$(cat without/output.md)" "$PACK" > without/score.json

# WITH: kapi rewrites the draft to the voice profile, then we score the result.
voice_rewrite_text "$(cat without/output.md)" "$PACK" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["rewritten"])' > with/output.md
voice_score_text "$(cat with/output.md)" "$PACK" > with/score.json

make_scorecard without/score.json with/score.json > scorecard.json
echo "scorecard:"; cat scorecard.json
