/**
 * The crop geometry the recorder and the renderer share.
 *
 * A desktop beat names a region of the recorded window and the composition
 * pushes the camera into it. Whether that push happens at all is decided by one
 * number, the region's *fit*: the scale at which the region fills the crop area
 * above the captions. `DesktopScene` shows the whole window when the fit is
 * under `FULL_WINDOW_FIT`, so a region close to the window's own size is drawn
 * at 1x with no translation, and the beat lands in the middle of the window
 * rather than on the thing its caption names.
 *
 * The recorder resolves selector crops against the live page, hours before
 * anything is rendered, so it computes the same fit here and says so while the
 * take is still cheap to redo.
 *
 * The crop area is wider than it is tall, so height is what binds: a region
 * taller than roughly two thirds of the window can never be cropped into,
 * whatever its width. `fitCeilings` states the two limits for a demo.
 */
import { FRAME_H, FRAME_W, sceneLayout } from "../remotion/components/layout.ts";
import type { SceneLayout } from "../remotion/components/layout.ts";
import type { ZoomRect } from "../types.ts";

/** Furthest a crop pushes into the window on its own; a beat's `zoom` can take it to 3. */
export const MAX_CROP_SCALE = 2.5;

/** A region whose fit is under this is shown as the whole window. */
export const FULL_WINDOW_FIT = 1.15;

/** A full-window card fits this much of the picture box, so its tilt never clips. */
export const CARD_INSET = 0.96;

/**
 * Web demos capture a browser SPA with no window chrome, so the composition
 * frames them inside a browser top bar and the capture sits below it. Native
 * app demos keep their own title-bar gutter and get the dots overlay instead.
 */
export function isWebDemo(demoId: string): boolean {
  return demoId.startsWith("bowrain-web");
}

/** Where the window card sits inside a scene's picture box, in the box's own pixels. */
export interface CardGeometry {
  /** The capture's drawn width and height. */
  bw: number;
  bh: number;
  /** The browser bar above the capture (0 for a native app). */
  bar: number;
  /** The whole card, capture plus bar. */
  cardW: number;
  cardH: number;
  cardCx: number;
  cardCy: number;
  /** The picture box the card is drawn in, which clips it. */
  boxW: number;
  boxH: number;
  /** The area a crop is fitted into: the picture box down to the caption band. */
  areaW: number;
  areaH: number;
  areaCx: number;
  areaCy: number;
}

/** The card's geometry for one scene, from the frame grid and the capture's aspect. */
export function cardGeometry(layout: SceneLayout, aspect: number, web: boolean): CardGeometry {
  const bar = web ? 44 : 0;
  const boxW = layout.width;
  const boxH = layout.picBottom - layout.picTop;
  let bw = boxW * CARD_INSET;
  let bh = bw / aspect;
  if (bh + bar > boxH * CARD_INSET) {
    bh = boxH * CARD_INSET - bar;
    bw = bh * aspect;
  }
  const cardW = bw;
  const cardH = bh + bar;
  const areaH = layout.stackBottom - layout.picTop;
  return {
    bw,
    bh,
    bar,
    cardW,
    cardH,
    cardCx: (boxW - cardW) / 2 + cardW / 2,
    cardCy: (boxH - cardH) / 2 + cardH / 2,
    boxW,
    boxH,
    areaW: boxW,
    areaH,
    areaCx: boxW / 2,
    areaCy: areaH / 2,
  };
}

/** The scale at which `region` fills the crop area. Below `FULL_WINDOW_FIT` the shot is the whole window. */
export function regionFit(region: ZoomRect, geo: CardGeometry): number {
  return Math.min(geo.areaW / (region.w * geo.bw), geo.areaH / (region.h * geo.bh));
}

/**
 * How far past the picture box's edges the pushed-in window is kept, as a
 * share of the box, so the camera's tilt and breathing drift never open a gap
 * at an edge.
 */
export const EDGE_OVERSCAN = 0.02;

/**
 * The pan of a cropped shot, moved as little as needed so the window still
 * fills the picture box.
 *
 * A shot centres its region in the crop area. A region near the window's edge
 * then pulls the window's edge into the frame, with the backdrop beside it,
 * so the pan is clamped to keep each edge of the window past the edge of the
 * box. The region itself stays in the crop area: the clamp never moves it out
 * of the space above the captions, and where both cannot hold, the region
 * wins. Pans are in the box's pixels, as `DesktopScene` applies them about the
 * card's centre.
 */
