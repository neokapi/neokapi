// The squarified treemap layout (Bruls, Huizing and van Wijk, 2000), kept free
// of React so it can be tested on its own. The coordinates map lays out its
// products with it, then the channels inside each product.

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type Tile<T> = Rect & { item: T };

/** The worst aspect ratio of a row of areas laid along a side of this length. */
function worst(areas: number[], side: number): number {
  let sum = 0;
  let max = 0;
  let min = Infinity;
  for (const a of areas) {
    sum += a;
    if (a > max) max = a;
    if (a < min) min = a;
  }
  const s2 = side * side;
  const sum2 = sum * sum;
  return Math.max((s2 * max) / sum2, sum2 / (s2 * min));
}

/**
 * Lay the items out in the rect, each with an area proportional to its value.
 * The tiles cover the rect without overlapping. They come back largest first,
 * equal values in the order given, so the same input always draws the same
 * map. Items with a value of zero or less take no area and are left out.
 */
export function squarify<T>(
  items: readonly T[],
  value: (item: T) => number,
  rect: Rect,
): Tile<T>[] {
  const entries = items
    .map((item, i) => ({ item, i, v: value(item) }))
    .filter((e) => Number.isFinite(e.v) && e.v > 0)
    .sort((a, b) => b.v - a.v || a.i - b.i);
  if (entries.length === 0 || rect.w <= 0 || rect.h <= 0) return [];
  const total = entries.reduce((s, e) => s + e.v, 0);
  const scale = (rect.w * rect.h) / total;
  const out: Tile<T>[] = [];
  let { x, y, w, h } = rect;
  let row: { item: T; a: number }[] = [];

  // Place the row along the shorter side of what is left. The last row takes
  // all of what is left, so rounding never leaves a sliver uncovered.
  const place = (last: boolean) => {
    const sum = row.reduce((s, r) => s + r.a, 0);
    if (w >= h) {
      const cw = last ? w : sum / h;
      let cy = y;
      row.forEach((r, k) => {
        const rh = k === row.length - 1 ? y + h - cy : r.a / cw;
        out.push({ item: r.item, x, y: cy, w: cw, h: rh });
        cy += rh;
      });
      x += cw;
      w -= cw;
    } else {
      const rh = last ? h : sum / w;
      let cx = x;
      row.forEach((r, k) => {
        const rw = k === row.length - 1 ? x + w - cx : r.a / rh;
        out.push({ item: r.item, x: cx, y, w: rw, h: rh });
        cx += rw;
      });
      y += rh;
      h -= rh;
    }
    row = [];
  };

  for (const e of entries) {
    const a = e.v * scale;
    const side = Math.min(w, h);
    const areas = row.map((r) => r.a);
    if (row.length > 0 && worst([...areas, a], side) > worst(areas, side)) place(false);
    row.push({ item: e.item, a });
  }
  place(true);
  return out;
}

/** Shrink a rect by the inset on every side, never below zero. */
export function inset(r: Rect, by: number): Rect {
  return {
    x: r.x + Math.min(by, r.w / 2),
    y: r.y + Math.min(by, r.h / 2),
    w: Math.max(0, r.w - 2 * by),
    h: Math.max(0, r.h - 2 * by),
  };
}

/** The step, 0 to steps - 1, that a share from 0 to 1 falls in. */
export function shadeStep(share: number, steps: number): number {
  const s = Math.max(0, Math.min(1, Number.isFinite(share) ? share : 0));
  return Math.min(steps - 1, Math.floor(s * steps));
}

function parseHex(hex: string): [number, number, number] | null {
  const m = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!m) return null;
  const n = parseInt(m[1], 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

/** The colour t of the way from a to b (both #rrggbb); a when either fails to parse. */
export function mixHex(a: string, b: string, t: number): string {
  const ca = parseHex(a);
  const cb = parseHex(b);
  if (!ca || !cb) return a;
  const k = Math.max(0, Math.min(1, t));
  const c = ca.map((v, i) => Math.round(v + (cb[i] - v) * k));
  return `#${c.map((v) => v.toString(16).padStart(2, "0")).join("")}`;
}
