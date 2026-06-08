import { describe, it, expect } from "vitest";
import { computeUnmet } from "../ioGraph";
import type { FlowSpec, ToolInfo } from "../types";

const tools = new Map<string, ToolInfo>([
  [
    "ai-translate",
    {
      name: "ai-translate",
      description: "",
      category: "translation",
      produces: [{ type: "target", side: "target" }],
    },
  ],
  [
    "qa-check",
    {
      name: "qa-check",
      description: "",
      category: "quality",
      consumes: [{ type: "target", side: "target" }],
      produces: [{ type: "qa", side: "target" }],
    },
  ],
  [
    "segmentation",
    {
      name: "segmentation",
      description: "",
      category: "text-processing",
      produces: [{ type: "segmentation", side: "source" }],
    },
  ],
  [
    "tm-leverage",
    {
      name: "tm-leverage",
      description: "",
      category: "translation",
      // optional segmentation consume + required nothing
      consumes: [{ type: "segmentation", side: "source", optional: true }],
      produces: [{ type: "target", side: "target" }],
    },
  ],
]);

describe("computeUnmet", () => {
  it("flags a required target consume when nothing upstream produces target", () => {
    const spec: FlowSpec = { steps: [{ tool: "qa-check" }] };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual(["target@target"]);
  });

  it("is satisfied when a translation precedes the consumer", () => {
    const spec: FlowSpec = { steps: [{ tool: "ai-translate" }, { tool: "qa-check" }] };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual([]); // ai-translate has no required consumes
    expect(steps[1]).toEqual([]); // qa-check's target is now produced upstream
  });

  it("never flags optional consumes", () => {
    const spec: FlowSpec = { steps: [{ tool: "tm-leverage" }] };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual([]); // segmentation is optional
  });

  it("treats source content as always available", () => {
    const spec: FlowSpec = { steps: [{ tool: "segmentation" }] };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual([]);
  });

  it("evaluates parallel branches against the shared upstream", () => {
    // qa-check inside a parallel group, with no translation upstream → unmet.
    const spec: FlowSpec = {
      steps: [{ tool: "", parallel: [{ tool: "qa-check" }, { tool: "segmentation" }] }],
    };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual(["target@target"]);
  });

  it("source-transform produces are available to the main steps", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "ai-translate" }],
      steps: [{ tool: "qa-check" }],
    };
    const { sourceTransforms, steps } = computeUnmet(spec, tools);
    expect(sourceTransforms[0]).toEqual([]);
    expect(steps[0]).toEqual([]); // target produced by the source-transform stage
  });
});
