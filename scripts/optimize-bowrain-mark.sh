#!/usr/bin/env bash
#
# Reframe + optimize a raw bowrain mark export (Inkscape or similar) into the
# compact 64x64-viewBox SVG the asset pipeline expects. Run this whenever you
# edit the source vector, then regenerate everything derived from it:
#
#   scripts/optimize-bowrain-mark.sh <full-export.svg>  bowrain/assets/brand/mark.svg
#   scripts/optimize-bowrain-mark.sh <plain-export.svg> bowrain/assets/brand/mark-favicon.svg
#   make bowrain-logo
#
# What it does (see scripts/lib/svg_mark_optimize.py):
#   1. fit-to-contain reframe into viewBox="0 0 64 64"
#   2. svgo cleanup (strip editor metadata, minify precision/ids)
#   3. Inkscape's "Soft Focus Lens" filter (applied per shape) is STRIPPED by
#      default -- it blends via mode="screen", which halos white on dark
#      backgrounds (fine on the white artboard it was designed on, broken on
#      Bowrain's dual-theme dark mode). Pass --keep-glow to instead merge the
#      ~800 near-identical per-shape filters into one shared filter (only
#      if every filter's effect chain is identical; skipped with a warning
#      otherwise -- never silently drops a differing effect).
#
# The output has no header comment describing what it is (full mark vs
# favicon-scale companion) -- add one by hand after the first run, or copy it
# forward from the previous version of the file before overwriting.
#
# Requires: python3, node (npx svgo, fetched on first use).
set -euo pipefail
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[[ $# -eq 2 || $# -eq 3 ]] || { echo "usage: $0 <input.svg> <output.svg> [--keep-glow]" >&2; exit 1; }
python3 "$REPO_ROOT/scripts/lib/svg_mark_optimize.py" "$@"