export function panInsideWindow(region: ZoomRect, s: number, tx: number, ty: number, geo: CardGeometry): { tx: number; ty: number } {
  const mx = EDGE_OVERSCAN * geo.boxW;
  const my = EDGE_OVERSCAN * geo.boxH;
  // Where a window point lands in the box, with no pan: x = cardCx + tx + s * (u * bw - cardW / 2).
  const x0 = geo.cardCx - (s * geo.cardW) / 2; // the window's left edge
  const x1 = geo.cardCx + s * (geo.bw - geo.cardW / 2); // its right edge
  const y0 = geo.cardCy - (s * geo.cardH) / 2; // the card's top (bar included)
  const y1 = geo.cardCy + s * (geo.bar + geo.bh - geo.cardH / 2); // its bottom
  const within = (v: number, a: number, b: number) => Math.max(Math.min(a, b), Math.min(Math.max(a, b), v));
  // Keep the window's edges past the box's: left edge at or before -mx, right at or after boxW + mx.
  let px = within(tx, -mx - x0, geo.boxW + mx - x1);
  let py = within(ty, -my - y0, geo.boxH + my - y1);
  // The region stays inside the crop area, which comes first.
  const rx0 = geo.cardCx + s * (region.x * geo.bw - geo.cardW / 2);
  const rx1 = geo.cardCx + s * ((region.x + region.w) * geo.bw - geo.cardW / 2);
  const ry0 = geo.cardCy + s * (geo.bar + region.y * geo.bh - geo.cardH / 2);
  const ry1 = geo.cardCy + s * (geo.bar + (region.y + region.h) * geo.bh - geo.cardH / 2);
  px = within(px, -rx0, geo.areaW - rx1);
  py = within(py, -ry0, geo.areaH - ry1);
  return { tx: px, ty: py };
}

/** How a demo's beats are framed: the geometry every scene of it shares. */
export interface DemoFrame {
  web: boolean;
  /** With a chapter line the picture box starts lower, so the crop area is shorter. */
  chapter: boolean;
  aspect: number;
}

/** The card geometry for a beat of `frame`. */
export function frameGeometry(frame: DemoFrame): CardGeometry {
  return cardGeometry(sceneLayout(frame.chapter), frame.aspect, frame.web);
}

/**
 * The widest and tallest a region may be, as a fraction of the window, and
 * still be cropped into. Height is the binding one on every demo.
 */
export function fitCeilings(frame: DemoFrame): { w: number; h: number } {
  const geo = frameGeometry(frame);
  return {
    w: geo.areaW / geo.bw / FULL_WINDOW_FIT,
    h: geo.areaH / geo.bh / FULL_WINDOW_FIT,
  };
}

/** An element's box on the page, in capture pixels, as Playwright reports it. */
export interface ElementBox {
  x: number;
  y: number;
  width: number;
  height: number;
}

/**
 * The union of the parts of `boxes` that are on camera, padded, as a region in
 * [0,1] window coordinates.
 *
 * Every box is clipped to the capture first. An element scrolled out of the
 * viewport still has a box, with coordinates past the window's own edges, and
 * folding one of those into the union used to produce a region with a negative
 * height: `kapi-desktop-content/extract` wrote `h: -0.13` into its
 * screencast.json, which no shot can be composed from. A box with nothing on
 * camera is left out, and a union with nothing left in it is no region at all.
 */
export function unionRect(
  boxes: readonly (ElementBox | null | undefined)[],
  pad: number,
  width = FRAME_W,
  height = FRAME_H,
): ZoomRect | null {
  let x0 = Infinity;
  let y0 = Infinity;
  let x1 = -Infinity;
  let y1 = -Infinity;
  let any = false;
  for (const box of boxes) {
    if (!box) continue;
    const bx0 = Math.max(0, Math.min(width, box.x));
    const by0 = Math.max(0, Math.min(height, box.y));
    const bx1 = Math.max(0, Math.min(width, box.x + box.width));
    const by1 = Math.max(0, Math.min(height, box.y + box.height));
    if (bx1 <= bx0 || by1 <= by0) continue; // wholly off camera
    any = true;
    x0 = Math.min(x0, bx0);
    y0 = Math.min(y0, by0);
    x1 = Math.max(x1, bx1);
    y1 = Math.max(y1, by1);
  }
  if (!any) return null;
  const x = Math.max(0, x0 / width - pad);
  const y = Math.max(0, y0 / height - pad);
  const w = Math.min(1 - x, (x1 - x0) / width + 2 * pad);
  const h = Math.min(1 - y, (y1 - y0) / height + 2 * pad);
  if (w <= 0 || h <= 0) return null;
  return { x, y, w, h };
}
