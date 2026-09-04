import { describe, it, expect } from "vite-plus/test";
import type { FlowDefinitionInfo } from "@neokapi/ui";
import {
  definitionToSpec,
  flowStepNames,
  specToDefinition,
  toEditorDefinition,
  toEditorTools,
} from "./flowGraph";
import { builtInTranslate, projectReview, sampleTools } from "./fixtures";

describe("definitionToSpec", () => {
  it("reads the server's one-row built-in as an ordered chain", () => {
    const spec = definitionToSpec(builtInTranslate);
    expect(spec.steps.map((s) => s.tool)).toEqual(["recycle", "translate", "qa"]);
    expect(spec.steps[1].config).toEqual({ skipMatched: true });
    expect(spec.description).toBe(builtInTranslate.description);
  });

  it("reads a fan-out as a parallel step", () => {
    const spec = definitionToSpec(projectReview);
    expect(spec.steps).toHaveLength(2);
    expect(spec.steps[0].tool).toBe("translate");
    expect(spec.steps[1].parallel?.map((s) => s.tool)).toEqual(["qa", "term-check"]);
  });

  it("leaves reader and writer nodes out of the steps", () => {
    const legacy: FlowDefinitionInfo = {
      id: "legacy",
      name: "Legacy",
      source: "project",
      nodes: [
        { id: "reader", type: "reader", name: "auto", position: { x: 0, y: 100 } },
        { id: "t", type: "tool", name: "translate", position: { x: 250, y: 100 } },
        { id: "writer", type: "writer", name: "auto", position: { x: 500, y: 100 } },
      ],
      edges: [
        { id: "e1", source: "reader", target: "t" },
        { id: "e2", source: "t", target: "writer" },
      ],
    };
    expect(definitionToSpec(legacy).steps.map((s) => s.tool)).toEqual(["translate"]);
    expect(toEditorDefinition(legacy).edges).toEqual([]);
  });
});

describe("specToDefinition", () => {
  it("round-trips a parallel group through the persisted graph", () => {
    const spec = definitionToSpec(projectReview);
    const def = specToDefinition(spec, projectReview);
    expect(def.id).toBe("flow-review");
    expect(def.name).toBe("Translate and review");
    expect(def.source).toBe("project");
    expect(def.nodes.map((n) => n.name)).toEqual(["translate", "qa", "term-check"]);
    // The fan-out is two edges from the entry to each branch.
    expect(def.edges.map((e) => `${e.source}>${e.target}`).sort()).toEqual([
      "tool-0>tool-1",
      "tool-0>tool-2",
    ]);
    expect(definitionToSpec(def)).toEqual(spec);
  });

  it("carries a renamed flow without changing its id", () => {
    const def = specToDefinition(
      { steps: [{ tool: "translate" }] },
      { ...projectReview, name: "Renamed" },
    );
    expect(def.id).toBe("flow-review");
    expect(def.name).toBe("Renamed");
  });
});

describe("flowStepNames", () => {
  it("names steps by label, then tool display name, and a group as one chip", () => {
    expect(flowStepNames(builtInTranslate, sampleTools)).toEqual([
      "Memory Reuse",
      "Translate",
      "Quality Check",
    ]);
    expect(flowStepNames(projectReview, sampleTools)).toEqual(["Translate", "Parallel group"]);
  });

  it("falls back to the tool id when the registry does not know the tool", () => {
    expect(flowStepNames(projectReview, [])).toEqual(["translate", "Parallel group"]);
  });
});

describe("toEditorTools", () => {
  it("carries the transformer flag and the IO contract into the editor's shape", () => {
    const [tool] = toEditorTools([
      {
        ...sampleTools[0],
        is_source_transform: true,
        consumes: [{ type: "entities", side: "source" }],
      },
    ]);
    expect(tool.name).toBe("recycle");
    expect(tool.display_name).toBe("Recycle");
    expect(tool.isSourceTransform).toBe(true);
    expect(tool.consumes).toHaveLength(1);
  });
});
