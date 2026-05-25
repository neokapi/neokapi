import { describe, it, expect } from "vitest";
import { stepsToGraph, graphToSteps } from "../conversion";
import type { FlowSpec, ToolInfo } from "../types";

// ---------------------------------------------------------------------------
// stepsToGraph — source-transform stage
// ---------------------------------------------------------------------------

describe("stepsToGraph — source-transform stage", () => {
  it("emits source-transform nodes before main nodes (after reader)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec);

    expect(nodes[0].type).toBe("reader");
    expect(nodes[1].type).toBe("tool");
    expect(nodes[1].data.toolName).toBe("redact");
    expect(nodes[1].data.stage).toBe("source-transform");
    expect(nodes[2].type).toBe("tool");
    expect(nodes[2].data.toolName).toBe("ai-translate");
    expect(nodes[2].data.stage).toBeUndefined();
    expect(nodes[3].type).toBe("writer");
  });

  it("assigns tool-0… IDs to source-transform nodes first", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }, { tool: "normalise" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec);
    const toolNodes = nodes.filter((n) => n.type === "tool");

    expect(toolNodes[0].id).toBe("tool-0");
    expect(toolNodes[0].data.toolName).toBe("redact");
    expect(toolNodes[1].id).toBe("tool-1");
    expect(toolNodes[1].data.toolName).toBe("normalise");
    expect(toolNodes[2].id).toBe("tool-2");
    expect(toolNodes[2].data.toolName).toBe("ai-translate");
  });

  it("positions source-transform nodes before main nodes on the primary axis (vertical)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec, undefined, "vertical");
    const redact = nodes.find((n) => n.data.toolName === "redact")!;
    const translate = nodes.find((n) => n.data.toolName === "ai-translate")!;

    expect(redact.position.y).toBeLessThan(translate.position.y);
  });

  it("positions source-transform nodes before main nodes on the primary axis (horizontal)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec, undefined, "horizontal");
    const redact = nodes.find((n) => n.data.toolName === "redact")!;
    const translate = nodes.find((n) => n.data.toolName === "ai-translate")!;

    expect(redact.position.x).toBeLessThan(translate.position.x);
  });

  it("edges chain reader → ST nodes → main nodes → writer", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { edges } = stepsToGraph(spec);

    expect(edges[0]).toMatchObject({ source: "reader", target: "tool-0" });
    expect(edges[1]).toMatchObject({ source: "tool-0", target: "tool-1" });
    expect(edges[2]).toMatchObject({ source: "tool-1", target: "writer" });
  });

  it("handles flow with only source-transform steps (no main steps)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [],
    };

    const { nodes, edges } = stepsToGraph(spec);

    expect(nodes).toHaveLength(3); // reader + redact + writer
    expect(nodes[1].data.stage).toBe("source-transform");
    expect(edges[0]).toMatchObject({ source: "reader", target: "tool-0" });
    expect(edges[1]).toMatchObject({ source: "tool-0", target: "writer" });
  });

  it("handles flow with no source-transform steps (backward compat)", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec);
    const toolNode = nodes.find((n) => n.type === "tool")!;

    expect(toolNode.data.stage).toBeUndefined();
  });

  it("enriches source-transform nodes with toolMap data including isSourceTransform", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "redact",
        {
          name: "redact",
          description: "Redact sensitive spans",
          category: "transform",
          isSourceTransform: true,
        },
      ],
    ]);

    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [],
    };

    const { nodes } = stepsToGraph(spec, toolMap);
    const stNode = nodes.find((n) => n.data.toolName === "redact")!;

    expect(stNode.data.stage).toBe("source-transform");
    expect(stNode.data.category).toBe("transform");
    expect(stNode.data.isSourceTransform).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// graphToSteps — source-transform extraction
// ---------------------------------------------------------------------------

describe("graphToSteps — source-transform extraction", () => {
  it("extracts source-transform nodes into sourceTransforms", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec);
    const result = graphToSteps(nodes);

    expect(result.sourceTransforms).toHaveLength(1);
    expect(result.sourceTransforms![0].tool).toBe("redact");
    expect(result.steps).toHaveLength(1);
    expect(result.steps[0].tool).toBe("ai-translate");
  });

  it("preserves sourceTransform order (primary-axis order)", () => {
    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }, { tool: "normalise" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec, undefined, "vertical");
    const result = graphToSteps(nodes, "vertical");

    expect(result.sourceTransforms).toHaveLength(2);
    expect(result.sourceTransforms![0].tool).toBe("redact");
    expect(result.sourceTransforms![1].tool).toBe("normalise");
  });

  it("omits sourceTransforms from result when there are none", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec);
    const result = graphToSteps(nodes);

    expect(result.sourceTransforms).toBeUndefined();
    expect(result.steps).toHaveLength(1);
  });

  it("returns empty sourceTransforms omitted when only main steps exist", () => {
    const spec: FlowSpec = { steps: [{ tool: "a" }, { tool: "b" }] };
    const { nodes } = stepsToGraph(spec);
    const result = graphToSteps(nodes);

    expect(result.sourceTransforms).toBeUndefined();
  });
});

