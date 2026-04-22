import { describe, it, expect } from "vite-plus/test";
import { pseudoTranslate, pseudoTranslateCoded } from "../components/editor/pseudoTranslate";

// Unicode markers matching the Go model constants
const OPENING = "\uE001";
const CLOSING = "\uE002";
const PLACEHOLDER = "\uE003";

const OPEN = "\u2592 ";
const CLOSE = " \u2592";

describe("pseudoTranslate", () => {
  it("wraps text in shade markers (with inner spaces) and accents ASCII letters", () => {
    expect(pseudoTranslate("Hello")).toBe(`${OPEN}\u0124\u00e9\u013c\u013c\u00f6${CLOSE}`);
  });

  it("wraps with shade markers for empty string", () => {
    expect(pseudoTranslate("")).toBe(`${OPEN}${CLOSE}`);
  });

  it("preserves non-ASCII characters", () => {
    expect(pseudoTranslate("café")).toBe(`${OPEN}ç\u00e0ƒ\u00e9${CLOSE}`);
  });

  it("accents both upper and lower case", () => {
    expect(pseudoTranslate("Az")).toBe(`${OPEN}\u00c0\u017e${CLOSE}`);
  });

  it("preserves digits, punctuation, and whitespace", () => {
    expect(pseudoTranslate("Hi 123!")).toBe(`${OPEN}\u0124\u00ee 123!${CLOSE}`);
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

describe("pseudoTranslateCoded", () => {
  it("accents text and preserves span markers", () => {
    const coded = `Click ${OPENING}here${CLOSING}`;
    const result = pseudoTranslateCoded(coded);
    // Markers should be preserved in place
    expect(result).toContain(OPENING);
    expect(result).toContain(CLOSING);
    // Text around markers should be wrapped with shade markers
    expect(result.startsWith(OPEN)).toBe(true);
    expect(result.endsWith(CLOSE)).toBe(true);
  });

  it("preserves placeholder markers", () => {
    const coded = `Name: ${PLACEHOLDER}`;
    const result = pseudoTranslateCoded(coded);
    expect(result).toContain(PLACEHOLDER);
    expect(result.startsWith(OPEN)).toBe(true);
  });

  it("handles text with no markers like pseudoTranslate but with shade markers", () => {
    const result = pseudoTranslateCoded("Hello");
    expect(result).toBe(`${OPEN}\u0124\u00e9\u013c\u013c\u00f6${CLOSE}`);
  });

  it("handles empty string", () => {
    expect(pseudoTranslateCoded("")).toBe(`${OPEN}${CLOSE}`);
  });

  it("accents only text characters, not markers", () => {
    // All three marker types in sequence
    const coded = `a${OPENING}b${CLOSING}c${PLACEHOLDER}d`;
    const result = pseudoTranslateCoded(coded);
    expect(result).toBe(
      `${OPEN}\u00e0${OPENING}\u0183${CLOSING}\u00e7${PLACEHOLDER}\u0111${CLOSE}`,
    );
  });
});
