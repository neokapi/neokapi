import test from "node:test";
import assert from "node:assert/strict";
import { sliceCaptions, whisperLanguage, wordShareBoundaries } from "./captions.ts";

test("wordShareBoundaries splits a track by each scene's share of the words", () => {
  assert.deepEqual(wordShareBoundaries(["one two three", "four", "five six"], 6000), [0, 3000, 4000, 6000]);
});

test("sliceCaptions keeps the words that start inside a span and re-bases them", () => {
  const all = [
    { text: " a", startMs: 0, endMs: 300, timestampMs: 100, confidence: 1 },
    { text: " b", startMs: 900, endMs: 1400, timestampMs: 1000, confidence: 1 },
    { text: " c", startMs: 1500, endMs: 1900, timestampMs: 1600, confidence: 1 },
  ];
  assert.deepEqual(sliceCaptions(all, 800, 1500), [{ text: " b", startMs: 100, endMs: 600, timestampMs: 200, confidence: 1 }]);
  assert.deepEqual(
    sliceCaptions(all, 1500, 5000).map((c) => c.text),
    [" c"],
  );
});

test("whisperLanguage maps the narration locale to what whisper.cpp expects", () => {
  assert.equal(whisperLanguage("en"), "en");
  assert.equal(whisperLanguage("nb"), "no");
  assert.equal(whisperLanguage("nb-NO"), "no");
  assert.equal(whisperLanguage("de-DE"), "de");
});
