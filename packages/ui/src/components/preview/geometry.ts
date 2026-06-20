// Shared coordinate math for the spatial views (LayoutView, MediaCanvas /
// OCROverlay). A block's GeometryView carries a bounding box in a coordinate
// space named by `resolution` (a normalized grid edge, e.g. DocLang's 512; 0 =
// absolute units) and `origin` ("top-left" default, or "bottom-left"). Mapping a
// box onto a rendered surface is the same operation everywhere — placing a box on
// a scaled page, or overlaying it on a raster — so the math lives here once.

import type { CSSProperties } from "react";
import type { ContentNode, ContentTree, GeometryView } from "./types";

/** A block carrying geometry, with its document reading-order index. */
export interface PlacedBlock {
  node: ContentNode;
  g: GeometryView;
  /** 1-based reading-order index across all blocks (placed or not). */
  order: number;
}

/**
 * Walk the tree in document order, splitting blocks into those with page
 * geometry and those without. The reading-order index counts every block (so a
 * placed block keeps its true position even when neighbors are unplaced).
 */
export function flattenGeometry(tree: ContentTree): {
  placed: PlacedBlock[];
  unplaced: ContentNode[];
} {
  const placed: PlacedBlock[] = [];
  const unplaced: ContentNode[] = [];
  let order = 0;
  const walk = (n: ContentNode) => {
    if (n.kind === "block") {
      order += 1;
      if (n.geometry) placed.push({ node: n, g: n.geometry, order });
      else unplaced.push(n);
    }
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return { placed, unplaced };
}

/** A box positioned as percentages of the coordinate extent it lives in. */
export interface BoxPercent {
  left: number;
  top: number;
  width: number;
  height: number;
}

/** The minimal box shape both blocks and glyphs satisfy. */
export interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
  origin?: string;
}

/**
 * topUnits flips a box's y to a top-down coordinate when it uses a bottom-left
 * origin (PDF-style), so every surface can render top-down. extent is the height
 * of the coordinate space (the resolution grid, or the surface's natural height).
 */
export function topUnits(
  origin: string | undefined,
  y: number,
  h: number,
  extentH: number,
): number {
  return origin === "bottom-left" ? extentH - y - h : y;
}

/**
 * boxPercent maps a box to %-of-extent positioning, flipping Y for a bottom-left
 * origin. Percentages (not pixels) keep the overlay correct as the surface
 * scales — the same approach LayoutView uses for its scaled pages.
 */
export function boxPercent(box: Box, extentW: number, extentH: number): BoxPercent {
  return {
    left: (box.x / extentW) * 100,
    top: (topUnits(box.origin, box.y, box.h, extentH) / extentH) * 100,
    width: (box.w / extentW) * 100,
    height: (box.h / extentH) * 100,
  };
}

/**
 * extentOf resolves the coordinate extent for a set of geometry boxes: the
 * normalized grid when a resolution is declared (DocLang = 512), else a fallback
 * (the natural surface size, or the max box extent + a small margin). When a
 * surface's natural pixel size is known (an image's naturalWidth/Height), pass it
 * as the fallback so absolute-pixel boxes map exactly onto the raster.
 */
export function extentOf(
  boxes: { g: GeometryView }[],
  fallbackW?: number,
  fallbackH?: number,
): { extentW: number; extentH: number } {
  const res = boxes.find((b) => b.g.resolution && b.g.resolution > 0)?.g.resolution ?? 0;
  if (res > 0) return { extentW: res, extentH: res };
  if (fallbackW && fallbackH && fallbackW > 0 && fallbackH > 0) {
    return { extentW: fallbackW, extentH: fallbackH };
  }
  let maxX = 0;
  let maxY = 0;
  for (const b of boxes) {
    maxX = Math.max(maxX, b.g.x + b.g.w);
    maxY = Math.max(maxY, b.g.y + b.g.h);
  }
  return { extentW: maxX > 0 ? maxX * 1.04 : 1, extentH: maxY > 0 ? maxY * 1.04 : 1 };
}

/** A CSS-percent style object for a box; convenience over boxPercent. */
export function boxStyle(box: Box, extentW: number, extentH: number): CSSProperties {
  const p = boxPercent(box, extentW, extentH);
  return {
    left: `${p.left}%`,
    top: `${p.top}%`,
    width: `${p.width}%`,
    height: `${p.height}%`,
  };
}
