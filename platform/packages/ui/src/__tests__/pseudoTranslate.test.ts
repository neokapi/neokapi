import { describe, it, expect } from "vite-plus/test";
import { pseudoTranslate, pseudoTranslateCoded } from "../components/editor/pseudoTranslate";

// Unicode markers matching the Go model constants
const OPENING = "\uE001";
const CLOSING = "\uE002";
const PLACEHOLDER = "\uE003";

describe("pseudoTranslate", () => {
  it("wraps text in brackets and accents ASCII letters", () => {
    expect(pseudoTranslate("Hello")).toBe("[H\u00e9ll\u00f6]");
  });

  it("wraps with brackets for empty string", () => {
    expect(pseudoTranslate("")).toBe("[]");
  });

  it("preserves non-ASCII characters", () => {
    expect(pseudoTranslate("café")).toBe("[ç\u00e0ƒ\u00e9]");
  });

  it("accents both upper and lower case", () => {
    expect(pseudoTranslate("Az")).toBe("[\u00c0\u017e]");
  });

  it("preserves digits, punctuation, and whitespace", () => {
    expect(pseudoTranslate("Hi 123!")).toBe("[H\u00ee 123!]");
  });

  it("handles a full sentence", () => {
    const result = pseudoTranslate("Save changes?");
    expect(result.startsWith("[")).toBe(true);
    expect(result.endsWith("]")).toBe(true);
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
    // Text around markers should be accented
    expect(result.startsWith("[")).toBe(true);
    expect(result.endsWith("]")).toBe(true);
  });

  it("preserves placeholder markers", () => {
    const coded = `Name: ${PLACEHOLDER}`;
    const result = pseudoTranslateCoded(coded);
    expect(result).toContain(PLACEHOLDER);
    expect(result.startsWith("[")).toBe(true);
  });

  it("handles text with no markers like pseudoTranslate but with brackets", () => {
    const result = pseudoTranslateCoded("Hello");
    expect(result).toBe("[H\u00e9ll\u00f6]");
  });

  it("handles empty string", () => {
    expect(pseudoTranslateCoded("")).toBe("[]");
  });

  it("accents only text characters, not markers", () => {
    // All three marker types in sequence
    const coded = `a${OPENING}b${CLOSING}c${PLACEHOLDER}d`;
    const result = pseudoTranslateCoded(coded);
    expect(result).toBe(`[\u00e0${OPENING}\u0183${CLOSING}\u00e7${PLACEHOLDER}\u0111]`);
  });
});
