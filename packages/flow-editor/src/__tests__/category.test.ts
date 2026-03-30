import { describe, it, expect } from "vitest";
import { getCategoryStyle, getCategoryColor, ALL_CATEGORIES } from "../category";

describe("getCategoryStyle", () => {
  it("returns style for known categories", () => {
    const style = getCategoryStyle("translate");
    expect(style.label).toBe("Translate");
    expect(style.color).toBeTruthy();
    expect(style.bg).toBeTruthy();
    expect(style.text).toBeTruthy();
    expect(style.icon).toBeTruthy();
  });

  it("returns pipeline style for unknown categories", () => {
    const style = getCategoryStyle("nonexistent");
    expect(style.label).toBe("Pipeline");
  });

  it("returns pipeline style for empty string", () => {
    const style = getCategoryStyle("");
    expect(style.label).toBe("Pipeline");
  });

  it.each(["translate", "validate", "transform", "convert", "enrich", "pipeline"])(
    "returns unique style for %s",
    (category) => {
      const style = getCategoryStyle(category);
      expect(style.label).toBeTruthy();
      expect(style.color).toContain("oklch");
    },
  );
});

describe("getCategoryColor", () => {
  it("returns the color string for a known category", () => {
    const color = getCategoryColor("validate");
    expect(color).toContain("oklch");
  });

  it("falls back to pipeline color for unknown category", () => {
    expect(getCategoryColor("unknown")).toBe(getCategoryColor("pipeline"));
  });
});

describe("ALL_CATEGORIES", () => {
  it("contains all 6 categories", () => {
    expect(ALL_CATEGORIES).toHaveLength(6);
  });

  it("has unique IDs", () => {
    const ids = ALL_CATEGORIES.map((c) => c.id);
    expect(new Set(ids).size).toBe(ids.length);
  });

  it("includes translate and validate", () => {
    const ids = ALL_CATEGORIES.map((c) => c.id);
    expect(ids).toContain("translate");
    expect(ids).toContain("validate");
  });
});
