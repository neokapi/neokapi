/**
 * The camera of a desktop walkthrough beat, in the manner of a screen recording
 * editor: the whole window, a zoom onto the region the beat acts on, and back.
 * Pure functions of the frame, so the renderer and the tests agree.
 */
import { FULL_WINDOW_FIT, MAX_CROP_SCALE, panInsideWindow, regionFit } from "../lib/crop.ts";
import type { CardGeometry } from "../lib/crop.ts";
import type { ZoomRect } from "../types.ts";

const clamp = (v: number, lo: number, hi: number) => Math.max(lo, Math.min(hi, v));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

// Camera pose: how far the window card is pushed in, and where it is panned.
export interface Cam {
  s: number; // zoom (scale)
  tx: number; // pan x (px)
  ty: number; // pan y (px)
}

export const WIDE: Cam = { s: 1, tx: 0, ty: 0 };

/**
 * The zoomed shot for a region, or null when the region is close to the whole
 * window and the beat stays on the wide shot. The scale fits the region into
 * the crop area (above the captions) with a margin (`ZOOM_FILL`), times the beat's own
 * `zoom`, and the region's centre goes to the area's centre, or as near it as
 * the window's edges allow (`panInsideWindow`).
 */
export function zoomPose(region: ZoomRect | null | undefined, zoomMul: number, geo: CardGeometry): Cam | null {
  if (!region) return null;
  const s = clamp(regionFit(region, geo) * zoomMul, 1, MAX_CROP_SCALE);
  if (s < FULL_WINDOW_FIT) return null;
  const cx = region.x + region.w / 2;
  const cy = region.y + region.h / 2;
  const ox = cx * geo.bw - geo.cardW / 2;
  const oy = geo.bar + cy * geo.bh - geo.cardH / 2;
  const { tx, ty } = panInsideWindow(region, s, geo.areaCx - geo.cardCx - s * ox, geo.areaCy - geo.cardCy - s * oy, geo);
  return { s, tx, ty };
}

/**
 * Whether the camera goes straight from one zoomed shot to the next instead of
 * pulling back to the wide shot between them: both are zoomed, at similar
 * scales, and near each other, so a pan reads as following the action.
 */
export function staysZoomed(a: Cam | null, b: Cam | null, geo: CardGeometry): boolean {
  if (!a || !b) return false;
  if (Math.abs(Math.log(a.s / b.s)) > 0.35) return false;
  return Math.hypot(a.tx - b.tx, a.ty - b.ty) < 0.3 * geo.boxW;
}

const easeInOut = (t: number) => {
  t = clamp(t, 0, 1);
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
};

const mix = (a: Cam, b: Cam, t: number): Cam => ({ s: lerp(a.s, b.s, t), tx: lerp(a.tx, b.tx, t), ty: lerp(a.ty, b.ty, t) });

/**
 * The camera for one frame of a beat, in the manner of a screen recording
 * editor: the beat opens on the whole window, eases in on its region, holds,
 * and eases back out before it ends. Where the neighbouring beat's shot is
 * close (`staysZoomed`), the camera pans between the two instead.
 */
export function cameraAt(frame: number, durationFrames: number, fps: number, shot: Cam | null, prev: Cam | null, next: Cam | null, geo: CardGeometry): Cam {
  if (!shot) return WIDE;
  const fromPrev = staysZoomed(prev, shot, geo);
  const intoNext = staysZoomed(shot, next, geo);
  let delay = fromPrev ? 0 : 0.35 * fps;
  let zoomIn = (fromPrev ? 0.7 : 1.0) * fps;
  let zoomOut = intoNext ? 0 : 0.8 * fps;
  // A short beat compresses its moves rather than overlapping them, and keeps
  // at least a third of its length on the zoomed shot.
  const budget = durationFrames * 0.66;
  const moves = delay + zoomIn + zoomOut;
  if (moves > budget) {
    const k = budget / moves;
    delay *= k;
    zoomIn *= k;
    zoomOut *= k;
  }
  const start = fromPrev && prev ? prev : WIDE;
  if (frame < delay) return start;
  if (frame < delay + zoomIn) return mix(start, shot, easeInOut((frame - delay) / zoomIn));
  if (zoomOut > 0 && frame > durationFrames - zoomOut) return mix(shot, WIDE, easeInOut((frame - (durationFrames - zoomOut)) / zoomOut));
  return shot;
}
