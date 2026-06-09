import { describe, it, expect } from "vitest";
import { evaluateCondition } from "../components/schema-form/hooks/useConditionalVisibility";
import { resolveWidgetName, WIDGET_NAMES } from "../components/schema-form/registry";
import { resolveRef, hasAdditionalProperties, humanizeKey } from "../components/schema-form/utils";
import type { PropertySchema, ConditionExpr } from "../components/schema-form/types";

// ── evaluateCondition ──────────────────────────────────────────────────

describe("evaluateCondition", () => {
  const values = { mode: "advanced", enabled: true, count: 5, empty: "" };
  const properties: Record<string, PropertySchema> = {
    mode: { type: "string", default: "simple" },
    enabled: { type: "boolean", default: false },
    count: { type: "integer", default: 0 },
    empty: { type: "string" },
  };

  it("returns true when no condition", () => {
    expect(evaluateCondition(undefined, values, properties)).toBe(true);
  });

  it("returns true when no values", () => {
    const cond: ConditionExpr = { field: "mode", eq: "x" };
    expect(evaluateCondition(cond, undefined, undefined)).toBe(true);
  });

  describe("eq condition", () => {
    it("returns true when field matches", () => {
      expect(evaluateCondition({ field: "mode", eq: "advanced" }, values, properties)).toBe(true);
    });

    it("returns false when field does not match", () => {
      expect(evaluateCondition({ field: "mode", eq: "simple" }, values, properties)).toBe(false);
    });

    it("falls back to default when value is unset", () => {
      expect(evaluateCondition({ field: "mode", eq: "simple" }, {}, properties)).toBe(true);
    });

    it("handles boolean comparison", () => {
      expect(evaluateCondition({ field: "enabled", eq: true }, values, properties)).toBe(true);
    });

    it("handles string-coerced comparison", () => {
      expect(evaluateCondition({ field: "count", eq: "5" }, values, properties)).toBe(true);
    });
  });

  describe("empty condition", () => {
    it("returns true when field is empty and empty=true", () => {
      expect(evaluateCondition({ field: "empty", empty: true }, values, properties)).toBe(true);
    });

    it("returns false when field is not empty and empty=true", () => {
      expect(evaluateCondition({ field: "mode", empty: true }, values, properties)).toBe(false);
    });

    it("returns true when field is not empty and empty=false", () => {
      expect(evaluateCondition({ field: "mode", empty: false }, values, properties)).toBe(true);
    });

    it("treats undefined as empty", () => {
      expect(evaluateCondition({ field: "missing", empty: true }, values, properties)).toBe(true);
    });

    it("treats null as empty", () => {
      expect(evaluateCondition({ field: "empty", empty: true }, { empty: null }, properties)).toBe(
        true,
      );
    });
  });

  describe("compound conditions", () => {
    it("all: returns true when all conditions match", () => {
      const cond: ConditionExpr = {
        all: [
          { field: "mode", eq: "advanced" },
          { field: "enabled", eq: true },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("all: returns false when any condition fails", () => {
      const cond: ConditionExpr = {
        all: [
          { field: "mode", eq: "advanced" },
          { field: "enabled", eq: false },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("any: returns true when at least one matches", () => {
      const cond: ConditionExpr = {
        any: [
          { field: "mode", eq: "simple" },
          { field: "enabled", eq: true },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });

    it("any: returns false when none match", () => {
      const cond: ConditionExpr = {
        any: [
          { field: "mode", eq: "simple" },
          { field: "enabled", eq: false },
        ],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(false);
    });

    it("not: inverts condition", () => {
      expect(evaluateCondition({ not: { field: "mode", eq: "simple" } }, values, properties)).toBe(
        true,
      );
      expect(
        evaluateCondition({ not: { field: "mode", eq: "advanced" } }, values, properties),
      ).toBe(false);
    });

    it("nested compound", () => {
      const cond: ConditionExpr = {
        all: [{ field: "enabled", eq: true }, { not: { field: "mode", eq: "simple" } }],
      };
      expect(evaluateCondition(cond, values, properties)).toBe(true);
    });
  });
});

// ── resolveWidgetName ──────────────────────────────────────────────────

describe("resolveWidgetName", () => {
  it("returns undefined for undefined input", () => {
    expect(resolveWidgetName(undefined)).toBeUndefined();
  });

  it("passes through canonical names", () => {
    for (const name of WIDGET_NAMES) {
      expect(resolveWidgetName(name)).toBe(name);
    }
  });

  it("resolves legacy aliases", () => {
    expect(resolveWidgetName("codeFinderRules")).toBe("code-finder");
    expect(resolveWidgetName("multilineText")).toBe("textarea");
    expect(resolveWidgetName("regexBuilder")).toBe("regex");
    expect(resolveWidgetName("tagList")).toBe("tags");
    expect(resolveWidgetName("numberList")).toBe("number-list");
    expect(resolveWidgetName("checkList")).toBe("checklist");
    expect(resolveWidgetName("simplifierRulesEditor")).toBe("simplifier-rules");
    expect(resolveWidgetName("elementRulesEditor")).toBe("element-rules");
    expect(resolveWidgetName("attributeRulesEditor")).toBe("attribute-rules");
  });

  it("passes through unknown names unchanged", () => {
    expect(resolveWidgetName("custom-widget")).toBe("custom-widget");
  });
});

// ── utils ──────────────────────────────────────────────────────────────

describe("resolveRef", () => {
  it("returns additionalProperties schema when it is an object", () => {
    const schema: PropertySchema = {
      type: "object",
      additionalProperties: { type: "string" },
    };
    expect(resolveRef(schema)).toEqual({ type: "string" });
  });

  it("returns undefined when additionalProperties is true", () => {
    const schema: PropertySchema = { type: "object", additionalProperties: true };
    expect(resolveRef(schema)).toBeUndefined();
  });

  it("returns undefined when no additionalProperties", () => {
    const schema: PropertySchema = { type: "object" };
    expect(resolveRef(schema)).toBeUndefined();
  });
});

describe("hasAdditionalProperties", () => {
  it("returns true for object additionalProperties", () => {
    expect(
      hasAdditionalProperties({ type: "object", additionalProperties: { type: "string" } }),
    ).toBe(true);
  });

  it("returns true for boolean true", () => {
    expect(hasAdditionalProperties({ type: "object", additionalProperties: true })).toBe(true);
  });

  it("returns false for boolean false", () => {
    expect(hasAdditionalProperties({ type: "object", additionalProperties: false })).toBe(false);
  });

  it("returns false when undefined", () => {
    expect(hasAdditionalProperties({ type: "object" })).toBe(false);
  });
});

// ── humanizeKey ────────────────────────────────────────────────────────

describe("humanizeKey", () => {
  it("splits camelCase into title-cased words", () => {
    expect(humanizeKey("checkLeadingWhitespace")).toBe("Check Leading Whitespace");
  });

  it("splits snake_case and kebab-case", () => {
    expect(humanizeKey("target_language")).toBe("Target Language");
    expect(humanizeKey("source-locale")).toBe("Source Locale");
  });

  it("keeps acronym runs together", () => {
    expect(humanizeKey("enableTMLookup")).toBe("Enable TM Lookup");
  });

  it("title-cases a single lowercase word", () => {
    expect(humanizeKey("model")).toBe("Model");
  });
});
