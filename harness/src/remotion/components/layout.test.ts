import test from "node:test";
import assert from "node:assert/strict";
import { CAPTION_BAND, CHAPTER_BAND, SAFE_X, SAFE_Y, sceneLayout, TERM_FS, TERM_LH } from "./layout.ts";

test("the grid keeps text inside the safe area and a window above the captions", () => {
  const lay = sceneLayout(false);
  assert.equal(lay.left, SAFE_X);
  assert.equal(lay.width, 1920 - 2 * SAFE_X);
  assert.equal(lay.captionBottom, 1080 - SAFE_Y);
  assert.equal(lay.captionTop, lay.captionBottom - CAPTION_BAND);
  assert.ok(lay.stackBottom < lay.captionTop);
  assert.equal(lay.picTop, 100);
  assert.equal(lay.picBottom, 980);
});

test("a chapter line pushes the picture down by its band", () => {
  const plain = sceneLayout(false);
  const chaptered = sceneLayout(true);
  assert.equal(chaptered.chapterTop, SAFE_Y);
  assert.equal(chaptered.picTop, SAFE_Y + CHAPTER_BAND);
  assert.equal(chaptered.stackBottom, plain.stackBottom);
});

test("a terminal window shows at most sixteen lines", () => {
  const line = TERM_FS * TERM_LH;
  for (const chapter of [false, true]) {
    const lay = sceneLayout(chapter);
    const transcript = lay.stackBottom - lay.picTop - 48 - 52; // title bar and padding
    assert.ok(Math.floor(transcript / line) <= 16);
    assert.ok(Math.floor(transcript / line) >= 10);
  }
});
