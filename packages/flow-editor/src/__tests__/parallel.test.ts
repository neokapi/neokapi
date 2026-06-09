import { describe, it, expect } from "vitest";
import { stepsToGraph, graphToSteps } from "../conversion";
import { suggestParallelGroups } from "../parallelChecker";
import type { FlowSpec, ToolInfo } from "../types";

describe("stepsToGraph with parallel branches", () => {
  it("creates a single composite node for a parallel step", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "ai-translate" },
        {
          tool: "",
          parallel: [{ tool: "ai-qa" }, { tool: "tm-leverage" }],
        },
        { tool: "merge-results" },
      ],
    };

    const { nodes } = stepsToGraph(spec, undefined);

    // ai-translate (tool) + parallel group (1 node) + merge-results (tool) = 3 nodes
    expect(nodes).toHaveLength(3);

    const group = nodes.find((n) => n.type === "parallel")!;
    expect(group).toBeDefined();
    const branches = group.data.branches as Array<{ toolName: string }>;
    expect(branches.map((b) => b.toolName)).toEqual(["ai-qa", "tm-leverage"]);
  });

  it("connects the previous node to the parallel group with a single edge", () => {
    const spec: FlowSpec = {
      steps: [
        { tool: "ai-translate" },
        {
          tool: "",
          parallel: [{ tool: "ai-qa" }, { tool: "tm-leverage" }],
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
          parallel: [{ tool: "ai-qa" }, { tool: "tm-leverage" }],
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
            { tool: "tm", label: "TM Lookup" },
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
    expect(branches[1].label).toBe("TM Lookup");
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
        { tool: "ai-translate", config: { provider: "anthropic" } },
        {
          tool: "",
          parallel: [{ tool: "ai-qa" }, { tool: "brand-check" }],
        },
      ],
    };

    const { nodes } = stepsToGraph(original);
    const result = graphToSteps(nodes);

    expect(result.steps).toHaveLength(2);
    expect(result.steps[0].tool).toBe("ai-translate");
    expect(result.steps[0].config).toEqual({ provider: "anthropic" });
    expect(result.steps[1].parallel).toBeDefined();
    expect(result.steps[1].parallel).toHaveLength(2);
  });
});

describe("suggestParallelGroups", () => {
  const makeToolMap = (...tools: Array<[string, string]>): Map<string, ToolInfo> =>
    new Map(
      tools.map(([name, category]) => [
        name,
        { name, description: "", category, has_schema: false },
      ]),
    );

  it("suggests parallelizing adjacent validate + enrich tools", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "brand-check" }],
    };
    const toolMap = makeToolMap(
      ["ai-translate", "translate"],
      ["qa-check", "validate"],
      ["brand-check", "validate"],
    );

    const suggestions = suggestParallelGroups(spec, toolMap);
    expect(suggestions).toHaveLength(1);
    expect(suggestions[0].stepIndices).toEqual([1, 2]);
    expect(suggestions[0].toolNames).toEqual(["qa-check", "brand-check"]);
  });

  it("does not suggest parallelizing mutating tools", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }, { tool: "pseudo-translate" }],
    };
    const toolMap = makeToolMap(["ai-translate", "translate"], ["pseudo-translate", "translate"]);

    const suggestions = suggestParallelGroups(spec, toolMap);
    expect(suggestions).toHaveLength(0);
  });

  it("does not suggest for a single tool", () => {
    const spec: FlowSpec = { steps: [{ tool: "qa-check" }] };
    const toolMap = makeToolMap(["qa-check", "validate"]);

    const suggestions = suggestParallelGroups(spec, toolMap);
    expect(suggestions).toHaveLength(0);
  });

  it("suggests enrich + validate mixed groups", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "entity-extract" }, { tool: "qa-check" }, { tool: "term-lookup" }],
    };
    const toolMap = makeToolMap(
      ["entity-extract", "enrich"],
      ["qa-check", "validate"],
      ["term-lookup", "enrich"],
    );

    const suggestions = suggestParallelGroups(spec, toolMap);
    expect(suggestions).toHaveLength(1);
    expect(suggestions[0].stepIndices).toEqual([0, 1, 2]);
  });

  it("skips steps that are already parallel", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "",
          parallel: [{ tool: "qa-check" }, { tool: "brand-check" }],
        },
        { tool: "term-lookup" },
      ],
    };
    const toolMap = makeToolMap(
      ["qa-check", "validate"],
      ["brand-check", "validate"],
      ["term-lookup", "enrich"],
    );

    const suggestions = suggestParallelGroups(spec, toolMap);
    expect(suggestions).toHaveLength(0); // single tool after parallel, nothing to group
  });

  it("returns empty for an empty flow", () => {
    const suggestions = suggestParallelGroups({ steps: [] }, new Map());
    expect(suggestions).toHaveLength(0);
  });
});
