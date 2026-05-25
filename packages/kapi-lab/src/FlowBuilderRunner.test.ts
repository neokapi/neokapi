import { describe, expect, it } from "vitest";
import { buildRecipe, buildToolInfos, STARTER_SOURCE_TRANSFORMS } from "./FlowBuilderRunner";
import type { FlowSpec } from "@neokapi/flow-editor";

// ── buildRecipe ──────────────────────────────────────────────────────────────

describe("buildRecipe", () => {
  it("emits source_transforms: before steps: when sourceTransforms are present", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "search-replace" }],
      steps: [{ tool: "ai-translate" }],
    };
    const recipe = buildRecipe(spec);
    const stIdx = recipe.indexOf("source_transforms:");
    const stepsIdx = recipe.indexOf("steps:");
    expect(stIdx).toBeGreaterThan(-1);
    expect(stepsIdx).toBeGreaterThan(stIdx);
  });

  it("does NOT emit source_transforms: when the list is empty", () => {
    const spec: FlowSpec = {
      sourceTransforms: [],
      steps: [{ tool: "word-count" }],
    };
    const recipe = buildRecipe(spec);
    expect(recipe).not.toContain("source_transforms:");
    expect(recipe).toContain("steps:");
  });

  it("serializes search-replace pairs config into the source_transforms block", () => {
    const spec: FlowSpec = {
      sourceTransforms: [
        {
          tool: "search-replace",
          config: {
            pairs: [{ search: "color", replace: "colour" }],
            source: true,
            target: false,
          },
        },
      ],
      steps: [{ tool: "word-count" }],
    };
    const recipe = buildRecipe(spec);
    // The pairs array must appear as JSON inside the YAML config block.
    expect(recipe).toContain('"color"');
    expect(recipe).toContain('"colour"');
    expect(recipe).toContain("pairs:");
    // source: true / target: false
    expect(recipe).toContain("source: true");
    expect(recipe).toContain("target: false");
  });

  it("serializes redact rules inline into the source_transforms block", () => {
    const spec: FlowSpec = {
      sourceTransforms: [
        {
          tool: "redact",
          config: {
            detectors: ["rules"],
            rules: [
              { term: "Acme Corp", category: "org" },
              { term: "Jane Doe", category: "person" },
            ],
          },
        },
      ],
      steps: [{ tool: "ai-translate" }],
    };
    const recipe = buildRecipe(spec);
    expect(recipe).toContain("- tool: redact");
    expect(recipe).toContain("rules:");
    // JSON-serialized arrays contain the term strings.
    expect(recipe).toContain("Acme Corp");
    expect(recipe).toContain("Jane Doe");
    expect(recipe).toContain("detectors:");
  });

  it("uses the starter source transforms and produces a well-formed recipe", () => {
    const spec: FlowSpec = {
      sourceTransforms: STARTER_SOURCE_TRANSFORMS,
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    };
    const recipe = buildRecipe(spec);
    // Structural checks.
    expect(recipe).toContain("version: v1");
    expect(recipe).toContain("flows:");
    expect(recipe).toContain("  lab:");
    // Source-transform stage.
    expect(recipe).toContain("    source_transforms:");
    expect(recipe).toContain("      - tool: search-replace");
    expect(recipe).toContain("      - tool: redact");
    // source_transforms: appears before steps:.
    expect(recipe.indexOf("source_transforms:")).toBeLessThan(recipe.indexOf("steps:"));
    // Main steps.
    expect(recipe).toContain("    steps:");
    expect(recipe).toContain("      - tool: ai-translate");
    expect(recipe).toContain("      - tool: qa-check");
  });
});

// ── buildToolInfos ───────────────────────────────────────────────────────────

describe("buildToolInfos", () => {
  it("flags search-replace as isSourceTransform", () => {
    const infos = buildToolInfos();
    const sr = infos.find((t) => t.name === "search-replace");
    expect(sr).toBeDefined();
    expect(sr?.isSourceTransform).toBe(true);
  });

  it("flags redact as isSourceTransform", () => {
    const infos = buildToolInfos();
    const redact = infos.find((t) => t.name === "redact");
    expect(redact).toBeDefined();
    expect(redact?.isSourceTransform).toBe(true);
  });

  it("does NOT flag ai-translate as isSourceTransform", () => {
    const infos = buildToolInfos();
    const ait = infos.find((t) => t.name === "ai-translate");
    expect(ait).toBeDefined();
    expect(ait?.isSourceTransform).toBeFalsy();
  });

  it("does NOT flag word-count as isSourceTransform", () => {
    const infos = buildToolInfos();
    const wc = infos.find((t) => t.name === "word-count");
    expect(wc).toBeDefined();
    expect(wc?.isSourceTransform).toBeFalsy();
  });

  it("maps search-replace and redact to the transform category", () => {
    const infos = buildToolInfos();
    for (const id of ["search-replace", "redact"]) {
      const info = infos.find((t) => t.name === id);
      expect(info?.category).toBe("transform");
    }
  });
});
