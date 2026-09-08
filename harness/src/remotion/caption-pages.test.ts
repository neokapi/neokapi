import test from "node:test";
import assert from "node:assert/strict";
import type { CaptionsFile } from "../types.ts";
import type { SceneTiming } from "./timeline.ts";
import { layoutCaptionPages } from "./caption-pages.ts";

const word = (text: string, startMs: number): CaptionsFile["scenes"][number]["captions"][number] => ({
  text: ` ${text}`,
  startMs,
  endMs: startMs + 300,
  timestampMs: startMs + 100,
  confidence: 1,
});

const scene = (id: string, from: number, durationFrames: number, transitionAfter = 0): SceneTiming => ({
  id,
  kind: "terminal",
  from,
  durationFrames,
  transitionBefore: 0,
  transitionAfter,
  termFrom: 0,
  revealStart: 0,
  revealEnd: 0,
});

test("pages are laid out from each scene's start and hand over to the next page", () => {
  const captions: CaptionsFile = {
    id: "x",
    engine: "test",
    scenes: [{ id: "a", captions: [word("one", 0), word("two", 400), word("three", 1500), word("four", 1900)] }],
  };
  const slots = layoutCaptionPages([scene("a", 90, 300)], captions, 30);
  assert.equal(slots.length, 2);
  // The pager keeps a word on the page while the words stay close, so the
  // first page carries three words and the second starts on "four" at 1.9 s.
  assert.equal(slots[0]!.page.text, "one two three");
  assert.equal(slots[0]!.from, 90);
  assert.equal(slots[0]!.durationInFrames, 57);
  assert.equal(slots[1]!.page.text, "four");
  assert.equal(slots[1]!.from, 147);
  // The last page lingers 600 ms after its last word (2.2 s + 0.6 s = 2.8 s).
  assert.equal(slots[1]!.from + slots[1]!.durationInFrames, 90 + 84);
});

test("the last page never runs past the scene's own end", () => {
  const captions: CaptionsFile = { id: "x", engine: "test", scenes: [{ id: "a", captions: [word("late", 2800)] }] };
  const slots = layoutCaptionPages([scene("a", 0, 102, 12)], captions, 30);
  assert.equal(slots.length, 1);
  assert.equal(slots[0]!.from, 84);
  assert.equal(slots[0]!.from + slots[0]!.durationInFrames, 90); // scene end = 102 - 12
});

test("scenes without captions and a missing file lay out nothing", () => {
  assert.deepEqual(layoutCaptionPages([scene("a", 0, 100)], null, 30), []);
  assert.deepEqual(layoutCaptionPages([scene("a", 0, 100)], { id: "x", engine: "t", scenes: [] }, 30), []);
});
