import test from "node:test";
import assert from "node:assert/strict";
import { clickFrames, planPlayback } from "./playback.ts";

test("a beat about as long as its scene plays at its natural rate", () => {
  assert.deepEqual(planPlayback(10, 300, 30), { rate: 1, playFrames: 300, heldFrames: 0 });
  assert.deepEqual(planPlayback(9, 300, 30), { rate: 0.9, playFrames: 300, heldFrames: 0 });
  assert.deepEqual(planPlayback(15, 300, 30), { rate: 1.5, playFrames: 300, heldFrames: 0 });
});

test("a beat much shorter than its scene plays at 1x and holds its last frame", () => {
  // 3 s of picture under 10 s of narration: never 0.3x slow motion.
  assert.deepEqual(planPlayback(3, 300, 30), { rate: 1, playFrames: 90, heldFrames: 210 });
});

test("a beat much longer than its scene is capped at twice speed", () => {
  assert.deepEqual(planPlayback(30, 300, 30), { rate: 2, playFrames: 300, heldFrames: 0 });
});

test("clicks inside the played slice land on their scene frame, the rest are dropped", () => {
  const beat = { tStart: 10, tEnd: 14 };
  const plan = planPlayback(4, 300, 30); // 1x, 120 frames played, 180 held
  assert.deepEqual(clickFrames([9.5, 10.5, 12, 13.9, 14.2], beat, plan, 30), [15, 60, 117]);
  const fast = planPlayback(8, 120, 30); // 2x
  assert.deepEqual(clickFrames([11], { tStart: 10, tEnd: 18 }, fast, 30), [15]);
  assert.deepEqual(clickFrames(undefined, beat, plan, 30), []);
});