// ---------------------------------------------------------------------------
// Round-trip stability
// ---------------------------------------------------------------------------

describe("round-trip stability — source-transform", () => {
  it("roundtrips a flow with source transforms (vertical)", () => {
    const original: FlowSpec = {
      sourceTransforms: [{ tool: "redact", config: { mode: "placeholder" } }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    };

    const { nodes } = stepsToGraph(original, undefined, "vertical");
    const result = graphToSteps(nodes, "vertical");

    expect(result.sourceTransforms).toHaveLength(1);
    expect(result.sourceTransforms![0].tool).toBe("redact");
    expect(result.sourceTransforms![0].config).toEqual({ mode: "placeholder" });
    expect(result.steps).toHaveLength(2);
    expect(result.steps[0].tool).toBe("ai-translate");
    expect(result.steps[1].tool).toBe("qa-check");
  });

  it("roundtrips a flow with source transforms (horizontal)", () => {
    const original: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(original, undefined, "horizontal");
    const result = graphToSteps(nodes, "horizontal");

    expect(result.sourceTransforms).toHaveLength(1);
    expect(result.sourceTransforms![0].tool).toBe("redact");
    expect(result.steps).toHaveLength(1);
    expect(result.steps[0].tool).toBe("ai-translate");
  });

  it("multiple roundtrip passes are stable", () => {
    const original: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }],
    };

    const pass1nodes = stepsToGraph(original).nodes;
    const pass1result = graphToSteps(pass1nodes);

    const pass2nodes = stepsToGraph(pass1result).nodes;
    const pass2result = graphToSteps(pass2nodes);

    expect(pass2result).toEqual(pass1result);
  });

  it("roundtrips an empty flow with no source transforms", () => {
    const original: FlowSpec = { steps: [] };
    const { nodes } = stepsToGraph(original);
    const result = graphToSteps(nodes);
    expect(result).toEqual({ steps: [] });
  });
});

// ---------------------------------------------------------------------------
// isSourceTransform gating (ToolInfo field)
// ---------------------------------------------------------------------------

describe("ToolInfo.isSourceTransform gating", () => {
  it("isSourceTransform is passed through to node data for ST-capable tools", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "redact",
        {
          name: "redact",
          description: "Redact",
          category: "transform",
          isSourceTransform: true,
        },
      ],
      [
        "ai-translate",
        {
          name: "ai-translate",
          description: "Translate",
          category: "translate",
          isSourceTransform: false,
        },
      ],
    ]);

    const spec: FlowSpec = {
      steps: [{ tool: "redact" }, { tool: "ai-translate" }],
    };

    const { nodes } = stepsToGraph(spec, toolMap);
    const redactNode = nodes.find((n) => n.data.toolName === "redact")!;
    const translateNode = nodes.find((n) => n.data.toolName === "ai-translate")!;

    expect(redactNode.data.isSourceTransform).toBe(true);
    expect(translateNode.data.isSourceTransform).toBe(false);
  });

  it("isSourceTransform is undefined for tools not in toolMap", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "unknown" }] });
    const node = nodes.find((n) => n.type === "tool")!;
    expect(node.data.isSourceTransform).toBeUndefined();
  });

  it("main-stage node with isSourceTransform=true does not have stage set", () => {
    // A tool that CAN be in the source-transform stage but is currently in main stage
    // should have isSourceTransform=true but no stage data field.
    const toolMap = new Map<string, ToolInfo>([
      [
        "redact",
        {
          name: "redact",
          description: "Redact",
          category: "transform",
          isSourceTransform: true,
        },
      ],
    ]);

    const spec: FlowSpec = { steps: [{ tool: "redact" }] };
    const { nodes } = stepsToGraph(spec, toolMap);
    const node = nodes.find((n) => n.data.toolName === "redact")!;

    expect(node.data.stage).toBeUndefined();
    expect(node.data.isSourceTransform).toBe(true);
  });

  it("source-transform-stage node has both stage='source-transform' and isSourceTransform=true", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "redact",
        {
          name: "redact",
          description: "Redact",
          category: "transform",
          isSourceTransform: true,
        },
      ],
    ]);

    const spec: FlowSpec = {
      sourceTransforms: [{ tool: "redact" }],
      steps: [],
    };

    const { nodes } = stepsToGraph(spec, toolMap);
    const node = nodes.find((n) => n.data.toolName === "redact")!;

    expect(node.data.stage).toBe("source-transform");
    expect(node.data.isSourceTransform).toBe(true);
  });
});
