#!/usr/bin/env bash
# Demonstrate that kapi handles the same HTML asset via its built-in NATIVE
# reader AND via the okapi-bridge okf_html filter. Deterministic, offline
# (pseudo-translate; no LLM). Readers are selected explicitly with --map so the
# comparison is unambiguous.
#
# Head-to-head native↔okapi parity across the shared formats is verified continuously
# by the parity suite (cli/parity, `make parity`); this cell is the human-facing
# illustration of that capability.
set -euo pipefail
cd "$(dirname "$0")"
. ../_scripts/lib.sh

mkdir -p out
SRC=fixtures/page.html

wc_for() { # wc_for FORMAT  → translatable word count (0 on failure)
  "$KAPI" word-count "$SRC" --map "*.html=$1" --json 2>/dev/null \
    | python3 -c 'import json,sys; print(json.load(sys.stdin).get("total_source_words",0))' 2>/dev/null || echo 0
}

# NATIVE reader (built-in): word count + pseudo-translate round-trip.
NATIVE_WC=$(wc_for html)
"$KAPI" pseudo-translate "$SRC" --map '*.html=html' -o out/page.native.html >/dev/null 2>&1 || true

# OKAPI reader (okapi-bridge okf_html), when the plugin is installed AND healthy.
OKAPI_WC=0
OKAPI_STATUS="not installed"
if okapi_installed; then
  OKAPI_WC=$(wc_for okf_html)
  "$KAPI" pseudo-translate "$SRC" --map '*.html=okf_html' -o out/page.okapi.html >/dev/null 2>&1 || true
  if [ "$OKAPI_WC" -gt 0 ]; then OKAPI_STATUS="ok"; else OKAPI_STATUS="bridge unavailable in this environment"; fi
fi

NATIVE_RT=false; [ -s out/page.native.html ] && NATIVE_RT=true
OKAPI_RT=false;  [ -s out/page.okapi.html ]  && OKAPI_RT=true

python3 - "$NATIVE_WC" "$OKAPI_WC" "$OKAPI_STATUS" "$NATIVE_RT" "$OKAPI_RT" <<'PY' > parity.json
import json, sys
native, okapi, status, nrt, ort = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3], sys.argv[4] == "true", sys.argv[5] == "true"
# "both_ok" = each reader extracted content AND produced a round-trip. The two
# readers segment HTML slightly differently (hence different word counts); the
# authoritative, normalized native↔okapi comparison is the parity suite.
print(json.dumps({
    "asset": "fixtures/page.html",
    "native": {"reader": "html",     "word_count": native, "round_trip": nrt},
    "okapi":  {"reader": "okf_html",  "word_count": okapi,  "round_trip": ort, "status": status},
    "both_ok": (native > 0 and nrt) and (okapi > 0 and ort),
    "note": "Word counts differ by reader segmentation; the normalized native↔okapi parity across the shared formats is verified by cli/parity (`make parity`).",
}, indent=2))
PY
echo "parity:"; cat parity.json
