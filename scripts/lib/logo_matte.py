#!/usr/bin/env python3
"""Recover a transparent logo from two opaque renders over known backgrounds.

Image generators (Gemini, etc.) hand back a logo as two flat, fully-opaque
images: one composited over solid black, one over solid white. Given both, the
true straight-alpha matte is exact algebra -- no model, no heuristics:

    obs_black =      a * fg
    obs_white =      a * fg + (1 - a) * 255
  =>      a  = 1 - (obs_white - obs_black) / 255
  =>     fg  = obs_black / a                      (where a > 0)

Because it is pure per-pixel arithmetic, re-running it on an updated logo pair
produces the identical transform every time -- the whole point of scripting it.

It also strips the generator's corner watermark (e.g. the Gemini sparkle). That
mark sits in a fixed bottom-right corner over pure background, so after matting
it survives only as a faint semi-opaque ghost. We clear a corner box. A safety
check refuses to clear the box if it overlaps *opaque* logo content, so a future
logo that reaches into that corner fails loudly instead of being silently
cropped.

Usage:
    logo_matte.py BLACK WHITE OUT [--corner F] [--floor F] [--no-watermark-erase]
"""

from __future__ import annotations

import argparse
import sys

import numpy as np
from PIL import Image


def extract(
    black: np.ndarray,
    white: np.ndarray,
    *,
    corner: float,
    floor: float,
    erase_watermark: bool,
) -> np.ndarray:
    """Return an RGBA uint8 array recovered from the black/white-bg renders."""
    if black.shape != white.shape:
        sys.exit(f"size mismatch: black {black.shape} vs white {white.shape}")
    height, width, _ = black.shape

    # The white render must be >= the black render everywhere (more background
    # light leaks through where alpha is lower). Clip guards against noise.
    diff = np.clip(white - black, 0.0, 255.0)
    alpha = np.clip(1.0 - diff.mean(axis=2) / 255.0, 0.0, 1.0)

    # Pure background carries a hair of compression noise -> force it transparent.
    alpha[alpha < floor] = 0.0

    # Straight (un-premultiplied) foreground colour. Dividing by alpha undoes the
    # composite, recovering true edge colour -> clean anti-aliasing, no dark halo.
    safe = np.maximum(alpha, 1e-6)
    fg = np.clip(black / safe[..., None], 0.0, 255.0)

    if erase_watermark:
        x0, y0 = int(width * corner), int(height * corner)
        opaque = int((alpha[y0:, x0:] > 0.9).sum())
        if opaque > 50:
            sys.exit(
                f"refusing to erase watermark corner [x>={x0}, y>={y0}]: it "
                f"overlaps {opaque} opaque logo pixels (alpha>0.9). The logo now "
                f"reaches into the corner -- lower --corner or inspect the source."
            )
        alpha[y0:, x0:] = 0.0

    rgba = np.empty((height, width, 4), dtype=np.uint8)
    rgba[..., :3] = np.rint(fg).astype(np.uint8)
    rgba[..., 3] = np.rint(alpha * 255.0).astype(np.uint8)
    # Zero RGB under fully-transparent pixels: smaller, deterministic PNG output.
    rgba[..., :3][rgba[..., 3] == 0] = 0
    return rgba


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("black", help="logo rendered over a solid BLACK background")
    ap.add_argument("white", help="logo rendered over a solid WHITE background")
    ap.add_argument("out", help="output transparent RGBA PNG")
    ap.add_argument(
        "--corner",
        type=float,
        default=0.85,
        help="erase the watermark in the bottom-right [corner..1.0] box, as a "
        "fraction of each side (default: 0.85)",
    )
    ap.add_argument(
        "--floor",
        type=float,
        default=0.04,
        help="recovered alpha below this is forced to 0, killing background "
        "compression noise (default: 0.04)",
    )
    ap.add_argument(
        "--no-watermark-erase",
        action="store_true",
        help="keep the generator watermark (skip the corner clear)",
    )
    args = ap.parse_args()

    black = np.asarray(Image.open(args.black).convert("RGB"), dtype=np.float64)
    white = np.asarray(Image.open(args.white).convert("RGB"), dtype=np.float64)

    rgba = extract(
        black,
        white,
        corner=args.corner,
        floor=args.floor,
        erase_watermark=not args.no_watermark_erase,
    )
    Image.fromarray(rgba, "RGBA").save(args.out)

    height, width, _ = rgba.shape
    coverage = (rgba[..., 3] > 0).mean() * 100.0
    erase = "off" if args.no_watermark_erase else f">={args.corner:.0%}"
    print(f"matte -> {args.out}  {width}x{height}  coverage {coverage:.1f}%  watermark-erase {erase}")


if __name__ == "__main__":
    main()
