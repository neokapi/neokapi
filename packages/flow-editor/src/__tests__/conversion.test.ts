import { describe, it, expect } from "vitest";
import { stepsToGraph, graphToSteps, centerAlignRows } from "../conversion";
import type { FlowSpec, ToolInfo } from "../types";

describe("stepsToGraph", () => {
  it("converts single-step flow to a single tool node with no I/O nodes", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes, edges } = stepsToGraph(spec);

    // A flow owns no I/O (AD-026): tool nodes only, no reader/writer.
    expect(nodes).toHaveLength(1);
    expect(nodes[0].type).toBe("tool");
    expect(nodes[0].data.toolName).toBe("ai-translate");

    // The single tool has no incoming/outgoing edge (it is both first and last).
    expect(edges).toHaveLength(0);
  });

  it("converts multi-step flow with correct chaining", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "pseudo-translate" }],
    };

    const { nodes, edges } = stepsToGraph(spec);

    expect(nodes).toHaveLength(3); // 3 tools, no reader/writer
    expect(edges).toHaveLength(2); // t0→t1, t1→t2

    expect(edges[0]).toMatchObject({ id: "e-tool-0-tool-1", source: "tool-0", target: "tool-1" });
    expect(edges[1]).toMatchObject({ id: "e-tool-1-tool-2", source: "tool-1", target: "tool-2" });
  });

  it("carries source/sink binding locators through graphToSteps", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }],
      source: "xliff",
      sink: "store",
    };

    const { nodes } = stepsToGraph(spec);
    const result = graphToSteps(nodes, {
      source: spec.source,
      sink: spec.sink,
    });

    expect(result.source).toBe("xliff");
    expect(result.sink).toBe("store");
  });

  it("auto-layouts the chain left to right", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "a" }, { tool: "b" }] });

    for (let i = 1; i < nodes.length; i++) {
      expect(nodes[i].position.x).toBeGreaterThan(nodes[i - 1].position.x);
    }
  });

  it("preserves step labels and configs", () => {
    const { nodes } = stepsToGraph({
      steps: [{ tool: "ai-translate", label: "Translate", config: { provider: "anthropic" } }],
    });

    const toolNode = nodes.find((n) => n.type === "tool")!;
    expect(toolNode.data.label).toBe("Translate");
    expect(toolNode.data.config).toEqual({ provider: "anthropic" });
  });

  it("handles empty steps (no nodes, no edges)", () => {
    const { nodes, edges } = stepsToGraph({ steps: [] });
    expect(nodes).toHaveLength(0);
    expect(edges).toHaveLength(0);
  });

  it("enriches tool nodes with category and description from toolMap", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "ai-translate",
        {
          name: "ai-translate",
          description: "Translate with AI",
          category: "translate",
          has_schema: false,
          inputs: ["block"],
          tags: ["ai-powered"],
          requires: ["target-language"],
        },
      ],
    ]);

    const { nodes } = stepsToGraph({ steps: [{ tool: "ai-translate" }] }, toolMap);

    const toolNode = nodes.find((n) => n.type === "tool")!;
    expect(toolNode.data.category).toBe("translate");
    expect(toolNode.data.description).toBe("Translate with AI");
  });

  it("defaults to pipeline category when tool not in toolMap", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "unknown-tool" }] }, new Map());

    const toolNode = nodes.find((n) => n.type === "tool")!;
    expect(toolNode.data.category).toBe("pipeline");
    expect(toolNode.data.description).toBeUndefined();
  });

  it("works without toolMap (backward compatible)", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "a" }] });
    const toolNode = nodes.find((n) => n.type === "tool")!;
    expect(toolNode.data.category).toBe("pipeline");
  });
});

describe("graphToSteps", () => {
  it("extracts tool nodes in chain order", () => {
    const { nodes } = stepsToGraph({
      steps: [{ tool: "a" }, { tool: "b" }, { tool: "c" }],
    });

    const result = graphToSteps(nodes);
    expect(result.steps).toHaveLength(3);
    expect(result.steps[0].tool).toBe("a");
    expect(result.steps[1].tool).toBe("b");
    expect(result.steps[2].tool).toBe("c");
  });

  it("roundtrips a flow spec", () => {
    const original: FlowSpec = {
      steps: [{ tool: "ai-translate", config: { provider: "anthropic" } }, { tool: "qa-check" }],
    };

    const { nodes } = stepsToGraph(original);
    const result = graphToSteps(nodes);

    expect(result.steps).toHaveLength(2);
    expect(result.steps[0].tool).toBe("ai-translate");
    expect(result.steps[0].config).toEqual({ provider: "anthropic" });
    expect(result.steps[1].tool).toBe("qa-check");
  });

  it("reconstructs steps from tool nodes only", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "x" }] });
    expect(nodes.every((n) => n.type === "tool")).toBe(true);
    const result = graphToSteps(nodes);
    expect(result.steps).toHaveLength(1);
    expect(result.steps[0].tool).toBe("x");
  });
});

