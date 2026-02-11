import { describe, it, expect } from "vitest";
import {
  tagNameFromData,
  semanticCategory,
  semanticLabel,
  semanticTooltip,
  tagColors,
  buildPairs,
  validateTags,
  codedTextToHtml,
} from "../components/editor/tagSemantics";
import type { SpanInfo } from "../types/api";

function span(spanType: "opening" | "closing" | "placeholder", type: string, data: string): SpanInfo {
  return { span_type: spanType, type, id: "1", data };
}

// ---------------------------------------------------------------------------
// tagNameFromData
// ---------------------------------------------------------------------------
describe("tagNameFromData", () => {
  it("extracts tag name from opening tag", () => {
    expect(tagNameFromData("<b>")).toBe("b");
  });

  it("extracts tag name from closing tag", () => {
    expect(tagNameFromData("</div>")).toBe("div");
  });

  it("extracts tag name from self-closing tag", () => {
    expect(tagNameFromData("<br/>")).toBe("br");
  });

  it("extracts tag name from tag with attributes", () => {
    expect(tagNameFromData('<a href="url">')).toBe("a");
  });

  it("returns '?' for non-tag data", () => {
    expect(tagNameFromData("plain text")).toBe("?");
  });
});

// ---------------------------------------------------------------------------
// semanticCategory
// ---------------------------------------------------------------------------
describe("semanticCategory", () => {
  it("maps 'b' and 'strong' to bold", () => {
    expect(semanticCategory(span("opening", "b", "<b>"))).toBe("bold");
    expect(semanticCategory(span("opening", "strong", "<strong>"))).toBe("bold");
  });

  it("maps 'i', 'em', 'emphasis' to italic", () => {
    expect(semanticCategory(span("opening", "i", "<i>"))).toBe("italic");
    expect(semanticCategory(span("opening", "em", "<em>"))).toBe("italic");
  });

  it("maps link tags correctly", () => {
    expect(semanticCategory(span("opening", "a", "<a>"))).toBe("link");
  });

  it("maps code-like tags", () => {
    expect(semanticCategory(span("opening", "code", "<code>"))).toBe("code");
    expect(semanticCategory(span("opening", "kbd", "<kbd>"))).toBe("code");
    expect(semanticCategory(span("opening", "samp", "<samp>"))).toBe("code");
  });

  it("maps break tags", () => {
    expect(semanticCategory(span("placeholder", "br", "<br/>"))).toBe("break");
    expect(semanticCategory(span("placeholder", "wbr", "<wbr>"))).toBe("break");
  });

  it("falls back to generic for unknown tags", () => {
    expect(semanticCategory(span("opening", "span", "<span>"))).toBe("generic");
    expect(semanticCategory(span("opening", "div", "<div>"))).toBe("generic");
  });

  it("extracts type from data when type is empty", () => {
    expect(semanticCategory(span("opening", "", "<b>"))).toBe("bold");
    expect(semanticCategory(span("opening", "", "<em>"))).toBe("italic");
  });
});

// ---------------------------------------------------------------------------
// semanticLabel
// ---------------------------------------------------------------------------
describe("semanticLabel", () => {
  it("returns 'B>' for bold opening", () => {
    expect(semanticLabel(span("opening", "b", "<b>"))).toBe("B>");
  });

  it("returns '/B' for bold closing", () => {
    expect(semanticLabel(span("closing", "b", "</b>"))).toBe("/B");
  });

  it("returns 'I>' for italic opening", () => {
    expect(semanticLabel(span("opening", "i", "<i>"))).toBe("I>");
  });

  it("returns 'br' for break placeholder", () => {
    expect(semanticLabel(span("placeholder", "br", "<br/>"))).toBe("br");
  });

  it("returns 'a>' for link opening", () => {
    expect(semanticLabel(span("opening", "a", "<a>"))).toBe("a>");
  });

  it("handles generic tags with fallback to type name", () => {
    const s = span("opening", "span", "<span>");
    const label = semanticLabel(s);
    // generic has empty label, so falls back to type
    expect(label).toBe("span>");
  });
});

