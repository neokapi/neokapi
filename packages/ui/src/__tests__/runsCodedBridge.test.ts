import { describe, expect, it } from "vitest";

import { flattenRuns, type Run } from "@neokapi/kapi-format";

import { codedToRuns, runsToCoded } from "../components/editor/runsCodedBridge";
import type { SpanInfo } from "../types/span";

const boldOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:bold",
  id: "1",
  data: "<strong>",
  equiv_text: "strong",
};
const boldClose: SpanInfo = {
  span_type: "closing",
  type: "fmt:bold",
  id: "1",
  data: "</strong>",
  equiv_text: "strong",
};
const countPh: SpanInfo = {
  span_type: "placeholder",
  type: "jsx:var",
  id: "0",
  data: "{count}",
  equiv_text: "count",
};

describe("codedToRuns", () => {
  it("turns plain coded text into a single TextRun", () => {
    const runs = codedToRuns("Hello world", []);
    expect(runs).toEqual([{ text: "Hello world" }]);
  });

  it("turns each marker into a typed run, in document order", () => {
    // Coded text: text, opening, text, closing, text — three text
    // chunks, two tag markers, spans positionally aligned.
    const coded = "Click \uE001here\uE002 now";
    const runs = codedToRuns(coded, [boldOpen, boldClose]);
    expect(runs).toEqual([
      { text: "Click " },
      {
        pcOpen: {
          id: "1",
          type: "fmt:bold",
          data: "<strong>",
          equiv: "strong",
        },
      },
      { text: "here" },
      {
        pcClose: {
          id: "1",
          type: "fmt:bold",
          data: "</strong>",
          equiv: "strong",
        },
      },
      { text: " now" },
    ]);
  });

  it("turns placeholder markers into PlaceholderRuns", () => {
    const runs = codedToRuns("You have \uE003 messages", [countPh]);
    expect(runs).toEqual([
      { text: "You have " },
      {
        ph: {
          id: "0",
          type: "jsx:var",
          data: "{count}",
          equiv: "count",
        },
      },
      { text: " messages" },
    ]);
  });

  it("returns empty array for empty input", () => {
    expect(codedToRuns("", [])).toEqual([]);
  });
});

describe("runsToCoded", () => {
  it("emits coded text + spans for a flat run sequence", () => {
    const runs: Run[] = [
      { text: "Click " },
      { pcOpen: { id: "1", type: "fmt:bold", data: "<strong>", equiv: "strong" } },
      { text: "here" },
      { pcClose: { id: "1", type: "fmt:bold", data: "</strong>", equiv: "strong" } },
      { text: " now" },
    ];
    const result = runsToCoded(runs);
    expect(result.codedText).toBe("Click \uE001here\uE002 now");
    expect(result.spans).toHaveLength(2);
    expect(result.spans[0].span_type).toBe("opening");
    expect(result.spans[0].equiv_text).toBe("strong");
    expect(result.spans[1].span_type).toBe("closing");
  });

  it("turns PlaceholderRun into a placeholder marker + span", () => {
    const runs: Run[] = [
      { text: "You have " },
      { ph: { id: "0", type: "jsx:var", data: "{count}", equiv: "count" } },
      { text: " messages" },
    ];
    const result = runsToCoded(runs);
    expect(result.codedText).toBe("You have \uE003 messages");
    expect(result.spans[0].span_type).toBe("placeholder");
    expect(result.spans[0].equiv_text).toBe("count");
  });

  it("throws on plural / select wrappers", () => {
    expect(() =>
      runsToCoded([{ plural: { pivot: "n", forms: { other: [{ text: "x" }] } } }]),
    ).toThrow(/plural\/select/);
  });
});

describe("codedToRuns + runsToCoded round-trip", () => {
  it("preserves text + paired codes", () => {
    const coded = "Click \uE001here\uE002 now";
    const spans = [boldOpen, boldClose];
    const runs = codedToRuns(coded, spans);
    const back = runsToCoded(runs);
    expect(back.codedText).toBe(coded);
    expect(back.spans).toEqual(spans);
  });

  it("preserves placeholder markers", () => {
    const coded = "You have \uE003 messages";
    const runs = codedToRuns(coded, [countPh]);
    const back = runsToCoded(runs);
    expect(back.codedText).toBe(coded);
    expect(back.spans).toEqual([countPh]);
  });

  it("flattenRuns produces ICU-marker form for paired codes", () => {
    // The point of this bridge: each plural form's editor state
    // (coded text + spans) round-trips losslessly to a Run sequence,
    // which then serialises to the marker form ICU plurals expect
    // inside per-form content.
    const runs = codedToRuns("Click \uE001here\uE002 now", [boldOpen, boldClose]);
    const flat = flattenRuns(runs);
    // Paired codes flatten with the {=mN} marker — same shape that
    // `parseICUPluralString` round-trips with.
    expect(flat).toBe("Click {=m1}here{/=m1} now");
  });
});
