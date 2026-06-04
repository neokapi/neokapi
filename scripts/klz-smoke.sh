#!/usr/bin/env bash
# klz-smoke: headless gate for the .klz project snapshot hand-off + cached
# resume (AD-025 §5 / #787). Drives a project run (which caches overlays in
# the persistent block store), packs the working state into a .klz, unpacks
# it into a fresh project, and checks that a cached re-run is byte-identical
# and that a no-`--log` pack is byte-deterministic.
#
# Usage: scripts/klz-smoke.sh [path/to/kapi]
set -euo pipefail

KAPI="${1:-bin/kapi}"
if [[ ! -x "$KAPI" ]]; then
  echo "klz-smoke: kapi binary not found at $KAPI (run 'make build' first)" >&2
  exit 1
fi
KAPI="$(cd "$(dirname "$KAPI")" && pwd)/$(basename "$KAPI")"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# Isolate plugins/caches from the developer environment (the project recipe
# is explicit via -p, so project discovery is fine).
export KAPI_PLUGINS_DIR_ONLY=1 NO_COLOR=1
export KAPI_CONFIG_DIR="$WORK/config" XDG_DATA_HOME="$WORK/data" XDG_CACHE_HOME="$WORK/cache"

mkproject() {
  local dir="$1"
  mkdir -p "$dir/.kapi"
  printf '{"greeting":"Hello world","farewell":"Goodbye now","cta":"Sign up today"}' > "$dir/app.json"
  cat > "$dir/demo.kapi" <<'EOF'
version: "v1"
name: demo
defaults:
  source_locale: en
  target_locales: [fr-FR]
flows:
  pseudo:
    steps:
      - tool: pseudo-translate
EOF
}

mkproject "$WORK/p1"
REC="$WORK/p1/demo.kapi"

echo "klz-smoke: project run (caches overlays)"
"$KAPI" run pseudo -p "$REC" -i "$WORK/p1/app.json" -o "$WORK/p1/out1.json" --target-lang fr-FR >/dev/null
[[ -f "$WORK/p1/.kapi/cache/blocks.db" ]] || { echo "FAIL: block store cache not created"; exit 1; }

echo "klz-smoke: cached re-run is byte-identical"
"$KAPI" run pseudo -p "$REC" -i "$WORK/p1/app.json" -o "$WORK/p1/out2.json" --target-lang fr-FR >/dev/null
diff -q "$WORK/p1/out1.json" "$WORK/p1/out2.json" >/dev/null || { echo "FAIL: cached re-run differs"; exit 1; }

echo "klz-smoke: pack is deterministic without --log"
"$KAPI" pack -p "$REC" -o "$WORK/a.klz" >/dev/null
"$KAPI" pack -p "$REC" -o "$WORK/b.klz" >/dev/null
diff -q "$WORK/a.klz" "$WORK/b.klz" >/dev/null || { echo "FAIL: packs of the same state differ"; exit 1; }

echo "klz-smoke: pack → unpack into a fresh project"
mkproject "$WORK/p2"
"$KAPI" unpack "$WORK/a.klz" -p "$WORK/p2/demo.kapi" >/dev/null
[[ -f "$WORK/p2/.kapi/cache/blocks.db" ]] || { echo "FAIL: unpack did not rehydrate the block store"; exit 1; }

echo "klz-smoke: OK (cached resume byte-identical; pack deterministic; pack/unpack round-trips)"
