import { describe, it, expect } from "vitest";
import { evaluateCondition } from "@neokapi/ui-primitives/components/schema-form/hooks/useConditionalVisibility";
import { resolveWidgetName, WIDGET_NAMES } from "@neokapi/ui-primitives/components/schema-form/registry";
import type { PropertySchema, ConditionExpr } from "../types";

// ── evaluateCondition ──────────────────────────────────────────────────

describe("evaluateCondition", () => {
  const values = { mode: "advanced", enabled: true, count: 5, empty: "" };
  const properties: Record<string, PropertySchema> = {
    mode: { type: "string", default: "simple" },
    enabled: { type: "boolean", default: false },
    count: { type: "integer", default: 0 },
    empty: { type: "string" },
  };

  describe("simple eq condition", () => {
    it("returns true when field matches", () => {
      const cond: ConditionExpr = { field: "mode", eq: "advanced" };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("returns false when field does not match", () => {
      const cond: ConditionExpr = { field: "mode", eq: "simple" };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("falls back to default when value is unset", () => {
      const cond: ConditionExpr = { field: "mode", eq: "simple" };
      expect(evaluateCondition(cond, {}, properties)).toBe(true); // default is "simple"
    });

    it("handles boolean comparison", () => {
      const cond: ConditionExpr = { field: "enabled", eq: true };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });
  });

  describe("empty condition", () => {
    it("returns true when field is empty and empty=true", () => {
      const cond: ConditionExpr = { field: "empty", empty: true };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("returns false when field is not empty and empty=true", () => {
      const cond: ConditionExpr = { field: "mode", empty: true };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("returns true when field is not empty and empty=false", () => {
      const cond: ConditionExpr = { field: "mode", empty: false };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("treats undefined as empty", () => {
      const cond: ConditionExpr = { field: "missing", empty: true };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("treats null as empty", () => {
      const cond: ConditionExpr = { field: "nullField", empty: true };
      expect(evaluateCondition(cond, { nullField: null }, properties)).toBe(true);
    });
  });

  describe("compound: all (AND)", () => {
    it("returns true when all conditions match", () => {
      const cond: ConditionExpr = {
        all: [
          { field: "mode", eq: "advanced" },
          { field: "enabled", eq: true },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("returns false when any condition fails", () => {
      const cond: ConditionExpr = {
        all: [
          { field: "mode", eq: "advanced" },
          { field: "enabled", eq: false },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("handles empty all array", () => {
      const cond: ConditionExpr = { all: [] };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });
  });

  describe("compound: any (OR)", () => {
    it("returns true when any condition matches", () => {
      const cond: ConditionExpr = {
        any: [
          { field: "mode", eq: "simple" },
          { field: "mode", eq: "advanced" },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("returns false when no condition matches", () => {
      const cond: ConditionExpr = {
        any: [
          { field: "mode", eq: "simple" },
          { field: "mode", eq: "expert" },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("handles empty any array", () => {
      const cond: ConditionExpr = { any: [] };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });
  });

  describe("compound: not (NOT)", () => {
    it("negates a true condition", () => {
      const cond: ConditionExpr = {
        not: { field: "mode", eq: "advanced" },
      };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("negates a false condition", () => {
      const cond: ConditionExpr = {
        not: { field: "mode", eq: "simple" },
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });
  });

  describe("nested compound", () => {
    it("handles all + not", () => {
      const cond: ConditionExpr = {
        all: [
          { field: "enabled", eq: true },
          { not: { field: "mode", eq: "simple" } },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("handles any + all", () => {
      const cond: ConditionExpr = {
        any: [
          { all: [{ field: "mode", eq: "simple" }, { field: "enabled", eq: true }] },
          { field: "count", eq: "5" }, // string comparison of number
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true); // "5" == "5"
    });
  });

  describe("edge cases", () => {
    it("returns true when condition is undefined", () => {
      expect(evaluateCondition(undefined, values, properties)).toBe(true);
    });

    it("returns true when allValues is undefined", () => {
      const cond: ConditionExpr = { field: "mode", eq: "advanced" };
      expect(evaluateCondition(cond, undefined, properties)).toBe(true);
    });
  });
});

// schemaToZod tests removed — Zod validation now lives in the SchemaForm package.

// ── resolveWidgetName ──────────────────────────────────────────────────

describe("resolveWidgetName", () => {
  it("returns undefined for undefined input", () => {
    expect(resolveWidgetName(undefined)).toBeUndefined();
  });

  it("passes through canonical widget names", () => {
    expect(resolveWidgetName("textarea")).toBe("textarea");
    expect(resolveWidgetName("code-finder")).toBe("code-finder");
    expect(resolveWidgetName("file-picker")).toBe("file-picker");
    expect(resolveWidgetName("checklist")).toBe("checklist");
  });

  it("resolves legacy aliases", () => {
    expect(resolveWidgetName("multilineText")).toBe("textarea");
    expect(resolveWidgetName("codeFinderRules")).toBe("code-finder");
    expect(resolveWidgetName("simplifierRulesEditor")).toBe("simplifier-rules");
    expect(resolveWidgetName("elementRulesEditor")).toBe("element-rules");
    expect(resolveWidgetName("attributeRulesEditor")).toBe("attribute-rules");
    expect(resolveWidgetName("regexBuilder")).toBe("regex");
    expect(resolveWidgetName("tagList")).toBe("tags");
    expect(resolveWidgetName("numberList")).toBe("number-list");
    expect(resolveWidgetName("checkList")).toBe("checklist");
  });

  it("passes through unknown widget names", () => {
    expect(resolveWidgetName("custom-widget")).toBe("custom-widget");
  });
});

describe("WIDGET_NAMES", () => {
  it("contains all canonical widget names", () => {
    const expected = [
      "text", "textarea", "password", "code-editor", "regex", "tags",
      "number-list", "segmented", "file-picker", "folder-picker",
      "checklist", "select", "code-finder", "element-rules",
      "attribute-rules", "simplifier-rules",
    ];
    for (const name of expected) {
      expect(WIDGET_NAMES).toContain(name);
    }
  });
});

// ── ui:order sorting ───────────────────────────────────────────────────

describe("ui:order sorting", () => {
  it("sorts fields by ui:order", () => {
    const properties: Record<string, PropertySchema> = {
      c: { type: "string", "ui:order": 3 },
      a: { type: "string", "ui:order": 1 },
      b: { type: "string", "ui:order": 2 },
    };
    const sorted = Object.keys(properties).sort((a, b) => {
      const orderA = properties[a]?.["ui:order"] ?? Infinity;
      const orderB = properties[b]?.["ui:order"] ?? Infinity;
      return orderA - orderB;
    });
    expect(sorted).toEqual(["a", "b", "c"]);
  });

  it("puts unordered fields last", () => {
    const properties: Record<string, PropertySchema> = {
      unordered: { type: "string" },
      first: { type: "string", "ui:order": 1 },
      second: { type: "string", "ui:order": 2 },
    };
    const sorted = Object.keys(properties).sort((a, b) => {
      const orderA = properties[a]?.["ui:order"] ?? Infinity;
      const orderB = properties[b]?.["ui:order"] ?? Infinity;
      return orderA - orderB;
    });
    expect(sorted).toEqual(["first", "second", "unordered"]);
  });

  it("preserves original order when no ui:order is set", () => {
    const properties: Record<string, PropertySchema> = {
      b: { type: "string" },
      a: { type: "string" },
      c: { type: "string" },
    };
    const sorted = Object.keys(properties).sort((a, b) => {
      const orderA = properties[a]?.["ui:order"] ?? Infinity;
      const orderB = properties[b]?.["ui:order"] ?? Infinity;
      return orderA - orderB;
    });
    // All Infinity, so sort is stable (preserves insertion order)
    expect(sorted).toEqual(["b", "a", "c"]);
  });
});
