import { describe, expect, it } from "vitest";
import { buildRecipe, buildToolInfos } from "./FlowBuilderRunner";
import { LAB_SCENARIOS } from "./labScenarios";
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

  it("produces a well-formed recipe for the build-your-own scenario", () => {
    const scenario = LAB_SCENARIOS.find((s) => s.id === "build-your-own")!;
    const recipe = buildRecipe({ steps: scenario.steps });
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

  it("emits project tool presets under defaults.tools", () => {
    const scenario = LAB_SCENARIOS.find((s) => s.id === "redaction")!;
    const recipe = buildRecipe({ steps: scenario.steps }, scenario.presets);
    expect(recipe).toContain("defaults:");
    expect(recipe).toContain("  tools:");
    expect(recipe).toContain("    redact:");
    // Preset values serialize as YAML scalars / JSON flow values.
    expect(recipe).toContain("Acme Corp");
    expect(recipe).toContain("detectors:");
    // The redact STEP stays bare — the preset is project scope, not step config.
    expect(recipe).toContain("      - tool: redact\n      - tool: ai-translate");
  });

  it("omits defaults.tools when no presets are given", () => {
    const recipe = buildRecipe({ steps: [{ tool: "word-count" }] });
    expect(recipe).not.toContain("  tools:");
  });
});

// ── scenarios ────────────────────────────────────────────────────────────────

describe("LAB_SCENARIOS", () => {
  it("only uses browser-safe tools the palette offers", () => {
    const palette = new Set(buildToolInfos().map((t) => t.name));
    for (const scenario of LAB_SCENARIOS) {
      for (const step of scenario.steps) {
        if (step.tool) expect(palette, `${scenario.id}: ${step.tool}`).toContain(step.tool);
        for (const branch of step.parallel ?? []) {
          expect(palette, `${scenario.id}: ${branch.tool}`).toContain(branch.tool);
        }
      }
    }
  });

  it("covers the redaction, segmentation and annotations teaching stories", () => {
    const ids = LAB_SCENARIOS.map((s) => s.id);
    expect(ids).toContain("redaction");
    expect(ids).toContain("segmentation");
    expect(ids).toContain("annotations");
  });

  it("the redaction scenario carries its rules as a project preset", () => {
    const s = LAB_SCENARIOS.find((x) => x.id === "redaction")!;
    expect(s.presets?.redact).toBeDefined();
    const redactStep = s.steps.find((st) => st.tool === "redact")!;
    expect(redactStep.config).toBeUndefined();
    // unredact closes the wrap so the originals come back after translation.
    expect(s.steps.some((st) => st.tool === "unredact")).toBe(true);
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

describe("buildRecipe multi-line strings", () => {
  it("emits a script step's code as a YAML literal block, not one escaped string", () => {
    const code = 'function process(part) {\n  log("hi");\n  return part;\n}\n';
    const recipe = buildRecipe({
      steps: [{ tool: "script", config: { allowSourceMutation: true, code } }],
    });
    expect(recipe).toContain("          code: |");
    expect(recipe).toContain("            function process(part) {");
    expect(recipe).toContain('              log("hi");');
    expect(recipe).not.toContain("\\n");
  });

  it("keeps blank code lines as empty YAML lines and single-line strings quoted", () => {
    const recipe = buildRecipe(
      { steps: [{ tool: "script", config: { code: "a\n\nb" } }] },
      { redact: { note: "one line" } },
    );
    const lines = recipe.split("\n");
    const a = lines.indexOf("            a");
    expect(a).toBeGreaterThan(0);
    expect(lines[a + 1]).toBe("");
    expect(lines[a + 2]).toBe("            b");
    // Presets use the same emitter; single-line strings stay quoted scalars.
    expect(recipe).toContain('      note: "one line"');
  });
});
