import { describe, it, expect } from "vite-plus/test";
import { pseudoAccent, pseudoTranslate, pseudoWrap } from "../components/editor/pseudoTranslate";

const OPEN = "▒ ";
const CLOSE = " ▒";

describe("pseudoTranslate", () => {
  it("wraps text in shade markers (with inner spaces) and accents ASCII letters", () => {
    expect(pseudoTranslate("Hello")).toBe(`${OPEN}Ĥéļļö${CLOSE}`);
  });

  it("wraps with shade markers for empty string", () => {
    expect(pseudoTranslate("")).toBe(`${OPEN}${CLOSE}`);
  });

  it("preserves non-ASCII characters", () => {
    expect(pseudoTranslate("café")).toBe(`${OPEN}çàƒé${CLOSE}`);
  });

  it("accents both upper and lower case", () => {
    expect(pseudoTranslate("Az")).toBe(`${OPEN}Àž${CLOSE}`);
  });

  it("preserves digits, punctuation, and whitespace", () => {
    expect(pseudoTranslate("Hi 123!")).toBe(`${OPEN}Ĥî 123!${CLOSE}`);
  });

  it("handles a full sentence", () => {
    const result = pseudoTranslate("Save changes?");
    expect(result.startsWith(OPEN)).toBe(true);
    expect(result.endsWith(CLOSE)).toBe(true);
    // Should not contain any plain ASCII a-z / A-Z that has a mapping
    expect(result).not.toContain("S");
    expect(result).toContain("?");
  });
});

// The preview accents each text run on its own and wraps the block once, so
// the inline codes between the runs stay as they are.
describe("pseudoAccent / pseudoWrap", () => {
  it("accents letters without the shade markers", () => {
    expect(pseudoAccent("Hello")).toBe("Ĥéļļö");
    expect(pseudoAccent("")).toBe("");
  });

  it("wraps a block once, so pseudoTranslate is the two together", () => {
    expect(pseudoWrap("x")).toBe(`${OPEN}x${CLOSE}`);
    expect(pseudoWrap(pseudoAccent("Hello"))).toBe(pseudoTranslate("Hello"));
  });

  it("leaves the markup of a code untouched when the runs are joined", () => {
    const html = pseudoWrap(`${pseudoAccent("Click ")}<b>${pseudoAccent("here")}</b>`);
    expect(html).toBe(`${OPEN}Çļîçķ <b>ĥéŕé</b>${CLOSE}`);
  });
});
