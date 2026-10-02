import { describe, it, expect } from "vitest";

// The runtime refuses a step holding a parallel: list, and the editor creates
// none. A flow loaded with one still has to be drawn, so the reader sees the
// group the refusal names: these cover how such a step converts.
import { stepsToGraph, graphToSteps } from "../conversion";
import type { FlowSpec } from "../types";

describe("stepsToGraph with parallel branches", () => {
  it("creates a single composite node for a parallel step", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "translate" },
        {
          tool: "",
          parallel: [{ tool: "qa" }, { tool: "recycle" }],
        },
        { tool: "merge-results" },
      ],
    };

    const { nodes } = stepsToGraph(spec, undefined);

    // translate (tool) + parallel group (1 node) + merge-results (tool) = 3 nodes
    expect(nodes).toHaveLength(3);

    const group = nodes.find((n) => n.type === "parallel")!;
    expect(group).toBeDefined();
    const branches = group.data.branches as Array<{ toolName: string }>;
    expect(branches.map((b) => b.toolName)).toEqual(["qa", "recycle"]);
  });

  it("connects the previous node to the parallel group with a single edge", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "translate" },
        {
          tool: "",
          parallel: [{ tool: "qa" }, { tool: "recycle" }],
        },
      ],
    };

    const { edges } = stepsToGraph(spec);

    // translate → parallel group = 1 edge (no fan-out)
    expect(edges).toHaveLength(1);
    expect(edges[0].source).toBe("tool-0");
    expect(edges[0].target).toBe("tool-1");
  });

  it("connects the parallel group to the next node with a single edge", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "",
          parallel: [{ tool: "qa" }, { tool: "recycle" }],
        },
        { tool: "merge" },
      ],
    };

    const { edges } = stepsToGraph(spec);

    // group → merge = 1 edge (no merge fan-in)
    expect(edges).toHaveLength(1);
    expect(edges[0].source).toBe("tool-0");
    expect(edges[0].target).toBe("tool-1");
  });

  it("handles three-way parallel branches in one group node", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "",
          parallel: [{ tool: "a" }, { tool: "b" }, { tool: "c" }],
        },
      ],
    };

    const { nodes, edges } = stepsToGraph(spec);

    expect(nodes).toHaveLength(1);
    expect(nodes[0].type).toBe("parallel");
    expect((nodes[0].data.branches as unknown[]).length).toBe(3);
    // A lone group has no preceding or following node, so no edges.
    expect(edges).toHaveLength(0);
  });

  it("preserves labels and configs in parallel branches", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "",
          parallel: [
            { tool: "qa", label: "Quality Check", config: { strict: true } },
            { tool: "tm", label: "Memory Lookup" },
          ],
        },
      ],
    };

    const { nodes } = stepsToGraph(spec, undefined);
    const group = nodes.find((n) => n.type === "parallel")!;
    const branches = group.data.branches as Array<{
      toolName: string;
      label: string;
      config?: Record<string, unknown>;
    }>;
    expect(branches[0].label).toBe("Quality Check");
    expect(branches[0].config).toEqual({ strict: true });
    expect(branches[1].label).toBe("Memory Lookup");
  });
});

describe("graphToSteps with parallel branches", () => {
  it("reconstructs parallel groups from nodes at same X position", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "translate" },
        {
          tool: "",
          parallel: [{ tool: "qa" }, { tool: "tm" }],
        },
        { tool: "merge" },
      ],
    };

    const { nodes } = stepsToGraph(spec, undefined);
    const result = graphToSteps(nodes);

    expect(result.steps).toHaveLength(3);
    expect(result.steps[0].tool).toBe("translate");
    expect(result.steps[1].parallel).toHaveLength(2);
    expect(result.steps[1].parallel![0].tool).toBe("qa");
    expect(result.steps[1].parallel![1].tool).toBe("tm");
    expect(result.steps[2].tool).toBe("merge");
  });

  it("roundtrips a flow with parallel branches", () => {
    const original: FlowSpec = {
      steps: [
        { tool: "translate", config: { provider: "anthropic" } },
        {
          tool: "",
          parallel: [{ tool: "qa" }, { tool: "voice-check" }],
        },
      ],
    };

    const { nodes } = stepsToGraph(original);
    const result = graphToSteps(nodes);

    expect(result.steps).toHaveLength(2);
    expect(result.steps[0].tool).toBe("translate");
    expect(result.steps[0].config).toEqual({ provider: "anthropic" });
    expect(result.steps[1].parallel).toBeDefined();
    expect(result.steps[1].parallel).toHaveLength(2);
  });
});
