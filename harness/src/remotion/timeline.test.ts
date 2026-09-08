import test from "node:test";
import assert from "node:assert/strict";
import type { TimelineEvent } from "../types.ts";
import { computeTiming, eventCountForStep, type TimedScene } from "./timeline.ts";

const scene = (id: string, kind: TimedScene["kind"], durationSec: number, extra: Partial<TimedScene> = {}): TimedScene => ({
  id,
  kind,
  text: "x",
  caption: "",
  durationSec,
  holdSec: 0,
  ...extra,
});

const FPS = 30;

test("a cut-only timeline is the sum of its scenes, floored per kind", () => {
  const t = computeTiming([scene("t", "title", 1), scene("a", "terminal", 10), scene("o", "outro", 2)], FPS);
  assert.deepEqual(
    t.scenes.map((s) => [s.from, s.durationFrames]),
    [
      [0, 90], // 1 s floored to the 3 s title minimum
      [90, 300],
      [390, 120], // 2 s floored to the 4 s outro minimum
    ],
  );
  assert.equal(t.totalFrames, 510);
  assert.equal(t.totalTermFrames, 300);
});

test("a transition lengthens the scene it leaves and starts the next scene on its narration", () => {
  const t = computeTiming([scene("t", "title", 4), scene("a", "artifact", 5), scene("b", "artifact", 5)], FPS, {
    transition: () => 12,
  });
  const [title, a, b] = t.scenes;
  assert.equal(title!.durationFrames, 120 + 12);
  assert.equal(title!.transitionAfter, 12);
  assert.equal(a!.from, 120); // the narration of `a` starts when the fade completes
  assert.equal(a!.transitionBefore, 12);
  assert.equal(a!.durationFrames, 150 + 12);
  assert.equal(b!.from, 270);
  assert.equal(b!.durationFrames, 150);
  assert.equal(b!.transitionAfter, 0);
  // Total = sum of bare lengths = sum of durations minus the two overlaps.
  assert.equal(t.totalFrames, 120 + 150 + 150);
  assert.equal(t.totalFrames, t.scenes.reduce((n, s) => n + s.durationFrames, 0) - 24);
});

test("hold is a floor on a scene's length", () => {
  const t = computeTiming([scene("d", "desktop", 2, { hold: 6 }), scene("e", "desktop", 8, { hold: 3 })], FPS);
  assert.equal(t.scenes[0]!.durationFrames, 180);
  assert.equal(t.scenes[1]!.durationFrames, 240);
});

const events: TimelineEvent[] = [
  { i: 0, kind: "comment", text: "first" },
  { i: 1, kind: "command", text: "kapi check" },
  { i: 2, kind: "output", text: "FAIL", isError: false },
  { i: 3, kind: "output", text: "more", isError: false },
  { i: 4, kind: "command", text: "cat x" },
  { i: 5, kind: "output", text: "{}", isError: false },
];

test("eventCountForStep counts a step's line and the output under it", () => {
  assert.equal(eventCountForStep(events, 1), 1);
  assert.equal(eventCountForStep(events, 2), 4);
  assert.equal(eventCountForStep(events, 3), 6);
  assert.equal(eventCountForStep(events, 9), 6);
});

test("without anchors the reveal is uniform over the terminal frames", () => {
  const t = computeTiming([scene("a", "terminal", 5), scene("b", "terminal", 5)], FPS, { events });
  const [a, b] = t.scenes;
  assert.equal(a!.revealStart, 0);
  assert.equal(a!.revealEnd, 3.25); // half of N + 0.5
  assert.equal(b!.revealStart, 3.25);
  assert.equal(b!.revealEnd, 6);
});

test("a through anchor pins what is on screen at the end of its scene", () => {
  const t = computeTiming(
    [scene("t", "title", 3), scene("a", "terminal", 5, { through: 2 }), scene("b", "terminal", 5), scene("c", "terminal", 5, { through: 3 })],
    FPS,
    { events },
  );
  const [, a, b, c] = t.scenes;
  assert.equal(a!.revealStart, 0);
  assert.equal(a!.revealEnd, 4); // the check and its two output lines
  assert.equal(b!.revealStart, 4);
  assert.equal(b!.revealEnd, 5); // halfway from 4 to 6 by frames
  assert.equal(c!.revealEnd, 6);
});
