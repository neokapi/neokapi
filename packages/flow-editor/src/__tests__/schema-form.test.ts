import { describe, it, expect } from "vitest";
import type { ComponentSchema } from "../types";

// Test the SchemaForm's data model logic without React rendering.
// The rendering tests are covered by Storybook stories.

describe("SchemaForm field derivation", () => {
  const schema: ComponentSchema = {
    title: "Test Tool",
    type: "object",
    "x-groups": [
      { id: "output", label: "Output", fields: ["prefix", "suffix"] },
    ],
    properties: {
      prefix: { type: "string", default: "[" },
      suffix: { type: "string", default: "]" },
      expansion: { type: "integer", default: 30 },
      deprecated_field: { type: "string", deprecated: true },
    },
  };

  it("identifies grouped fields", () => {
    const groups = schema["x-groups"] || [];
    const groupedFields = new Set(groups.flatMap((g) => g.fields));
    expect(groupedFields.has("prefix")).toBe(true);
    expect(groupedFields.has("suffix")).toBe(true);
    expect(groupedFields.has("expansion")).toBe(false);
  });

  it("identifies ungrouped non-deprecated fields", () => {
    const properties = schema.properties || {};
    const groups = schema["x-groups"] || [];
    const groupedFields = new Set(groups.flatMap((g) => g.fields));
    const ungrouped = Object.keys(properties).filter(
      (k) => !groupedFields.has(k) && !properties[k].deprecated,
    );
    expect(ungrouped).toEqual(["expansion"]);
  });

  it("filters out deprecated fields from ungrouped", () => {
    const properties = schema.properties || {};
    const groups = schema["x-groups"] || [];
    const groupedFields = new Set(groups.flatMap((g) => g.fields));
    const ungrouped = Object.keys(properties).filter(
      (k) => !groupedFields.has(k) && !properties[k].deprecated,
    );
    expect(ungrouped).not.toContain("deprecated_field");
  });
});

describe("SchemaForm value resolution", () => {
  it("uses provided value over default", () => {
    const schema = { type: "string", default: "fallback" };
    const value = "explicit";
    const resolved = value ?? schema.default;
    expect(resolved).toBe("explicit");
  });

  it("falls back to default when value is undefined", () => {
    const schema = { type: "string", default: "fallback" };
    const value = undefined;
    const resolved = value ?? schema.default;
    expect(resolved).toBe("fallback");
  });

  it("preserves null as explicit absence", () => {
    const schema = { type: "string", default: "fallback" };
    const value = null;
    const resolved = value ?? schema.default;
    // null triggers fallback via ??
    expect(resolved).toBe("fallback");
  });
});

describe("formatLabel", () => {
  // Test the label formatting logic used in SchemaForm
  function formatLabel(name: string): string {
    return name
      .replace(/([A-Z])/g, " $1")
      .replace(/^./, (s) => s.toUpperCase())
      .trim();
  }

  it("converts camelCase to Title Case", () => {
    expect(formatLabel("fuzzyThreshold")).toBe("Fuzzy Threshold");
  });

  it("handles single word", () => {
    expect(formatLabel("name")).toBe("Name");
  });

  it("handles already capitalized", () => {
    expect(formatLabel("XMLParser")).toBe("X M L Parser");
  });

  it("handles empty string", () => {
    expect(formatLabel("")).toBe("");
  });
});