describe("stepsToGraph IO contract fields", () => {
  it("passes cardinality and defaultLocale to sequential node data", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "pseudo-translate",
        {
          name: "pseudo-translate",
          description: "Pseudo translate",
          category: "transform",
          cardinality: "bilingual",
          default_locale: "qps",
          side_effects: ["tm-read"],
        },
      ],
    ]);

    const { nodes } = stepsToGraph({ steps: [{ tool: "pseudo-translate" }] }, toolMap);

    const toolNode = nodes.find((n) => n.type === "tool");
    expect(toolNode).toBeDefined();
    expect(toolNode!.data.cardinality).toBe("bilingual");
    expect(toolNode!.data.defaultLocale).toBe("qps");
    expect(toolNode!.data.sideEffects).toEqual(["tm-read"]);
  });

  it("passes cardinality to parallel branch node data", () => {
    const toolMap = new Map<string, ToolInfo>([
      [
        "ai-translate",
        {
          name: "ai-translate",
          description: "AI Translate",
          category: "translate",
          cardinality: "bilingual",
        },
      ],
      [
        "pseudo-translate",
        {
          name: "pseudo-translate",
          description: "Pseudo",
          category: "transform",
          cardinality: "bilingual",
          default_locale: "qps",
        },
      ],
    ]);

    const spec: FlowSpec = {
      steps: [{ parallel: [{ tool: "ai-translate" }, { tool: "pseudo-translate" }] }],
    };
    const { nodes } = stepsToGraph(spec, toolMap);

    // A parallel step is one composite node listing its branch tools.
    const group = nodes.find((n) => n.type === "parallel")!;
    expect(group).toBeDefined();
    const branches = group.data.branches as Array<{ toolName: string }>;
    expect(branches.map((b) => b.toolName)).toEqual(["ai-translate", "pseudo-translate"]);
  });

  it("handles tools without IO contract fields", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "unknown-tool" }] });
    const toolNode = nodes.find((n) => n.type === "tool");
    expect(toolNode!.data.cardinality).toBeUndefined();
    expect(toolNode!.data.defaultLocale).toBeUndefined();
    expect(toolNode!.data.sideEffects).toBeUndefined();
  });

  it("marks unknown tools as invalid when toolMap is provided", () => {
    const toolMap = new Map<string, ToolInfo>([
      ["known-tool", { name: "known-tool", description: "A known tool", category: "validate" }],
    ]);

    const spec: FlowSpec = {
      steps: [{ tool: "known-tool" }, { tool: "bogus-tool" }],
    };
    const { nodes } = stepsToGraph(spec, toolMap);

    const toolNodes = nodes.filter((n) => n.type === "tool");
    expect(toolNodes).toHaveLength(2);
    expect(toolNodes[0].data.valid).toBe(true);
    expect(toolNodes[1].data.valid).toBe(false);
  });

  it("marks all tools as valid when no toolMap is provided", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "anything" }] });
    const toolNode = nodes.find((n) => n.type === "tool");
    expect(toolNode!.data.valid).toBe(true);
  });
});

describe("centerAlignRows", () => {
  const node = (id: string, x: number, y: number) => ({
    id,
    type: "tool",
    position: { x, y },
    data: {},
  });

  it("centers shorter nodes on the tallest node's centerline within a row", () => {
    const nodes = [node("a", 0, 0), node("b", 240, 0), node("c", 480, 0)];
    const h: Record<string, number> = { a: 80, b: 160, c: 100 };
    const out = centerAlignRows(nodes, 200, (n) => h[n.id]);
    const centerOf = (id: string) => out.find((n) => n.id === id)!.position.y + h[id] / 2;
    expect(out.find((n) => n.id === "b")!.position.y).toBe(0); // tallest stays put
    expect(centerOf("a")).toBeCloseTo(centerOf("b"));
    expect(centerOf("c")).toBeCloseTo(centerOf("b"));
  });

  it("aligns rows independently and leaves singletons untouched", () => {
    const nodes = [node("a", 0, 0), node("b", 0, 200)];
    const out = centerAlignRows(nodes, 200, () => 120);
    expect(out.find((n) => n.id === "a")!.position.y).toBe(0);
    expect(out.find((n) => n.id === "b")!.position.y).toBe(200);
  });

  it("leaves nodes untouched until their height is known", () => {
    const nodes = [node("a", 0, 0), node("b", 240, 0)];
    expect(centerAlignRows(nodes, 200, () => undefined)).toBe(nodes);
  });
});
