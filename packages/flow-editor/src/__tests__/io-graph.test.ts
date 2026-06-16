import { describe, it, expect } from "vitest";
import { computeUnmet, slotContext, toolFit } from "../ioGraph";
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

  it("a parallel group's produces are available to the steps after it", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "", parallel: [{ tool: "ai-translate" }, { tool: "segmentation" }] },
        { tool: "qa-check" },
      ],
    };
    const { steps } = computeUnmet(spec, tools);
    expect(steps[0]).toEqual([]);
    expect(steps[1]).toEqual([]); // target produced inside the group upstream
  });
});

describe("slotContext", () => {
  it("offers only the source at the head of an empty flow", () => {
    const ctx = slotContext({ steps: [] }, tools, 0);
    expect(ctx.available.map((p) => p.type)).toEqual(["source"]);
    expect(ctx.has.has("source@source")).toBe(true);
  });

  it("accumulates ports produced by steps before the slot", () => {
    const spec: FlowSpec = { steps: [{ tool: "ai-translate" }, { tool: "qa-check" }] };
    // Before step 0: only source. Before step 1: source + target.
    expect(slotContext(spec, tools, 0).has.has("target@target")).toBe(false);
    expect(slotContext(spec, tools, 1).has.has("target@target")).toBe(true);
    // Appending at the end sees everything, incl. qa-check's output.
    expect(slotContext(spec, tools, 2).has.has("qa@target")).toBe(true);
  });

  it("dedupes available ports and clamps the slot to the flow length", () => {
    const spec: FlowSpec = { steps: [{ tool: "ai-translate" }, { tool: "tm-leverage" }] };
    // Both produce target@target — listed once.
    const ctx = slotContext(spec, tools, 99);
    const targets = ctx.available.filter((p) => p.type === "target");
    expect(targets).toHaveLength(1);
  });

  it("makes a parallel route's branch produces visible to later slots", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "", parallel: [{ tool: "ai-translate" }] }, { tool: "qa-check" }],
    };
    expect(slotContext(spec, tools, 1).has.has("target@target")).toBe(true);
  });
});

describe("toolFit", () => {
  it("marks a tool ready when its required inputs are available", () => {
    const ctx = slotContext({ steps: [{ tool: "ai-translate" }] }, tools, 1);
    const fit = toolFit(tools.get("qa-check")!, ctx);
    expect(fit.ready).toBe(true);
    expect(fit.unmet).toEqual([]);
  });

  it("reports unmet required inputs at the head of a flow", () => {
    const ctx = slotContext({ steps: [] }, tools, 0);
    const fit = toolFit(tools.get("qa-check")!, ctx);
    expect(fit.ready).toBe(false);
    expect(fit.unmet.map((p) => p.type)).toEqual(["target"]);
  });

  it("never counts optional inputs against fit", () => {
    const ctx = slotContext({ steps: [] }, tools, 0);
    const fit = toolFit(tools.get("tm-leverage")!, ctx);
    expect(fit.ready).toBe(true); // segmentation is optional
  });
});
