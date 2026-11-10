import { test } from "node:test";
import assert from "node:assert/strict";
import { cameraAt, staysZoomed, WIDE, zoomPose } from "./camera.ts";
import { frameGeometry } from "../lib/crop.ts";

const geo = frameGeometry({ web: false, chapter: true, aspect: 16 / 9 });
const fps = 30;
const small = { x: 0.1, y: 0.1, w: 0.3, h: 0.2 };
const nearby = { x: 0.15, y: 0.12, w: 0.3, h: 0.2 };
const far = { x: 0.6, y: 0.65, w: 0.3, h: 0.2 };

test("a region close to the whole window stays on the wide shot", () => {
  assert.equal(zoomPose({ x: 0, y: 0, w: 0.95, h: 0.9 }, 1, geo), null);
  assert.deepEqual(cameraAt(60, 300, fps, null, null, null, geo), WIDE);
});

test("a beat opens wide, zooms in on its region, and ends wide", () => {
  const shot = zoomPose(small, 1, geo)!;
  assert.ok(shot.s > 1.15);
  assert.deepEqual(cameraAt(0, 300, fps, shot, null, null, geo), WIDE);
  assert.deepEqual(cameraAt(150, 300, fps, shot, null, null, geo), shot);
  const end = cameraAt(300, 300, fps, shot, null, null, geo);
  assert.ok(Math.abs(end.s - 1) < 1e-6, `ends at scale ${end.s}`);
});

test("the camera pans between nearby shots instead of pulling back", () => {
  const a = zoomPose(small, 1, geo)!;
  const b = zoomPose(nearby, 1, geo)!;
  assert.ok(staysZoomed(a, b, geo));
  assert.deepEqual(cameraAt(300, 300, fps, a, null, b, geo), a);
  assert.deepEqual(cameraAt(0, 300, fps, b, a, null, geo), a);
});

test("the camera pulls back between distant shots", () => {
  assert.ok(!staysZoomed(zoomPose(small, 1, geo), zoomPose(far, 1, geo), geo));
});

test("a short beat still spends a third of its length on the zoomed shot", () => {
  const shot = zoomPose(small, 1, geo)!;
  let held = 0;
  for (let f = 0; f < 45; f++) if (cameraAt(f, 45, fps, shot, null, null, geo).s === shot.s) held++;
  assert.ok(held >= 15, `held ${held} of 45 frames`);
});
