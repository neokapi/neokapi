import { describe, it, expect } from "vitest";
import { stepsToGraph, graphToSteps } from "../conversion";
import { suggestParallelGroups } from "../parallelChecker";
import type { FlowSpec, ToolInfo } from "../types";

describe("stepsToGraph with parallel branches", () => {
  it("creates fan-out nodes for parallel steps", () => {
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

    const { nodes } = stepsToGraph(spec, undefined, "horizontal");

    // ai-translate + ai-qa + tm-leverage + merge-results = 4 tool nodes (no reader/writer)
    expect(nodes).toHaveLength(4);

    const toolNodes = nodes.filter((n) => n.type === "tool");
    expect(toolNodes).toHaveLength(4);

    // The two parallel nodes should have the same X position
    const qaNode = toolNodes.find((n) => n.data.toolName === "ai-qa")!;
    const tmNode = toolNodes.find((n) => n.data.toolName === "tm-leverage")!;
    expect(qaNode.position.x).toBe(tmNode.position.x);

    // But different Y positions
    expect(qaNode.position.y).not.toBe(tmNode.position.y);

    // Parallel nodes should be marked
    expect(qaNode.data.parallel).toBe(true);
    expect(tmNode.data.parallel).toBe(true);
  });

  it("creates fan-out edges from previous node to all branches", () => {
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

    // translate → qa, translate → tm = 2 (no reader/writer edges)
    expect(edges).toHaveLength(2);

    // translate fans out to both qa and tm
    const fanOutEdges = edges.filter((e) => e.source === "tool-0");
    expect(fanOutEdges).toHaveLength(2);
    const targets = fanOutEdges.map((e) => e.target).sort();
    expect(targets).toEqual(["tool-1", "tool-2"]);
  });

  it("creates merge edges from all branches to next sequential node", () => {
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

    // reader → qa, reader → tm, qa → merge, tm → merge, merge → writer = 5
    const mergeEdges = edges.filter((e) => e.target === "tool-2");
    expect(mergeEdges).toHaveLength(2);
  });

  it("handles three-way parallel branches", () => {
    const spec: FlowSpec = {
      steps: [
        {
          tool: "",
          parallel: [{ tool: "a" }, { tool: "b" }, { tool: "c" }],
        },
      ],
    };

    const { nodes, edges } = stepsToGraph(spec);

    const toolNodes = nodes.filter((n) => n.type === "tool");
    expect(toolNodes).toHaveLength(3);

    // A lone parallel group has no preceding or following node, so there are no
    // fan-out or merge edges — the three branches are all entry and exit points.
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

    const { nodes } = stepsToGraph(spec, undefined, "horizontal");
    const qaNode = nodes.find((n) => n.data.toolName === "qa")!;
    expect(qaNode.data.label).toBe("Quality Check");
    expect(qaNode.data.config).toEqual({ strict: true });
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

    const { nodes } = stepsToGraph(spec, undefined, "horizontal");
    const result = graphToSteps(nodes, "horizontal");

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
