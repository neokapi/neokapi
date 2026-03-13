import { describe, it, expect } from "vite-plus/test";
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

function span(
  spanType: "opening" | "closing" | "placeholder",
  type: string,
  data: string,
): SpanInfo {
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
  it("maps fmt:bold to formatting", () => {
    expect(semanticCategory(span("opening", "fmt:bold", "<b>"))).toBe("formatting");
  });

  it("maps fmt:italic to formatting", () => {
    expect(semanticCategory(span("opening", "fmt:italic", "<i>"))).toBe("formatting");
  });

  it("maps link:hyperlink to linking", () => {
    expect(semanticCategory(span("opening", "link:hyperlink", "<a>"))).toBe("linking");
  });

  it("maps fmt:code to formatting", () => {
    expect(semanticCategory(span("opening", "fmt:code", "<code>"))).toBe("formatting");
  });

  it("maps struct:break to structure", () => {
    expect(semanticCategory(span("placeholder", "struct:break", "<br/>"))).toBe("structure");
  });

  it("falls back to generic for unknown types", () => {
    expect(semanticCategory(span("opening", "custom:unknown", "<span>"))).toBe("generic");
  });
});

// ---------------------------------------------------------------------------
// semanticLabel
// ---------------------------------------------------------------------------
describe("semanticLabel", () => {
  it("returns 'B>' for bold opening", () => {
    expect(semanticLabel(span("opening", "fmt:bold", "<b>"))).toBe("B>");
  });

  it("returns '/B' for bold closing", () => {
    expect(semanticLabel(span("closing", "fmt:bold", "</b>"))).toBe("/B");
  });

  it("returns 'I>' for italic opening", () => {
    expect(semanticLabel(span("opening", "fmt:italic", "<i>"))).toBe("I>");
  });

  it("returns 'br' for break placeholder", () => {
    expect(semanticLabel(span("placeholder", "struct:break", "<br/>"))).toBe("br");
  });

  it("returns 'a>' for link opening", () => {
    expect(semanticLabel(span("opening", "link:hyperlink", "<a>"))).toBe("a>");
  });

  it("uses display_text from span when available", () => {
    const s: SpanInfo = {
      span_type: "opening",
      type: "fmt:bold",
      id: "1",
      data: "<b>",
      display_text: "[BOLD]",
    };
    expect(semanticLabel(s)).toBe("[BOLD]");
  });

  it("handles unknown types with fallback labels", () => {
    const s = span("opening", "custom:unknown", "<span>");
    const label = semanticLabel(s);
    expect(label).toBe("?>");
  });
});

// ---------------------------------------------------------------------------
// semanticTooltip
// ---------------------------------------------------------------------------
describe("semanticTooltip", () => {
  it("generates tooltip for bold opening", () => {
    const tooltip = semanticTooltip(span("opening", "fmt:bold", "<b>"));
    expect(tooltip).toBe("Bold open — <b>");
  });

  it("generates tooltip for italic closing", () => {
    const tooltip = semanticTooltip(span("closing", "fmt:italic", "</i>"));
    expect(tooltip).toBe("Italic close — </i>");
  });

  it("generates tooltip for break placeholder", () => {
    const tooltip = semanticTooltip(span("placeholder", "struct:break", "<br/>"));
    expect(tooltip).toBe("Line Break placeholder — <br/>");
  });
});