// ---------------------------------------------------------------------------
// semanticTooltip
// ---------------------------------------------------------------------------
describe("semanticTooltip", () => {
  it("generates tooltip for bold opening", () => {
    const tooltip = semanticTooltip(span("opening", "b", "<b>"));
    expect(tooltip).toBe("Bold open (b) — <b>");
  });

  it("generates tooltip for italic closing", () => {
    const tooltip = semanticTooltip(span("closing", "i", "</i>"));
    expect(tooltip).toBe("Italic close (i) — </i>");
  });

  it("generates tooltip for break placeholder", () => {
    const tooltip = semanticTooltip(span("placeholder", "br", "<br/>"));
    expect(tooltip).toBe("Break placeholder (br) — <br/>");
  });
});

// ---------------------------------------------------------------------------
// tagColors
// ---------------------------------------------------------------------------
describe("tagColors", () => {
  it("returns color scheme with bg, border, text", () => {
    const colors = tagColors(span("opening", "b", "<b>"));
    expect(colors).toHaveProperty("bg");
    expect(colors).toHaveProperty("border");
    expect(colors).toHaveProperty("text");
  });

  it("returns different colors for different categories", () => {
    const boldColors = tagColors(span("opening", "b", "<b>"));
    const italicColors = tagColors(span("opening", "i", "<i>"));
    expect(boldColors.bg).not.toBe(italicColors.bg);
  });
});

