import { describe, expect, it } from "vitest";

import { projectScopeLines } from "../RecipeView";
import { buildRecipe } from "../FlowBuilderRunner";

describe("projectScopeLines", () => {
  it("covers the defaults block including project presets, and stops at flows", () => {
    const recipe = buildRecipe(
      { steps: [{ tool: "redact" }, { tool: "translate" }] },
      { redact: { detectors: ["rules"] } },
    );
    const lines = recipe.split("\n");
    const scope = projectScopeLines(recipe);

    const defaultsAt = lines.indexOf("defaults:");
    const flowsAt = lines.indexOf("flows:");
    expect(defaultsAt).toBeGreaterThanOrEqual(0);
    expect(scope.has(defaultsAt)).toBe(true);
    // Every line of the defaults block (source_language, tools, the preset
    // keys) is covered; nothing at or after flows: is.
    for (let i = defaultsAt; i < flowsAt; i++) expect(scope.has(i)).toBe(true);
    expect(scope.has(flowsAt)).toBe(false);
  });

  it("is empty when the recipe has no defaults block", () => {
    expect(projectScopeLines("version: v1\nflows:\n  lab:\n").size).toBe(0);
  });
});