// ---------------------------------------------------------------------------
// tagColors
// ---------------------------------------------------------------------------
describe("tagColors", () => {
  it("returns color scheme with bg, border, text", () => {
    const colors = tagColors(span("opening", "fmt:bold", "<b>"));
    expect(colors).toHaveProperty("bg");
    expect(colors).toHaveProperty("border");
    expect(colors).toHaveProperty("text");
  });

  it("returns different colors for different types", () => {
    const boldColors = tagColors(span("opening", "fmt:bold", "<b>"));
    const italicColors = tagColors(span("opening", "fmt:italic", "<i>"));
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
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(2);
    expect(pairs.get(0)!.pairIndex).toBe(pairs.get(1)!.pairIndex);
  });

  it("handles nested pairs", () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("opening", "fmt:italic", "<i>"),
      span("closing", "fmt:italic", "</i>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const pairs = buildPairs(spans);

    const bPair = pairs.get(0)!.pairIndex;
    const iPair = pairs.get(1)!.pairIndex;
    expect(bPair).not.toBe(iPair);
    expect(pairs.get(3)!.pairIndex).toBe(bPair);
    expect(pairs.get(2)!.pairIndex).toBe(iPair);
  });

  it("handles same-type nesting (stack behavior)", () => {
    const spans: SpanInfo[] = [
      span("opening", "fmt:bold", "<b>"),
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const pairs = buildPairs(spans);

    expect(pairs.get(1)!.pairIndex).toBe(pairs.get(2)!.pairIndex);
    expect(pairs.get(0)!.pairIndex).toBe(pairs.get(3)!.pairIndex);
  });

  it("assigns standalone pair to placeholders", () => {
    const spans: SpanInfo[] = [span("placeholder", "struct:break", "<br/>")];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(1);
    expect(pairs.get(0)!.pairIndex).toBe(1);
  });

  it("assigns standalone pair to unmatched closing", () => {
    const spans: SpanInfo[] = [span("closing", "fmt:bold", "</b>")];
    const pairs = buildPairs(spans);

    expect(pairs.size).toBe(1);
    expect(pairs.get(0)!.pairIndex).toBeGreaterThan(0);
  });
});

// ---------------------------------------------------------------------------
// validateTags
// ---------------------------------------------------------------------------
describe("validateTags", () => {
  it("returns valid for matching spans", () => {
    const source = [span("opening", "fmt:bold", "<b>"), span("closing", "fmt:bold", "</b>")];
    const target = [span("opening", "fmt:bold", "<b>"), span("closing", "fmt:bold", "</b>")];
    const result = validateTags(source, target);

    expect(result.valid).toBe(true);
    expect(result.errors).toHaveLength(0);
    expect(result.warnings).toHaveLength(0);
  });

  it("returns valid for empty spans", () => {
    expect(validateTags([], []).valid).toBe(true);
  });

  it("reports missing tags", () => {
    const source = [span("opening", "fmt:bold", "<b>"), span("closing", "fmt:bold", "</b>")];
    const target: SpanInfo[] = [];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.length).toBeGreaterThan(0);
    expect(result.errors.some((e) => e.type === "missing_tag")).toBe(true);
  });

  it("reports extra tags as warnings", () => {
    const source: SpanInfo[] = [];
    const target = [span("opening", "fmt:bold", "<b>"), span("closing", "fmt:bold", "</b>")];
    const result = validateTags(source, target);

    expect(result.warnings.some((w) => w.type === "extra_tag")).toBe(true);
  });

  it("reports unpaired closing tags in target", () => {
    const source = [span("closing", "fmt:bold", "</b>")];
    const target = [span("closing", "fmt:bold", "</b>")];
    const result = validateTags(source, target);

    expect(result.errors.some((e) => e.type === "unpaired")).toBe(true);
  });

  it("reports unpaired opening tags in target", () => {
    const source = [span("opening", "fmt:bold", "<b>")];
    const target = [span("opening", "fmt:bold", "<b>")];
    const result = validateTags(source, target);

    expect(result.errors.some((e) => e.type === "unpaired")).toBe(true);
  });

  it("reports deleted_non_deletable for missing non-deletable tag", () => {
    const source = [span("placeholder", "struct:break", "<br/>")];
    const target: SpanInfo[] = [];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.type === "deleted_non_deletable")).toBe(true);
    expect(result.errors.some((e) => e.type === "missing_tag")).toBe(false);
  });

  it("reports missing_tag (not deleted_non_deletable) for deletable tag", () => {
    const source = [span("opening", "fmt:bold", "<b>")];
    const target: SpanInfo[] = [];
    const result = validateTags(source, target);

    expect(result.errors.some((e) => e.type === "missing_tag")).toBe(true);
    expect(result.errors.some((e) => e.type === "deleted_non_deletable")).toBe(false);
  });

  it("reports cloned_non_cloneable for duplicated non-cloneable tag", () => {
    const source = [span("placeholder", "code:variable", "{x}")];
    const target = [
      span("placeholder", "code:variable", "{x}"),
      span("placeholder", "code:variable", "{x}"),
    ];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.type === "cloned_non_cloneable")).toBe(true);
    expect(result.warnings.some((w) => w.type === "extra_tag")).toBe(false);
  });

  it("reports extra_tag (not cloned_non_cloneable) for cloneable tag", () => {
    const source: SpanInfo[] = [];
    const target = [span("opening", "fmt:bold", "<b>")];
    const result = validateTags(source, target);

    expect(result.warnings.some((w) => w.type === "extra_tag")).toBe(true);
    expect(result.errors.some((e) => e.type === "cloned_non_cloneable")).toBe(false);
  });

  it("handles mixed valid and invalid tags", () => {
    const source = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
      span("placeholder", "struct:break", "<br/>"),
    ];
    const target = [
      span("opening", "fmt:bold", "<b>"),
      span("closing", "fmt:bold", "</b>"),
      // missing break placeholder
    ];
    const result = validateTags(source, target);

    expect(result.valid).toBe(false);
    expect(result.errors.some((e) => e.message.includes("Line Break"))).toBe(true);
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
    const spans = [span("opening", "fmt:bold", "<b>"), span("closing", "fmt:bold", "</b>")];
    const coded = `${OPENING}bold${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<b>bold</b>");
  });

  it("wraps italic text in <i> tags", () => {
    const spans = [span("opening", "fmt:italic", "<i>"), span("closing", "fmt:italic", "</i>")];
    const coded = `${OPENING}italic${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<i>italic</i>");
  });

  it("renders break placeholder using text equivalent", () => {
    const spans = [span("placeholder", "struct:break", "<br/>")];
    const coded = `line one${PLACEHOLDER}line two`;
    // struct:break has equiv "\n", which gets escaped to text
    expect(codedTextToHtml(coded, spans)).toContain("line one");
    expect(codedTextToHtml(coded, spans)).toContain("line two");
  });

  it("renders image placeholder as [Image]", () => {
    const spans = [span("placeholder", "media:image", "<img/>")];
    const coded = `text ${PLACEHOLDER} more`;
    expect(codedTextToHtml(coded, spans)).toContain("[Image]");
  });

  it("renders code spans", () => {
    const spans = [span("opening", "fmt:code", "<code>"), span("closing", "fmt:code", "</code>")];
    const coded = `${OPENING}foo${CLOSING}`;
    const html = codedTextToHtml(coded, spans);
    expect(html).toContain("<code");
    expect(html).toContain("</code>");
    expect(html).toContain("foo");
  });

  it("skips unknown types (text flows through)", () => {
    const spans = [
      span("opening", "custom:unknown", "<div>"),
      span("closing", "custom:unknown", "</div>"),
    ];
    const coded = `${OPENING}content${CLOSING}`;
    // Unknown types use fallback HTML (<span data-type="...">)
    const html = codedTextToHtml(coded, spans);
    expect(html).toContain("content");
  });

  it("handles nested tags", () => {
    const spans = [
      span("opening", "fmt:bold", "<b>"),
      span("opening", "fmt:italic", "<i>"),
      span("closing", "fmt:italic", "</i>"),
      span("closing", "fmt:bold", "</b>"),
    ];
    const coded = `${OPENING}${OPENING}text${CLOSING}${CLOSING}`;
    expect(codedTextToHtml(coded, spans)).toBe("<b><i>text</i></b>");
  });
});