// ---------------------------------------------------------------------------
// buildPairs
// ---------------------------------------------------------------------------
describe("buildPairs", () => {
  it("returns empty map for no spans", () => {
    expect(buildPairs([]).size).toBe(0);
  });

  it("pairs matching opening and closing tags", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("closing", "b", "</b>"),
    ];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(2);
    // Both should share the same pairIndex
    expect(pairs.get(0)!.pairIndex).toBe(pairs.get(1)!.pairIndex);
  });

  it("handles nested pairs", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),
      span("opening", "i", "<i>"),
      span("closing", "i", "</i>"),
      span("closing", "b", "</b>"),
    ];
    const pairs = buildPairs(spans);

    // b open/close share pair, i open/close share pair, different pair indices
    const bPair = pairs.get(0)!.pairIndex;
    const iPair = pairs.get(1)!.pairIndex;
    expect(bPair).not.toBe(iPair);
    expect(pairs.get(3)!.pairIndex).toBe(bPair);
    expect(pairs.get(2)!.pairIndex).toBe(iPair);
  });

  it("handles same-type nesting (stack behavior)", () => {
    const spans: SpanInfo[] = [
      span("opening", "b", "<b>"),   // 0
      span("opening", "b", "<b>"),   // 1
      span("closing", "b", "</b>"),  // 2 — pairs with 1 (stack LIFO)
      span("closing", "b", "</b>"),  // 3 — pairs with 0
    ];
    const pairs = buildPairs(spans);

    expect(pairs.get(1)!.pairIndex).toBe(pairs.get(2)!.pairIndex);
    expect(pairs.get(0)!.pairIndex).toBe(pairs.get(3)!.pairIndex);
  });

  it("assigns standalone pair to placeholders", () => {
    const spans: SpanInfo[] = [span("placeholder", "br", "<br/>")];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(1);
    expect(pairs.get(0)!.pairIndex).toBe(1);
  });

  it("assigns standalone pair to unmatched closing", () => {
    const spans: SpanInfo[] = [span("closing", "b", "</b>")];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(1);
    // unmatched closing gets its own pair
    expect(pairs.get(0)!.pairIndex).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// validateTags
// ---------------------------------------------------------------------------
describe("validateTags", () => {
  it("returns valid for matching spans", () => {
    const source = [span("opening", "b", "<b>"), span("closing", "b", "</b>")];
    const target = [span("opening", "b", "<b>"), span("closing", "b", "</b>")];
    const result = validateTags(source, target);

    expect(result.valid).toBe(true);
    expect(result.errors).toHaveLength(0);
    expect(result.warnings).toHaveLength(0);
  });

  it("returns valid for empty spans", () => {
    expect(validateTags([], []).valid).toBe(true);
  });

  it("reports missing tags", () => {
    const source = [span("opening", "b", "<b>"), span("closing", "b", "</b>")];
    const target: SpanInfo[] = [];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.length).toBeGreaterThan(0);
    expect(result.errors.some(e => e.type === "missing_tag")).toBe(true);
  });

  it("reports extra tags as warnings", () => {
    const source: SpanInfo[] = [];
    const target = [span("opening", "b", "<b>"), span("closing", "b", "</b>")];
    const result = validateTags(source, target);

    // extra tags are warnings, not errors
    expect(result.warnings.some(w => w.type === "extra_tag")).toBe(true);
  });

  it("reports unpaired closing tags in target", () => {
    const source = [span("closing", "b", "</b>")];
    const target = [span("closing", "b", "</b>")];
    const result = validateTags(source, target);

    expect(result.errors.some(e => e.type === "unpaired")).toBe(true);
  });

  it("reports unpaired opening tags in target", () => {
    const source = [span("opening", "b", "<b>")];
    const target = [span("opening", "b", "<b>")];
    const result = validateTags(source, target);

    expect(result.errors.some(e => e.type === "unpaired")).toBe(true);
  });

  it("handles mixed valid and invalid tags", () => {
    const source = [
      span("opening", "b", "<b>"),
      span("closing", "b", "</b>"),
      span("placeholder", "br", "<br/>"),
    ];
    const target = [
      span("opening", "b", "<b>"),
      span("closing", "b", "</b>"),
      // missing br placeholder
    ];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.some(e => e.message.includes("br"))).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// codedTextToHtml
// ---------------------------------------------------------------------------
describe("codedTextToHtml", () => {
  const OPENING = "\uE001";
  const CLOSING = "\uE002";
  const PLACEHOLDER = "\uE003";

  it("escapes HTML entities in plain text", () => {
    expect(codedTextToHtml("a < b & c > d", [])).toBe("a &lt; b &amp; c &gt; d");
  });

  it("wraps bold text in <b> tags", () => {
    const spans = [span("opening", "b", "<b>"), span("closing", "b", "</b>")];
    const coded = `${OPENING}bold${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<b>bold</b>");
  });

  it("wraps italic text in <i> tags", () => {
    const spans = [span("opening", "i", "<i>"), span("closing", "i", "</i>")];
    const coded = `${OPENING}italic${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<i>italic</i>");
  });

  it("renders break placeholder as <br/>", () => {
    const spans = [span("placeholder", "br", "<br/>")];
    const coded = `line one${PLACEHOLDER}line two`;
    expect(codedTextToHtml(coded, spans)).toBe("line one<br/>line two");
  });

  it("renders image placeholder as [img]", () => {
    const spans = [span("placeholder", "img", "<img/>")];
    const coded = `text ${PLACEHOLDER} more`;
    expect(codedTextToHtml(coded, spans)).toContain("[img]");
  });

  it("renders code spans with inline styling", () => {
    const spans = [span("opening", "code", "<code>"), span("closing", "code", "</code>")];
    const coded = `${OPENING}foo${CLOSING}`;
    const html = codedTextToHtml(coded, spans);
    expect(html).toContain("<code");
    expect(html).toContain("</code>");
    expect(html).toContain("foo");
  });

  it("skips generic/unknown tags", () => {
    const spans = [span("opening", "div", "<div>"), span("closing", "div", "</div>")];
    const coded = `${OPENING}content${CLOSING}`;
    // Generic tags have no HTML mapping — they're skipped
    expect(codedTextToHtml(coded, spans)).toBe("content");
  });

  it("handles nested tags", () => {
    const spans = [
      span("opening", "b", "<b>"),
      span("opening", "i", "<i>"),
      span("closing", "i", "</i>"),
      span("closing", "b", "</b>"),
    ];
    const coded = `${OPENING}${OPENING}text${CLOSING}${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<b><i>text</i></b>");
  });
});
