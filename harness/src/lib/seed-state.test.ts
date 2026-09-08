import test from "node:test";
import assert from "node:assert/strict";
import {
  describeReadiness,
  isRunnable,
  needsPendingWork,
  needsSettle,
  targetsToClear,
} from "./seed-state.ts";
import type { ConvergenceEstimate, EditorBlock } from "./seed-state.ts";

const est = (
  source: Partial<ConvergenceEstimate["source"]>,
  pending: number,
): ConvergenceEstimate => ({
  source: { gate: "checked", total: 14, ready: 0, held: 14, ...source },
  totals: { pending },
});

test("the seeded state the automations walk found is not runnable", () => {
  // GET /api/v1/bowmart/<project>/convergence/estimate, before this seed step.
  const held = est({ ready: 0, held: 14 }, 0);
  assert.equal(needsSettle(held), true);
  assert.equal(needsPendingWork(held), true);
  assert.equal(isRunnable(held), false);
});

test("settled source with every locale covered is still not runnable", () => {
  const covered = est({ ready: 14, held: 0 }, 0);
  assert.equal(needsSettle(covered), false);
  assert.equal(needsPendingWork(covered), true);
  assert.equal(isRunnable(covered), false);
});

test("settled source with pending work is runnable", () => {
  const ready = est({ ready: 14, held: 0 }, 14);
  assert.equal(needsSettle(ready), false);
  assert.equal(needsPendingWork(ready), false);
  assert.equal(isRunnable(ready), true);
});

test("a project that opted out of the gate needs only pending work", () => {
  const open = est({ gate: "none", ready: 14, held: 0 }, 6);
  assert.equal(needsSettle(open), false);
  assert.equal(isRunnable(open), true);
});

test("describeReadiness names whichever half is missing", () => {
  assert.match(describeReadiness(est({ ready: 0, held: 14 }, 0)), /14 of 14 .*"checked" gate/);
  assert.match(describeReadiness(est({ ready: 14, held: 0 }, 0)), /every locale already covered/);
  assert.match(describeReadiness(est({ ready: 14, held: 0 }, 6)), /6 unit\(s\) pending/);
});

const blocks: EditorBlock[] = [
  { id: "a", targets: { fr: { text: "Bonjour" }, ja: { text: "こんにちは" } } },
  { id: "b", targets: { fr: { text: "Salut" } } },
  { id: "c", targets: { ja: { text: "   " } } },
  { id: "d", translatable: false, targets: { ja: { text: "見出し" } } },
  { id: "e" },
];

test("targetsToClear names only the blocks holding target text in the locale", () => {
  assert.deepEqual(targetsToClear(blocks, "ja"), ["a"]);
  assert.deepEqual(targetsToClear(blocks, "fr"), ["a", "b"]);
});

test("targetsToClear is empty once the locale has been cleared", () => {
  const cleared: EditorBlock[] = blocks.map((b) =>
    b.targets?.ja ? { ...b, targets: { ...b.targets, ja: { text: "" } } } : b,
  );
  assert.deepEqual(targetsToClear(cleared, "ja"), []);
});
