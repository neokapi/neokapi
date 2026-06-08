import { describe, it, expect } from "vitest";
import { FLOW_TEMPLATES } from "../templates";
import type { ToolCategory } from "../types";

describe("FlowTemplateLibrary data layer", () => {
  it("FLOW_TEMPLATES has the expected count", () => {
    expect(FLOW_TEMPLATES.length).toBe(6);
  });

  it("each template has a valid category", () => {
    const validCategories: ToolCategory[] = [
      "translation",
      "quality",
      "analysis",
      "text-processing",
      "convert",
      "pipeline",
    ];
    for (const t of FLOW_TEMPLATES) {
      expect(validCategories).toContain(t.category);
    }
  });

  it("filter by category returns correct subset", () => {
    const translateTemplates = FLOW_TEMPLATES.filter((t) => t.category === "translation");
    expect(translateTemplates.length).toBeGreaterThan(0);
    for (const t of translateTemplates) {
      expect(t.category).toBe("translation");
    }
  });

  it("filter by category 'quality' returns quality templates", () => {
    const validateTemplates = FLOW_TEMPLATES.filter((t) => t.category === "quality");
    expect(validateTemplates.length).toBeGreaterThan(0);
    for (const t of validateTemplates) {
      expect(t.category).toBe("quality");
    }
  });

  it("filter by non-existent category returns empty", () => {
    const noneTemplates = FLOW_TEMPLATES.filter(
      (t) => t.category === ("nonexistent" as ToolCategory),
    );
    expect(noneTemplates.length).toBe(0);
  });

  it("null filter returns all templates", () => {
    const filter: string | null = null;
    const filtered = filter ? FLOW_TEMPLATES.filter((t) => t.category === filter) : FLOW_TEMPLATES;
    expect(filtered.length).toBe(FLOW_TEMPLATES.length);
  });

  it("each template has non-empty description", () => {
    for (const t of FLOW_TEMPLATES) {
      expect(t.description.length).toBeGreaterThan(10);
    }
  });

  it("each template spec has at least one step", () => {
    for (const t of FLOW_TEMPLATES) {
      expect(t.spec.steps.length).toBeGreaterThanOrEqual(1);
    }
  });
});
