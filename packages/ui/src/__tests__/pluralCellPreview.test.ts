import { describe, expect, it } from "vitest";

import { parsePluralFormForChips } from "../components/editor/pluralCellPreview";
import type { SpanInfo } from "../types/span";

const strongOpen: SpanInfo = {
  span_type: "opening",
  type: "fmt:bold",
  id: "1",
  data: "<strong>",
  equiv_text: "strong",
};
const strongClose: SpanInfo = {
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

describe("parsePluralFormForChips", () => {
  it("returns null for non-plural input", () => {
    expect(parsePluralFormForChips("Hello world", [])).toBeNull();
    expect(parsePluralFormForChips("", [])).toBeNull();
    expect(parsePluralFormForChips("{name}", [])).toBeNull();
  });

  it("picks 'other' by default when present", () => {
    const icu = "{count, plural, one {one item} other {many items}}";
    const preview = parsePluralFormForChips(icu, [])!;
    expect(preview.shownForm).toBe("other");
    expect(preview.codedText).toBe("many items");
    expect(preview.spans).toEqual([]);
  });

  it("respects preferredForm when provided", () => {
    const icu = "{count, plural, one {one item} other {many items}}";
    const preview = parsePluralFormForChips(icu, [], "one")!;
    expect(preview.shownForm).toBe("one");
    expect(preview.codedText).toBe("one item");
  });

  it("falls back to 'other' when preferredForm is missing", () => {
    const icu = "{count, plural, one {a} other {b}}";
    const preview = parsePluralFormForChips(icu, [], "few")!;
    expect(preview.shownForm).toBe("other");
  });

  it("falls back to the first available form when 'other' is also missing", () => {
    const icu = "{n, plural, one {only one}}";
    const preview = parsePluralFormForChips(icu, [], "few")!;
    expect(preview.shownForm).toBe("one");
    expect(preview.codedText).toBe("only one");
  });

  it("resolves {=equiv} / {/=equiv} markers to PcOpen + PcClose spans", () => {
    const icu = "{count, plural, other {Click {=strong}here{/=strong} for {count}}}";
    const preview = parsePluralFormForChips(icu, [strongOpen, strongClose, countPh])!;
    // Three markers in document order → three spans.
    expect(preview.spans).toHaveLength(3);
    expect(preview.spans.map((s) => s.span_type)).toEqual(["opening", "closing", "placeholder"]);
    expect(preview.spans.map((s) => s.equiv_text)).toEqual(["strong", "strong", "count"]);
    // Coded text uses the marker characters with text in between.
    expect(preview.codedText).toBe("Click \uE001here\uE002 for \uE003");
  });

  it("preserves unknown markers as literal text rather than dropping them", () => {
    const icu = "{count, plural, other {hi {unknown_var}}}";
    const preview = parsePluralFormForChips(icu, [])!;
    // {unknown_var} stays inline as text — translator sees the truth.
    expect(preview.codedText).toContain("{unknown_var}");
    expect(preview.spans).toEqual([]);
  });

  it("exposes the pivot variable name and available forms", () => {
    const icu = "{itemCount, plural, zero {none} one {1} few {a few} other {many}}";
    const preview = parsePluralFormForChips(icu, [])!;
    expect(preview.pivot).toBe("itemCount");
    expect(preview.availableForms.sort()).toEqual(["few", "one", "other", "zero"].sort());
  });
});
