import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { cueAt, cueWord, cuesFrom, narrationCues } from "./cues.ts";

const narration = {
  scenes: [
    { id: "title", kind: "title" },
    { id: "keep", kind: "desktop", beat: "keep" },
    { id: "choose-scene", kind: "desktop", beat: "choose" },
  ],
};
const captions = {
  scenes: [
    { id: "title", captions: [{ text: " An", startMs: 100 }] },
    {
      id: "keep",
      captions: [
        { text: " KapiMart.", startMs: 4700 },
        { text: " Pressing", startMs: 5700 },
        { text: " a", startMs: 6400 },
        { text: " keeps", startMs: 6500 },
        { text: " a", startMs: 10300 },
      ],
    },
    { id: "choose-scene", captions: [{ text: " claude-code's", startMs: 6500 }] },
  ],
};

test("cueWord drops case and the punctuation around a word, and keeps what is inside it", () => {
  assert.equal(cueWord(" KapiMart."), "kapimart");
  assert.equal(cueWord("claude-code's"), "claude-code's");
  assert.equal(cueWord("“sign"), "sign");
  assert.equal(cueWord("Ålesund,"), "ålesund");
});

test("cuesFrom keys each desktop scene's words by its beat, in seconds", () => {
  const cues = cuesFrom(narration, captions);
  assert.deepEqual(Object.keys(cues).sort(), ["choose", "keep"]);
  assert.deepEqual(cues.keep.map((c) => c.word), ["kapimart", "pressing", "a", "keeps", "a"]);
  assert.equal(cues.keep[1].at, 5.7);
});

test("cueAt finds the first time a word is spoken over a beat", () => {
  const cues = cuesFrom(narration, captions);
  assert.equal(cueAt(cues.keep, "a"), 6.4);
  assert.equal(cueAt(cues.keep, "Keeps"), 6.5);
  assert.equal(cueAt(cues.choose, "claude-code's"), 6.5);
  assert.equal(cueAt(cues.keep, "basket"), undefined);
  assert.equal(cueAt(undefined, "keeps"), undefined);
});

test("narrationCues reads the locale's files and is empty without them", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cues-"));
  try {
    assert.deepEqual(narrationCues(dir), {});
    fs.writeFileSync(path.join(dir, "narration-nb.json"), JSON.stringify(narration));
    fs.writeFileSync(path.join(dir, "captions-nb.json"), JSON.stringify(captions));
    assert.deepEqual(narrationCues(dir), {});
    assert.equal(cueAt(narrationCues(dir, "nb").keep, "pressing"), 5.7);
    fs.writeFileSync(path.join(dir, "captions.json"), "{");
    fs.writeFileSync(path.join(dir, "narration.json"), JSON.stringify(narration));
    assert.deepEqual(narrationCues(dir), {});
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});
