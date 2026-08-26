import { describe, expect, it } from "vitest";
import type { Run } from "./types";
import { rangeAnchorForBytes, rangeAnchorForChars, runsPlainText, textForBytes } from "./anchor";

// The fixtures mirror what a reader emits: text runs interleaved with inline
// codes, and a plural wrapper whose "other" branch carries the flat text.

const code = (id: string): Run => ({ ph: { id, type: "var", data: `{${id}}`, equiv: id } });

const simple: Run[] = [{ text: "Welcome to Acme" }];

const withCodes: Run[] = [
  { text: "Hello " },
  code("name"),
  { text: ", welcome to " },
  { pcOpen: { id: "b", type: "bold", data: "<b>", equiv: "b" } },
  { text: "Acme" },
  { pcClose: { id: "b", type: "bold", data: "</b>", equiv: "b" } },
  { text: "." },
];

const withPlural: Run[] = [
  { text: "You have " },
  {
    plural: {
      pivot: "count",
      forms: { one: [{ text: "1 message" }], other: [{ text: "N messages" }] },
    },
  },
  { text: "!" },
];

describe("runsPlainText", () => {
  it("concatenates text runs and skips inline codes", () => {
    expect(runsPlainText(withCodes)).toBe("Hello , welcome to Acme.");
  });

  it("flattens a plural run through its other branch", () => {
    expect(runsPlainText(withPlural)).toBe("You have N messages!");
  });

  it("falls back to the first branch when there is no other form", () => {
    const runs: Run[] = [{ plural: { pivot: "n", forms: { one: [{ text: "single" }] } } }];
    expect(runsPlainText(runs)).toBe("single");
  });

  it("flattens a select run through its other case", () => {
    const runs: Run[] = [
      { select: { pivot: "g", cases: { female: [{ text: "her" }], other: [{ text: "their" }] } } },
    ];
    expect(runsPlainText(runs)).toBe("their");
  });

  it("is empty for undefined and for a code-only sequence", () => {
    expect(runsPlainText(undefined)).toBe("");
    expect(runsPlainText([])).toBe("");
    expect(runsPlainText([code("a"), code("b")])).toBe("");
  });
});

describe("rangeAnchorForChars", () => {
  it("anchors a span inside a single text run", () => {
    // "Acme" is chars 11..15 — the end lands on the run boundary, so it reads
    // as the start of the (nonexistent) run 1, the engine's convention.
    expect(rangeAnchorForChars(simple, 11, 15)).toEqual({
      kind: "range",
      start: { run: 0, offset: 11 },
      end: { run: 1 },
    });
  });

  it("skips zero-width code runs when counting", () => {
    // "Hello , welcome to Acme." — "Acme" is chars 19..23. Both boundaries end
    // a text run, so they attach to the following code runs (3 = pcOpen,
    // 5 = pcClose) — a leading code belongs to the span it opens.
    expect(rangeAnchorForChars(withCodes, 19, 23)).toEqual({
      kind: "range",
      start: { run: 3 },
      end: { run: 5 },
    });
  });

  it("spans across runs when the text crosses a code", () => {
    // "welcome to Acme" starts mid-run-2 and ends on the run-4 boundary.
    expect(rangeAnchorForChars(withCodes, 8, 23)).toEqual({
      kind: "range",
      start: { run: 2, offset: 2 },
      end: { run: 5 },
    });
  });

  it("attributes a boundary at a run's end to the next run", () => {
    // Char 6 ends run 0 ("Hello ") exactly.
    expect(rangeAnchorForChars(withCodes, 0, 6)).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 1 },
    });
  });

  it("counts a plural run as the width of its other branch", () => {
    // "You have N messages!" — "messages" is chars 11..19, inside the plural.
    expect(rangeAnchorForChars(withPlural, 11, 19)).toEqual({
      kind: "range",
      start: { run: 1, offset: 2 },
      end: { run: 2 },
    });
  });

  it("clamps a negative start and an overrunning end", () => {
    expect(rangeAnchorForChars(simple, -5, 999)).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 1 },
    });
  });

  it("returns a zero range for an empty sequence", () => {
    expect(rangeAnchorForChars([], 0, 4)).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 0 },
    });
    expect(rangeAnchorForChars(undefined, 2, 4)).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 0 },
    });
  });
});

describe("rangeAnchorForBytes", () => {
  it("matches the char range for ASCII text", () => {
    expect(rangeAnchorForBytes(simple, 11, 15)).toEqual(rangeAnchorForChars(simple, 11, 15));
  });

  it("converts multi-byte UTF-8 offsets to code points", () => {
    // "Æblegrød" — "grød" is bytes 5..11 (Æ is 2 bytes, ø is 2), chars 4..8.
    const runs: Run[] = [{ text: "Æblegrød" }];
    expect(rangeAnchorForBytes(runs, 5, 11)).toEqual({
      kind: "range",
      start: { run: 0, offset: 4 },
      end: { run: 1 },
    });
  });

  it("handles astral code points as single positions", () => {
    // "a😀b" — the emoji is 4 UTF-8 bytes and one code point.
    const runs: Run[] = [{ text: "a😀b" }];
    expect(rangeAnchorForBytes(runs, 1, 5)).toEqual({
      kind: "range",
      start: { run: 0, offset: 1 },
      end: { run: 0, offset: 2 },
    });
  });

  it("clamps a byte offset past the end of the text", () => {
    expect(rangeAnchorForBytes(simple, 0, 9999)).toEqual({
      kind: "range",
      start: { run: 0 },
      end: { run: 1 },
    });
  });
});

describe("textForBytes", () => {
  it("returns the surface a byte span covers across runs", () => {
    expect(textForBytes(withCodes, 19, 23)).toBe("Acme");
    expect(textForBytes(withCodes, 6, 23)).toBe(", welcome to Acme");
  });

  it("slices multi-byte text by code point", () => {
    expect(textForBytes([{ text: "Æblegrød" }], 5, 11)).toBe("grød");
    expect(textForBytes([{ text: "a😀b" }], 1, 5)).toBe("😀");
  });

  it("is empty for an empty or code-only sequence", () => {
    expect(textForBytes(undefined, 0, 4)).toBe("");
    expect(textForBytes([code("a")], 0, 4)).toBe("");
  });
});
