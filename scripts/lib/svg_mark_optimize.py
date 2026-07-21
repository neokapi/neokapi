#!/usr/bin/env python3
"""Reframe + optimize a raw bowrain mark export into the compact SVG the
asset pipeline expects (scripts/generate-bowrain-brand-assets.sh).

Usage: svg_mark_optimize.py <input.svg> <output.svg> [--keep-glow]

Passes:
  1. Reframe: fit-to-contain scale + center into viewBox="0 0 64 64" (the
     pipeline's OG-card build step inlines this mark assuming that box).
  2. svgo: strip editor metadata, minify precision/whitespace/ids.
  3. Glow filter: Inkscape's "Soft Focus Lens" filter (applied per shape,
     ~800 near-identical copies) blends via `mode="screen"`, which biases
     toward white regardless of the source color. That's invisible against
     the white artboard it was designed on, but shows as a stark white halo
     around every shape's edge (and every internal facet seam) on a dark
     background. Bowrain is dual-theme, so by default this is STRIPPED
     entirely, not merged -- a light-only glow is a defect, not a style
     choice, for a mark that has to render on dark backgrounds too.
     Pass --keep-glow to preserve it instead (merges the ~800 near-identical
     filters into one shared filter using the widest observed
     objectBoundingBox clip region -- safe because a larger region never
     crops the effect, it only changes how much gets computed). Merge is
     skipped with a warning if the effect chains aren't byte-identical, so a
     genuinely different effect is never silently dropped.
"""
import re
import subprocess
import sys
import tempfile
from pathlib import Path

TARGET = 64.0
SVGO_VERSION = "svgo@4.0.2"


def reframe(data):
    m = re.search(r'viewBox="([\d.\-]+)\s+([\d.\-]+)\s+([\d.\-]+)\s+([\d.\-]+)"', data)
    if not m:
        raise SystemExit("no viewBox found in source SVG")
    vx, vy, vw, vh = (float(g) for g in m.groups())

    scale = min(TARGET / vw, TARGET / vh)
    scaled_w, scaled_h = vw * scale, vh * scale
    tx = (TARGET - scaled_w) / 2 - vx * scale
    ty = (TARGET - scaled_h) / 2 - vy * scale

    open_tag_end = data.index(">", data.index("<svg")) + 1
    close_tag_start = data.rindex("</svg>")
    inner = data[open_tag_end:close_tag_start]

    print(f"  reframe: viewBox {vw:.0f}x{vh:.0f} -> scale {scale:.6f}, "
          f"translate ({tx:.3f},{ty:.3f})", file=sys.stderr)

    return (
        '<svg xmlns="http://www.w3.org/2000/svg" '
        'xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" '
        'xmlns:sodipodi="http://sodipodi.sourceforge.net/DTD/sodipodi-0.dtd" '
        'viewBox="0 0 64 64" fill="none">\n'
        f'  <g transform="translate({tx:.6f},{ty:.6f}) scale({scale:.6f})">\n'
        f'{inner}\n  </g>\n</svg>\n'
    )


def run_svgo(data, tmpdir):
    src = tmpdir / "pre-svgo.svg"
    dst = tmpdir / "post-svgo.svg"
    src.write_text(data, encoding="utf-8")
    subprocess.run(
        ["npx", "--yes", SVGO_VERSION, "--multipass", "-p", "3", "-i", str(src), "-o", str(dst)],
        check=True, capture_output=True, text=True,
    )
    return dst.read_text(encoding="utf-8")


def strip_filters(data):
    filters = re.findall(r'<filter id="([^"]*)"', data)
    if not filters:
        return data

    data, n_defs = re.subn(r'<defs>.*?</defs>', "", data, count=1, flags=re.S)
    data, n_refs = re.subn(r'\s*style="filter:url\(#[^)]*\)"', "", data)
    print(f"  glow filter: STRIPPED ({len(filters)} per-shape 'soft focus' filters removed, "
          f"{n_refs} refs cleared -- screen-blend glow halos white on dark backgrounds; "
          f"pass --keep-glow to preserve it instead)", file=sys.stderr)
    return data


def merge_filters(data, shared_id="glow"):
    filters = re.findall(r'<filter id="([^"]*)"[^>]*>(.*?)</filter>', data, re.S)
    if not filters:
        return data  # nothing to merge

    chains = set(f[1] for f in filters)
    if len(chains) != 1:
        print(f"  filter merge: SKIPPED ({len(chains)} distinct effect chains "
              f"across {len(filters)} filters -- not safe to merge)", file=sys.stderr)
        return data
    chain = next(iter(chains))

    regions = re.findall(
        r'<filter id="[^"]*" width="([^"]*)" height="([^"]*)" x="([^"]*)" y="([^"]*)"', data
    )
    if len(regions) != len(filters):
        print("  filter merge: SKIPPED (couldn't parse every filter's region)", file=sys.stderr)
        return data
    ws = [float(r[0]) for r in regions]
    hs = [float(r[1]) for r in regions]
    xs = [float(r[2]) for r in regions]
    ys = [float(r[3]) for r in regions]
    x, y = min(xs) - 0.5, min(ys) - 0.5
    w, h = max(ws) + 1.0, max(hs) + 1.0

    shared_filter = (
        f'<filter id="{shared_id}" width="{w:.3f}" height="{h:.3f}" '
        f'x="{x:.3f}" y="{y:.3f}" style="color-interpolation-filters:sRGB">{chain}</filter>'
    )
    data, n_defs = re.subn(r'<defs>.*?</defs>', f'<defs>{shared_filter}</defs>', data, count=1, flags=re.S)
    if n_defs != 1:
        print("  filter merge: SKIPPED (expected exactly one <defs> block)", file=sys.stderr)
        return data
    data, n_refs = re.subn(r'filter:url\(#[^)]*\)', f'filter:url(#{shared_id})', data)
    print(f"  filter merge: {len(filters)} filters -> 1 shared "
          f"(region x={x:.2f} y={y:.2f} w={w:.2f} h={h:.2f}), {n_refs} refs repointed",
          file=sys.stderr)
    return data


def main():
    args = sys.argv[1:]
    keep_glow = "--keep-glow" in args
    args = [a for a in args if a != "--keep-glow"]
    if len(args) != 2:
        raise SystemExit(f"usage: {sys.argv[0]} <input.svg> <output.svg> [--keep-glow]")
    src_path, dst_path = Path(args[0]), Path(args[1])
    data = src_path.read_text(encoding="utf-8")

    print(f"optimizing {src_path} -> {dst_path}", file=sys.stderr)
    data = reframe(data)
    with tempfile.TemporaryDirectory() as td:
        tmpdir = Path(td)
        data = run_svgo(data, tmpdir)
        data = merge_filters(data) if keep_glow else strip_filters(data)
        data = run_svgo(data, tmpdir)  # final cleanup pass

    before = src_path.stat().st_size
    dst_path.write_text(data, encoding="utf-8")
    after = dst_path.stat().st_size
    print(f"  {before:,} -> {after:,} bytes ({100 * (1 - after / before):.0f}% smaller)", file=sys.stderr)


if __name__ == "__main__":
    main()
