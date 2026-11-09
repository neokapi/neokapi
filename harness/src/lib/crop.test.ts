import test from "node:test";
import assert from "node:assert/strict";
import { EDGE_OVERSCAN, FULL_WINDOW_FIT, MAX_CROP_SCALE, fitCeilings, frameGeometry, isWebDemo, panInsideWindow, regionFit, unionRect } from "./crop.ts";
import type { ZoomRect } from "../types.ts";

/** Compare a region to four numbers, past the noise of dividing by 1920. */
function assertRect(got: ZoomRect | null, want: [number, number, number, number]) {
  assert.ok(got, "expected a region");
  const keys = ["x", "y", "w", "h"] as const;
  keys.forEach((k, i) => assert.ok(Math.abs(got![k] - want[i]) < 1e-9, `${k}: ${got![k]} != ${want[i]}`));
}

const WIDTH = 1920;
const HEIGHT = 1080;
const native = { web: false, chapter: true, aspect: WIDTH / HEIGHT };
const web = { web: true, chapter: true, aspect: WIDTH / HEIGHT };

test("isWebDemo splits the browser demos from the native apps", () => {
  assert.equal(isWebDemo("bowrain-web-review"), true);
  assert.equal(isWebDemo("bowrain-desktop-automations"), false);
  assert.equal(isWebDemo("kapi-desktop-flows"), false);
});

test("height binds before width on both targets", () => {
  for (const frame of [native, web]) {
    const c = fitCeilings(frame);
    assert.ok(c.h < c.w, `expected height to bind, got w=${c.w} h=${c.h}`);
    assert.ok(c.h > 0.6 && c.h < 0.75, `height ceiling out of range: ${c.h}`);
  }
});

test("a region at the height ceiling crops and one just over it does not", () => {
  for (const frame of [native, web]) {
    const geo = frameGeometry(frame);
    const ceiling = fitCeilings(frame).h;
    assert.ok(regionFit({ x: 0, y: 0, w: 0.5, h: ceiling - 0.01 }, geo) >= FULL_WINDOW_FIT);
    assert.ok(regionFit({ x: 0, y: 0, w: 0.5, h: ceiling + 0.01 }, geo) < FULL_WINDOW_FIT);
  }
});

test("a full-window region never crops", () => {
  for (const frame of [native, web]) {
    assert.ok(regionFit({ x: 0, y: 0, w: 1, h: 1 }, frameGeometry(frame)) < FULL_WINDOW_FIT);
  }
});

test("unionRect pads a single box and keeps it inside the window", () => {
  assertRect(unionRect([{ x: 192, y: 108, width: 384, height: 216 }], 0.04, WIDTH, HEIGHT), [0.06, 0.06, 0.28, 0.28]);
});

test("unionRect clamps the padding at the window's edges", () => {
  assertRect(unionRect([{ x: 0, y: 0, width: WIDTH, height: HEIGHT }], 0.04, WIDTH, HEIGHT), [0, 0, 1, 1]);
});

test("unionRect drops an element scrolled off camera", () => {
  // kapi-desktop-content/extract: the gate cells sat below the viewport and
  // the union wrote h: -0.13 into screencast.json.
  const onCamera = { x: 192, y: 108, width: 384, height: 216 };
  const below = { x: 384, y: 1263, width: 260, height: 40 };
  assert.deepEqual(
    unionRect([onCamera, below], 0.04, WIDTH, HEIGHT),
    unionRect([onCamera], 0.04, WIDTH, HEIGHT),
  );
});

test("unionRect answers with no region when every element is off camera", () => {
  assert.equal(unionRect([{ x: 384, y: 1263, width: 260, height: 40 }], 0.04, WIDTH, HEIGHT), null);
  assert.equal(unionRect([{ x: -600, y: 100, width: 300, height: 40 }], 0.04, WIDTH, HEIGHT), null);
  assert.equal(unionRect([null, undefined], 0.04, WIDTH, HEIGHT), null);
});

test("unionRect clips a box that hangs over the bottom edge", () => {
  assertRect(unionRect([{ x: 0, y: 972, width: WIDTH, height: 500 }], 0, WIDTH, HEIGHT), [0, 0.9, 1, 0.1]);
});

test("unionRect never returns a region the composition cannot compose", () => {
  const cases: { x: number; y: number; width: number; height: number }[][] = [
    [{ x: 1919, y: 1079, width: 400, height: 400 }],
    [{ x: 0, y: 1080, width: 100, height: 100 }],
    [{ x: -100, y: -100, width: 100, height: 100 }],
    [{ x: 100, y: 100, width: 0, height: 0 }],
  ];
  for (const boxes of cases) {
    const r = unionRect(boxes, 0.04, WIDTH, HEIGHT);
    if (r === null) continue;
    assert.ok(r.w > 0 && r.h > 0, `non-positive region ${JSON.stringify(r)}`);
    assert.ok(r.x >= 0 && r.y >= 0 && r.x + r.w <= 1.0000001 && r.y + r.h <= 1.0000001);
  }
});

/** The pan that centres `region` in the crop area, as DesktopScene computes it, then clamped. */
function shot(region: ZoomRect, frame = native) {
  const geo = frameGeometry(frame);
  const s = Math.min(Math.max(regionFit(region, geo), 1), MAX_CROP_SCALE);
  const cx = region.x + region.w / 2;
  const cy = region.y + region.h / 2;
  const tx = geo.areaCx - geo.cardCx - s * (cx * geo.bw - geo.cardW / 2);
  const ty = geo.areaCy - geo.cardCy - s * (geo.bar + cy * geo.bh - geo.cardH / 2);
  const pan = panInsideWindow(region, s, tx, ty, geo);
  // A window point, in the box's pixels.
  const at = (u: number, v: number) => ({
    x: geo.cardCx + pan.tx + s * (u * geo.bw - geo.cardW / 2),
    y: geo.cardCy + pan.ty + s * (geo.bar + v * geo.bh - geo.cardH / 2),
  });
  return { geo, centred: { tx, ty }, pan, at };
}

test("a crop at the window's left edge pans so the window still fills the box", () => {
  // kapi-desktop-projects/add-collection: centring this region drew the
  // backdrop beside the window's left edge.
  const region = { x: 0.02, y: 0.5, w: 0.5, h: 0.45 };
  const { geo, centred, pan, at } = shot(region);
  assert.ok(centred.tx > pan.tx, "the pan should move the window left");
  assert.ok(at(0, 0).x <= -EDGE_OVERSCAN * geo.boxW + 1e-6, `left edge at ${at(0, 0).x}`);
  assert.ok(at(1, 0).x >= geo.boxW, "the right edge stays past the box");
  assert.ok(at(region.x, 0).x >= -1e-6 && at(region.x + region.w, 0).x <= geo.areaW + 1e-6);
  assert.ok(at(0, region.y).y >= -1e-6 && at(0, region.y + region.h).y <= geo.areaH + 1e-6);
});

test("a crop in the window's middle keeps its centred pan", () => {
  const { centred, pan } = shot({ x: 0.4, y: 0.35, w: 0.2, h: 0.2 });
  assert.ok(Math.abs(centred.tx - pan.tx) < 1e-9 && Math.abs(centred.ty - pan.ty) < 1e-9);
});

test("a crop at the window's bottom keeps the region above the captions", () => {
  const region = { x: 0.2, y: 0.88, w: 0.6, h: 0.1 };
  const { geo, at } = shot(region);
  assert.ok(at(0, region.y + region.h).y <= geo.areaH + 1e-6, "the region's bottom stays above the caption band");
  assert.ok(at(0, 0).y <= 0, "the window's top stays past the box's top");
});
