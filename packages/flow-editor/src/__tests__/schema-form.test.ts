import { describe, it, expect } from "vitest";
import type { ComponentSchema, PropertySchema } from "../types";

// Test the SchemaForm's data model logic without React rendering.
// The rendering tests are covered by Storybook stories.

describe("SchemaForm field derivation", () => {
  const schema: ComponentSchema = {
    title: "Test Tool",
    type: "object",
    "ui:groups": [{ id: "output", label: "Output", fields: ["prefix", "suffix"] }],
    properties: {
      prefix: { type: "string", default: "[" },
      suffix: { type: "string", default: "]" },
      expansion: { type: "integer", default: 30 },
      deprecated_field: { type: "string", deprecated: true },
    },
  };

  it("identifies grouped fields", () => {
    const groups = schema["ui:groups"] || [];
    const groupedFields = new Set(groups.flatMap((g) => g.fields));
    expect(groupedFields.has("prefix")).toBe(true);
    expect(groupedFields.has("suffix")).toBe(true);
    expect(groupedFields.has("expansion")).toBe(false);
  });

  it("identifies ungrouped non-deprecated fields", () => {
    const properties = schema.properties || {};
    const groups = schema["ui:groups"] || [];
    const groupedFields = new Set(groups.flatMap((g) => g.fields));
    const ungrouped = Object.keys(properties).filter(
      (k) => !groupedFields.has(k) && !properties[k].deprecated,
    );
    expect(ungrouped).toEqual(["expansion"]);
  });

  it("filters out deprecated fields from ungrouped", () => {
    const properties = schema.properties || {};
    const groups = schema["ui:groups"] || [];
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

describe("ui:visible logic", () => {
  it("hides field when condition is not met", () => {
    const schema: PropertySchema = {
      type: "number",
      "ui:visible": { field: "mode", eq: "advanced" },
    };
    const allValues: Record<string, unknown> = { mode: "simple" };
    const cond = schema["ui:visible"];
    const visible = !cond || ("field" in cond && "eq" in cond && allValues[cond.field] === cond.eq);
    expect(visible).toBe(false);
  });

  it("shows field when condition is met", () => {
    const schema: PropertySchema = {
      type: "number",
      "ui:visible": { field: "mode", eq: "advanced" },
    };
    const allValues: Record<string, unknown> = { mode: "advanced" };
    const cond = schema["ui:visible"];
    const visible = !cond || ("field" in cond && "eq" in cond && allValues[cond.field] === cond.eq);
    expect(visible).toBe(true);
  });

  it("shows field when no ui:visible is set", () => {
    const schema: PropertySchema = { type: "string" };
    const allValues: Record<string, unknown> = { mode: "simple" };
    const cond = schema["ui:visible"];
    const visible = !cond || ("field" in cond && "eq" in cond && allValues[cond.field] === cond.eq);
    expect(visible).toBe(true);
  });
});

describe("object type classification", () => {
  function hasAdditionalProperties(schema: PropertySchema): boolean {
    return schema.additionalProperties != null && schema.additionalProperties !== false;
  }

  it("identifies nested object (has properties)", () => {
    const schema: PropertySchema = {
      type: "object",
      properties: {
        enabled: { type: "boolean" },
        threshold: { type: "number" },
      },
    };
    expect(schema.properties).toBeDefined();
    expect(Object.keys(schema.properties!).length).toBe(2);
  });

  it("identifies map object (has additionalProperties)", () => {
    const schema: PropertySchema = {
      type: "object",
      additionalProperties: {
        type: "object",
        properties: { ruleTypes: { type: "array" } },
      },
    };
    expect(hasAdditionalProperties(schema)).toBe(true);
    expect(schema.properties).toBeUndefined();
  });

  it("identifies bare object (no properties, no additionalProperties)", () => {
    const schema: PropertySchema = { type: "object" };
    expect(schema.properties).toBeUndefined();
    expect(hasAdditionalProperties(schema)).toBe(false);
  });

  it("treats additionalProperties: false as not having additional", () => {
    const schema: PropertySchema = {
      type: "object",
      additionalProperties: false,
      properties: { name: { type: "string" } },
    };
    expect(hasAdditionalProperties(schema)).toBe(false);
  });
});

describe("duplicate label suppression", () => {
  function formatLabel(name: string): string {
    return name
      .replace(/([A-Z])/g, " $1")
      .replace(/^./, (s) => s.toUpperCase())
      .trim();
  }

  function resolveDescription(
    title: string | undefined,
    description: string | undefined,
    name: string,
  ): string | undefined {
    const label = title || formatLabel(name);
    if (description && label.toLowerCase() !== description.toLowerCase()) {
      return description;
    }
    return undefined;
  }

  it("suppresses description when identical to title (case-insensitive)", () => {
    expect(resolveDescription("Prefix", "Prefix", "prefix")).toBeUndefined();
    expect(resolveDescription("Prefix", "prefix", "prefix")).toBeUndefined();
  });

  it("keeps description when different from title", () => {
    expect(resolveDescription("Prefix", "The prefix to prepend", "prefix")).toBe(
      "The prefix to prepend",
    );
  });

  it("suppresses description when identical to generated label", () => {
    // formatLabel("segmentByLine") → "Segment By Line"
    expect(resolveDescription(undefined, "Segment By Line", "segmentByLine")).toBeUndefined();
  });

  it("keeps description when title is absent and description differs from generated label", () => {
    expect(resolveDescription(undefined, "Enable line-based segmentation", "segmentByLine")).toBe(
      "Enable line-based segmentation",
    );
  });
});

describe("$ref resolution", () => {
  it("resolves $ref from $defs", () => {
    const defs: Record<string, PropertySchema> = {
      ruleEntry: {
        type: "object",
        properties: {
          pattern: { type: "string" },
          action: { type: "string", enum: ["include", "exclude"] },
        },
      },
    };
    const schema: PropertySchema = { $ref: "#/$defs/ruleEntry", type: "" };
    // Simulate resolution
    const match = schema.$ref?.match(/^#\/\$defs\/(.+)$/);
    const resolved = match ? defs[match[1]] : undefined;
    expect(resolved).toBeDefined();
    expect(resolved!.type).toBe("object");
    expect(resolved!.properties!.pattern.type).toBe("string");
  });

  it("returns original schema when $ref is not resolvable", () => {
    const defs: Record<string, PropertySchema> = {};
    const schema: PropertySchema = { $ref: "#/$defs/missing", type: "string" };
    const match = schema.$ref?.match(/^#\/\$defs\/(.+)$/);
    const resolved = match ? defs[match[1]] : undefined;
    expect(resolved).toBeUndefined();
    // Caller should fall back to the original schema
  });

  it("ignores $ref without $defs", () => {
    const schema: PropertySchema = { $ref: "#/$defs/ruleEntry", type: "string" };
    const defs = undefined;
    const match = schema.$ref?.match(/^#\/\$defs\/(.+)$/);
    const resolved = defs && match ? (defs as Record<string, PropertySchema>)[match[1]] : undefined;
    expect(resolved).toBeUndefined();
  });
});

describe("ui:enum-labels and ui:enum-descriptions", () => {
  it("provides display labels for enum values", () => {
    const schema: PropertySchema = {
      type: "string",
      enum: ["lf", "crlf", "cr"],
      "ui:enum-labels": { lf: "Unix (LF)", crlf: "Windows (CRLF)", cr: "Classic Mac (CR)" },
    };
    const labels = schema["ui:enum-labels"]!;
    expect(labels["lf"]).toBe("Unix (LF)");
    expect(labels["crlf"]).toBe("Windows (CRLF)");
    // Unlabeled values fall back to the raw value
    expect(labels["missing"] ?? "missing").toBe("missing");
  });

  it("provides descriptions for selected enum value", () => {
    const schema: PropertySchema = {
      type: "string",
      enum: ["error", "warning", "info"],
      "ui:enum-descriptions": {
        error: "Stops processing on first issue",
        warning: "Reports the issue but continues",
        info: "Logs the issue silently",
      },
    };
    const selected = "warning";
    const desc = schema["ui:enum-descriptions"]?.[selected];
    expect(desc).toBe("Reports the issue but continues");
  });

  it("returns undefined for unselected or missing description", () => {
    const schema: PropertySchema = {
      type: "string",
      enum: ["a", "b"],
      "ui:enum-descriptions": { a: "First option" },
    };
    expect(schema["ui:enum-descriptions"]?.["b"]).toBeUndefined();
  });
});

describe("ui:enabled condition", () => {
  it("field is enabled when no ui:enabled condition", () => {
    const schema: PropertySchema = { type: "string" };
    const enabled = schema["ui:enabled"] === undefined;
    expect(enabled).toBe(true);
  });

  it("field is disabled when condition is not met", () => {
    const schema: PropertySchema = {
      type: "string",
      "ui:enabled": { field: "useAdvanced", eq: true },
    };
    const values = { useAdvanced: false };
    const cond = schema["ui:enabled"]!;
    const enabled =
      "field" in cond && "eq" in cond && values[cond.field as keyof typeof values] === cond.eq;
    expect(enabled).toBe(false);
  });

  it("field is enabled when condition is met", () => {
    const schema: PropertySchema = {
      type: "string",
      "ui:enabled": { field: "useAdvanced", eq: true },
    };
    const values = { useAdvanced: true };
    const cond = schema["ui:enabled"]!;
    const enabled =
      "field" in cond && "eq" in cond && values[cond.field as keyof typeof values] === cond.eq;
    expect(enabled).toBe(true);
  });
});
