import test from "node:test";
import assert from "node:assert/strict";
import { FULL_WINDOW_FIT, fitCeilings, frameGeometry, isWebDemo, regionFit, unionRect } from "./crop.ts";
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
