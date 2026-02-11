import { describe, it, expect } from "vitest";
import { parseCodedSegments, segmentsToCodedText, spanLabel } from "../components/editor/codedText";
import type { SpanInfo } from "../types/api";

// Unicode markers matching the Go model constants
const OPENING = "\uE001";
const CLOSING = "\uE002";
const PLACEHOLDER = "\uE003";

function span(spanType: "opening" | "closing" | "placeholder", type: string, data: string): SpanInfo {
  return { span_type: spanType, type, id: "1", data };
}

describe("parseCodedSegments", () => {
  it("returns a single text segment for plain text", () => {
    const result = parseCodedSegments("Hello world", []);
    expect(result).toEqual([{ type: "text", value: "Hello world" }]);
  });

  it("returns empty array for empty string", () => {
    expect(parseCodedSegments("", [])).toEqual([]);
  });

  it("parses opening and closing tags around text", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("closing", "b", "</b>"),
    ];
    const coded = `${OPENING}bold${CLOSING}`;
    const result = parseCodedSegments(coded, spans);

    expect(result).toEqual([
      { type: "tag", spanInfo: spans[0] },
      { type: "text", value: "bold" },
      { type: "tag", spanInfo: spans[1] },
    ]);
  });

  it("handles placeholder tags", () => {
    const spans: SpanInfo[] = [span("placeholder", "br", "<br/>")];
    const coded = `Line one${PLACEHOLDER}Line two`;
    const result = parseCodedSegments(coded, spans);

    expect(result).toEqual([
      { type: "text", value: "Line one" },
      { type: "tag", spanInfo: spans[0] },
      { type: "text", value: "Line two" },
    ]);
  });

  it("handles multiple adjacent tags", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("opening", "i", "<i>"),
      span("closing", "i", "</i>"),
      span("closing", "b", "</b>"),
    ];
    const coded = `${OPENING}${OPENING}text${CLOSING}${CLOSING}`;
    const result = parseCodedSegments(coded, spans);

    expect(result).toHaveLength(5);
    expect(result[0]).toEqual({ type: "tag", spanInfo: spans[0] });
    expect(result[1]).toEqual({ type: "tag", spanInfo: spans[1] });
    expect(result[2]).toEqual({ type: "text", value: "text" });
    expect(result[3]).toEqual({ type: "tag", spanInfo: spans[2] });
    expect(result[4]).toEqual({ type: "tag", spanInfo: spans[3] });
  });

  it("handles text before and after tags", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("closing", "b", "</b>"),
    ];
    const coded = `before ${OPENING}middle${CLOSING} after`;
    const result = parseCodedSegments(coded, spans);

    expect(result).toEqual([
      { type: "text", value: "before " },
      { type: "tag", spanInfo: spans[0] },
      { type: "text", value: "middle" },
      { type: "tag", spanInfo: spans[1] },
      { type: "text", value: " after" },
    ]);
  });

  it("ignores extra markers beyond available spans", () => {
    const spans: SpanInfo[] = [span("opening", "b", "<b>")];
    const coded = `${OPENING}text${CLOSING}`;
    const result = parseCodedSegments(coded, spans);

    expect(result).toEqual([
      { type: "tag", spanInfo: spans[0] },
      { type: "text", value: "text" },
      // second marker ignored because no span left
    ]);
  });
});

describe("segmentsToCodedText", () => {
  it("roundtrips plain text", () => {
    const { codedText, spans } = segmentsToCodedText([
      { type: "text", value: "Hello world" },
    ]);
    expect(codedText).toBe("Hello world");
    expect(spans).toEqual([]);
  });

  it("roundtrips tagged content", () => {
    const openingSpan = span("opening", "b", "<b>");
    const closingSpan = span("closing", "b", "</b>");
    const segments = [
      { type: "tag" as const, spanInfo: openingSpan },
      { type: "text" as const, value: "bold" },
      { type: "tag" as const, spanInfo: closingSpan },
    ];

    const { codedText, spans } = segmentsToCodedText(segments);
    expect(codedText).toBe(`${OPENING}bold${CLOSING}`);
    expect(spans).toEqual([openingSpan, closingSpan]);
  });

  it("uses correct marker for each span type", () => {
    const segments = [
      { type: "tag" as const, spanInfo: span("opening", "a", "<a>") },
      { type: "tag" as const, spanInfo: span("placeholder", "br", "<br/>") },
      { type: "tag" as const, spanInfo: span("closing", "a", "</a>") },
    ];

    const { codedText } = segmentsToCodedText(segments);
    expect(codedText).toBe(`${OPENING}${PLACEHOLDER}${CLOSING}`);
  });

  it("roundtrips with parseCodedSegments", () => {
    const originalSpans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("placeholder", "br", "<br/>"),
      span("closing", "b", "</b>"),
    ];
    const originalCoded = `before ${OPENING}${PLACEHOLDER}text${CLOSING} after`;

    const segments = parseCodedSegments(originalCoded, originalSpans);
    const { codedText, spans } = segmentsToCodedText(segments);

    expect(codedText).toBe(originalCoded);
    expect(spans).toEqual(originalSpans);
  });
});

describe("spanLabel", () => {
  it("labels bold opening as 'B>'", () => {
    expect(spanLabel(span("opening", "b", "<b>"))).toBe("B>");
  });

  it("labels italic closing as '/I'", () => {
    expect(spanLabel(span("closing", "i", "</i>"))).toBe("/I");
  });

  it("labels break placeholder as 'br'", () => {
    expect(spanLabel(span("placeholder", "br", "<br/>"))).toBe("br");
  });
});
