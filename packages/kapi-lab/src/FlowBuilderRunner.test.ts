import { describe, expect, it } from "vitest";
import { buildRecipe, buildToolInfos, STARTER_STEPS } from "./FlowBuilderRunner";
import type { FlowSpec } from "@neokapi/flow-editor";

// ── buildRecipe ──────────────────────────────────────────────────────────────

describe("buildRecipe", () => {
  it("emits transformers as plain ordered steps (no source_transforms block)", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "search-replace" }, { tool: "ai-translate" }],
    };
    const recipe = buildRecipe(spec);
    expect(recipe).not.toContain("source_transforms:");
    expect(recipe).toContain("steps:");
    // Order is preserved: the transformer precedes the translation.
    expect(recipe.indexOf("- tool: search-replace")).toBeLessThan(
      recipe.indexOf("- tool: ai-translate"),
    );
  });

  it("serializes search-replace pairs config into its step", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "search-replace",
          config: {
            pairs: [{ search: "color", replace: "colour" }],
            source: true,
            target: false,
          },
        },
        { tool: "word-count" },
      ],
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

  it("serializes redact rules inline into its step", () => {
    const spec: FlowSpec = {
      steps: [
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
        { tool: "ai-translate" },
      ],
    };
    const recipe = buildRecipe(spec);
    expect(recipe).toContain("- tool: redact");
    expect(recipe).toContain("rules:");
    // JSON-serialized arrays contain the term strings.
    expect(recipe).toContain("Acme Corp");
    expect(recipe).toContain("Jane Doe");
    expect(recipe).toContain("detectors:");
  });

  it("uses the starter steps and produces a well-formed recipe", () => {
    const recipe = buildRecipe({ steps: STARTER_STEPS });
    // Structural checks.
    expect(recipe).toContain("version: v1");
    expect(recipe).toContain("flows:");
    expect(recipe).toContain("  lab:");
    expect(recipe).not.toContain("source_transforms:");
    // All four steps, in order: the transformers run first.
    expect(recipe).toContain("    steps:");
    const order = ["search-replace", "redact", "ai-translate", "qa-check"].map((t) =>
      recipe.indexOf(`- tool: ${t}`),
    );
    expect(order.every((idx) => idx > -1)).toBe(true);
    expect([...order].sort((a, b) => a - b)).toEqual(order);
  });
});

// ── buildToolInfos ───────────────────────────────────────────────────────────

describe("buildToolInfos", () => {
  it("flags search-replace as a transformer (isSourceTransform)", () => {
    const infos = buildToolInfos();
    const sr = infos.find((t) => t.name === "search-replace");
    expect(sr).toBeDefined();
    expect(sr?.isSourceTransform).toBe(true);
  });

  it("flags redact as a transformer (isSourceTransform)", () => {
    const infos = buildToolInfos();
    const redact = infos.find((t) => t.name === "redact");
    expect(redact).toBeDefined();
    expect(redact?.isSourceTransform).toBe(true);
  });

  it("carries redact's recoverable flag from the reference dataset", () => {
    const infos = buildToolInfos();
    const redact = infos.find((t) => t.name === "redact");
    expect(redact?.recoverable).toBe(true);
  });

  it("does NOT flag ai-translate as a transformer", () => {
    const infos = buildToolInfos();
    const ait = infos.find((t) => t.name === "ai-translate");
    expect(ait).toBeDefined();
    expect(ait?.isSourceTransform).toBeFalsy();
  });

  it("does NOT flag word-count as a transformer", () => {
    const infos = buildToolInfos();
    const wc = infos.find((t) => t.name === "word-count");
    expect(wc).toBeDefined();
    expect(wc?.isSourceTransform).toBeFalsy();
  });

  it("carries the canonical text-processing category for search-replace and redact", () => {
    const infos = buildToolInfos();
    for (const id of ["search-replace", "redact"]) {
      const info = infos.find((t) => t.name === id);
      expect(info?.category).toBe("text-processing");
    }
  });
});
