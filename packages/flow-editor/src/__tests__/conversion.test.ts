import { describe, it, expect } from "vitest";
import { stepsToGraph, graphToSteps } from "../conversion";
import type { FlowSpec, ToolInfo } from "../types";

describe("stepsToGraph", () => {
  it("converts single-step flow to reader → tool → writer", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }],
    };

    const { nodes, edges } = stepsToGraph(spec);

    expect(nodes).toHaveLength(3);
    expect(nodes[0].type).toBe("reader");
    expect(nodes[1].type).toBe("tool");
    expect(nodes[1].data.toolName).toBe("ai-translate");
    expect(nodes[2].type).toBe("writer");

    expect(edges).toHaveLength(2);
    expect(edges[0].source).toBe("reader");
    expect(edges[0].target).toBe("tool-0");
    expect(edges[1].source).toBe("tool-0");
    expect(edges[1].target).toBe("writer");
  });

  it("converts multi-step flow with correct chaining", () => {
    const spec: FlowSpec = {
      steps: [{ tool: "ai-translate" }, { tool: "qa-check" }, { tool: "pseudo-translate" }],
    };

    const { nodes, edges } = stepsToGraph(spec);

    expect(nodes).toHaveLength(5); // reader + 3 tools + writer
    expect(edges).toHaveLength(4); // reader→t0, t0→t1, t1→t2, t2→writer

    expect(edges[0]).toMatchObject({ id: "e-reader-tool-0", source: "reader", target: "tool-0" });
    expect(edges[1]).toMatchObject({ id: "e-tool-0-tool-1", source: "tool-0", target: "tool-1" });
    expect(edges[2]).toMatchObject({ id: "e-tool-1-tool-2", source: "tool-1", target: "tool-2" });
    expect(edges[3]).toMatchObject({ id: "e-tool-2-writer", source: "tool-2", target: "writer" });
  });

  it("auto-layouts nodes left to right in horizontal mode", () => {
    const { nodes } = stepsToGraph(
      { steps: [{ tool: "a" }, { tool: "b" }] },
      undefined,
      "horizontal",
    );

    for (let i = 1; i < nodes.length; i++) {
      expect(nodes[i].position.x).toBeGreaterThan(nodes[i - 1].position.x);
    }
  });

  it("auto-layouts nodes top to bottom in vertical mode", () => {
    const { nodes } = stepsToGraph(
      { steps: [{ tool: "a" }, { tool: "b" }] },
      undefined,
      "vertical",
    );

    for (let i = 1; i < nodes.length; i++) {
      expect(nodes[i].position.y).toBeGreaterThan(nodes[i - 1].position.y);
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

  it("handles empty steps (just reader → writer)", () => {
    const { nodes, edges } = stepsToGraph({ steps: [] });
    expect(nodes).toHaveLength(2);
    expect(nodes[0].type).toBe("reader");
    expect(nodes[1].type).toBe("writer");
    expect(edges).toHaveLength(1);
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
  it("extracts tool nodes in correct order (vertical)", () => {
    const { nodes } = stepsToGraph({
      steps: [{ tool: "a" }, { tool: "b" }, { tool: "c" }],
    });

    const result = graphToSteps(nodes);
    expect(result.steps).toHaveLength(3);
    expect(result.steps[0].tool).toBe("a");
    expect(result.steps[1].tool).toBe("b");
    expect(result.steps[2].tool).toBe("c");
  });

  it("extracts tool nodes in correct order (horizontal)", () => {
    const { nodes } = stepsToGraph(
      { steps: [{ tool: "a" }, { tool: "b" }, { tool: "c" }] },
      undefined,
      "horizontal",
    );

    const result = graphToSteps(nodes, "horizontal");
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

  it("ignores reader and writer nodes", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "x" }] });
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

    const toolNodes = nodes.filter((n) => n.type === "tool");
    expect(toolNodes).toHaveLength(2);
    expect(toolNodes[0].data.cardinality).toBe("bilingual");
    expect(toolNodes[1].data.cardinality).toBe("bilingual");
    expect(toolNodes[1].data.defaultLocale).toBe("qps");
  });

  it("handles tools without IO contract fields", () => {
    const { nodes } = stepsToGraph({ steps: [{ tool: "unknown-tool" }] });
    const toolNode = nodes.find((n) => n.type === "tool");
    expect(toolNode!.data.cardinality).toBeUndefined();
    expect(toolNode!.data.defaultLocale).toBeUndefined();
    expect(toolNode!.data.sideEffects).toBeUndefined();
  });
});
