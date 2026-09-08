import test from "node:test";
import assert from "node:assert/strict";
import { alignScenes, sceneBoundaries, tokenize, type SpokenWord } from "./align.ts";

/** Speak a text at a steady 400 ms a word from `atMs`, with a pause after. */
function speak(text: string, atMs: number, words: SpokenWord[]): number {
  let t = atMs;
  for (const w of text.split(/\s+/).filter(Boolean)) {
    words.push({ text: ` ${w}`, startMs: t, endMs: t + 350 });
    t += 400;
  }
  return t + 600;
}

test("tokenize keeps letters and digits and drops case and punctuation", () => {
  assert.deepEqual(tokenize("kapi check, app.json -- 100/100!"), ["kapi", "check", "appjson", "100100"]);
});

test("alignScenes finds each scene's words in order and measures its span", () => {
  const scenes = ["An AI translated this app into German.", "kapi check reads the German against the English.", "Now it passes."];
  const words: SpokenWord[] = [];
  let t = 0;
  for (const s of scenes) t = speak(s, t, words);
  const { spans, coverage } = alignScenes(scenes, words);
  assert.equal(coverage, 1);
  assert.equal(spans[0]!.startMs, 0);
  assert.equal(spans[0]!.aligned, 7);
  assert.equal(spans[1]!.startMs, 7 * 400 + 600);
  assert.equal(spans[2]!.aligned, 3);
});

test("alignScenes survives a misheard product name and a dropped word", () => {
  const scenes = ["kapi check reads the German against the English and runs the rules.", "Now it passes, with the gate on."];
  const words: SpokenWord[] = [];
  let t = speak("copy check reads the german against english and runs the rules", 0, words);
  speak("now it passes with the gate on", t, words);
  const { spans, coverage } = alignScenes(scenes, words);
  assert.ok(coverage > 0.85, `coverage ${coverage}`);
  // "kapi" was heard as "copy" and one "the" was dropped: ten of twelve words
  // align, and the scene's span starts on "check".
  assert.equal(spans[0]!.startMs, 400);
  assert.equal(spans[0]!.aligned, 10);
  assert.equal(spans[0]!.total, 12);
  assert.equal(spans[1]!.startMs, t);
});

test("sceneBoundaries cuts inside the pause before a scene and fills a missed scene by word share", () => {
  const spans = [
    { startMs: 0, endMs: 3000, aligned: 6, total: 6 },
    null,
    { startMs: 8000, endMs: 10000, aligned: 4, total: 4 },
  ];
  const b = sceneBoundaries(spans, [6, 4, 4], 11000);
  assert.equal(b.length, 4);
  assert.equal(b[0], 0);
  assert.equal(b[2], 7800); // 200 ms ahead of the first word at 8000
  // The missed scene shares [3000, 7800) with its predecessor by words: 6 of 10.
  assert.equal(b[1], 3000 + (4800 * 6) / 10);
  assert.equal(b[3], 11000);
});

test("sceneBoundaries takes half a short pause rather than cutting into the previous scene", () => {
  const spans = [
    { startMs: 0, endMs: 2900, aligned: 2, total: 2 },
    { startMs: 3000, endMs: 5000, aligned: 2, total: 2 },
  ];
  const b = sceneBoundaries(spans, [2, 2], 5000);
  assert.equal(b[1], 2950);
});
