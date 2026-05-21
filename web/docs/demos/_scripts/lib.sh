#!/usr/bin/env bash
# Shared helpers for demo cells. Source from a cell's run.sh:
#   . ../_scripts/lib.sh
# Override the binary with KAPI=/path/to/kapi.
set -euo pipefail

KAPI="${KAPI:-kapi}"

# brand_score_text TEXT PACK  → BrandComplianceScore JSON on stdout
brand_score_text() {
  printf '%s' "$1" | "$KAPI" brand check --pack "$2" --text - --json 2>/dev/null
}

# brand_rewrite_text TEXT PACK  → rewrite result JSON on stdout
brand_rewrite_text() {
  printf '%s' "$1" | "$KAPI" brand rewrite --pack "$2" --text - --json 2>/dev/null
}

# json_field FILE FIELD  → extract a top-level JSON field
json_field() { python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))[sys.argv[2]])' "$1" "$2"; }

# make_scorecard WITHOUT_SCORE_JSON WITH_SCORE_JSON  → scorecard JSON on stdout
make_scorecard() {
  python3 - "$1" "$2" <<'PY'
import json, sys
w = json.load(open(sys.argv[1]))
k = json.load(open(sys.argv[2]))
print(json.dumps({
    "profile": k.get("profile"),
    "without": {"score": w["score"], "findings": len(w.get("findings") or [])},
    "with":    {"score": k["score"], "findings": len(k.get("findings") or [])},
    "delta":   k["score"] - w["score"],
}, indent=2))
PY
}

# okapi_installed  → 0 if the okapi-bridge plugin provides okf_* formats.
# (Capture first to avoid a SIGPIPE/pipefail false negative from grep -q.)
okapi_installed() {
  local out
  out=$("$KAPI" formats list 2>/dev/null || true)
  printf '%s' "$out" | grep -q "okapi-bridge"
}
