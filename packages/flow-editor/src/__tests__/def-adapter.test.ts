import { describe, it, expect } from "vitest";
import { defToSpec, specToDef } from "../defAdapter";
import type { FlowDefinitionInfo, ToolInfo } from "../types";

const tools: ToolInfo[] = [
  { name: "ai-translate", description: "Translate with AI", category: "translate" },
  { name: "ai-qa", description: "QA check", category: "validate" },
  {
    name: "redact",
    description: "Redact sensitive content",
    category: "transform",
    isSourceTransform: true,
  },
  { name: "unredact", description: "Restore originals", category: "transform" },
  { name: "term-check", description: "Term check", category: "validate" },
];

const base = { id: "f1", name: "My Flow", source: "user" as const };

describe("defToSpec", () => {
  it("converts a reader → tool → writer definition into a single-step spec", () => {
    const def: FlowDefinitionInfo = {
      id: "ai-translate",
      name: "AI Translate",
      description: "Translate content",
      source: "built-in",
      nodes: [
        { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
        {
          id: "ai-translate",
          type: "tool",
          name: "ai-translate",
          label: "AI Translate",
          position: { x: 250, y: 100 },
        },
        {
          id: "writer",
          type: "writer",
          name: "auto",
          label: "Output",
          position: { x: 500, y: 100 },
        },
      ],
      edges: [
        { id: "e1", source: "reader", target: "ai-translate" },
        { id: "e2", source: "ai-translate", target: "writer" },
      ],
    };

    const spec = defToSpec(def, "horizontal");
    expect(spec.steps).toHaveLength(1);
    expect(spec.steps[0].tool).toBe("ai-translate");
    expect(spec.sourceTransforms).toBeUndefined();
    expect(spec.description).toBe("Translate content");
  });

  it("collects source-transform nodes into spec.sourceTransforms", () => {
    const def: FlowDefinitionInfo = {
      id: "secure-translate",
      name: "Secure Translate",
      source: "built-in",
      nodes: [
        { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
        {
          id: "redact",
          type: "tool",
          name: "redact",
          label: "Redact",
          stage: "source-transform",
          position: { x: 250, y: 100 },
        },
        {
          id: "ai-translate",
          type: "tool",
          name: "ai-translate",
          label: "AI Translate",
          position: { x: 500, y: 100 },
        },
        {
          id: "writer",
          type: "writer",
          name: "auto",
          label: "Output",
          position: { x: 750, y: 100 },
        },
      ],
      edges: [],
    };

    const spec = defToSpec(def, "horizontal");
    expect(spec.sourceTransforms).toHaveLength(1);
    expect(spec.sourceTransforms![0].tool).toBe("redact");
    expect(spec.steps).toHaveLength(1);
    expect(spec.steps[0].tool).toBe("ai-translate");
  });

  it("carries config through to steps", () => {
    const def: FlowDefinitionInfo = {
      id: "f",
      name: "f",
      source: "user",
      nodes: [
        { id: "reader", type: "reader", name: "auto", position: { x: 0, y: 0 } },
        {
          id: "t",
          type: "tool",
          name: "ai-translate",
          config: { provider: "anthropic", model: "claude" },
          position: { x: 250, y: 0 },
        },
        { id: "writer", type: "writer", name: "auto", position: { x: 500, y: 0 } },
      ],
      edges: [],
    };
    const spec = defToSpec(def, "horizontal");
    expect(spec.steps[0].config).toEqual({ provider: "anthropic", model: "claude" });
  });
});

describe("specToDef", () => {
  it("produces reader/tool/writer nodes with auto reader+writer names", () => {
    const def = specToDef({ steps: [{ tool: "ai-translate" }] }, base, tools, "horizontal");
    expect(def.id).toBe("f1");
    expect(def.name).toBe("My Flow");
    expect(def.source).toBe("user");

    const reader = def.nodes.find((n) => n.type === "reader")!;
    const writer = def.nodes.find((n) => n.type === "writer")!;
    expect(reader.name).toBe("auto");
    expect(writer.name).toBe("auto");

    const tool = def.nodes.find((n) => n.type === "tool")!;
    expect(tool.name).toBe("ai-translate");
    expect(def.edges.length).toBeGreaterThanOrEqual(2);
  });

  it("marks source-transform tool nodes with stage", () => {
    const def = specToDef(
      { sourceTransforms: [{ tool: "redact" }], steps: [{ tool: "ai-translate" }] },
      base,
      tools,
      "horizontal",
    );
    const redact = def.nodes.find((n) => n.name === "redact")!;
    expect(redact.stage).toBe("source-transform");
    const aiTranslate = def.nodes.find((n) => n.name === "ai-translate")!;
    expect(aiTranslate.stage).toBeUndefined();
  });
});

describe("round-trip def → spec → def", () => {
  it("preserves tools, stage and parallel structure", () => {
    const original: FlowDefinitionInfo = {
      id: "secure",
      name: "Secure Translate",
      description: "Redact then translate",
      source: "user",
      nodes: [
        { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
        {
          id: "tool-0",
          type: "tool",
          name: "redact",
          label: "Redact",
          stage: "source-transform",
          position: { x: 250, y: 100 },
        },
        {
          id: "tool-1",
          type: "tool",
          name: "ai-translate",
          label: "AI Translate",
          position: { x: 500, y: 100 },
        },
        {
          id: "tool-2",
          type: "tool",
          name: "unredact",
          label: "Unredact",
          position: { x: 750, y: 100 },
        },
        {
          id: "writer",
          type: "writer",
          name: "auto",
          label: "Output",
          position: { x: 1000, y: 100 },
        },
      ],
      edges: [],
    };

    const spec = defToSpec(original, "horizontal");
    const back = specToDef(spec, original, tools, "horizontal");

    const toolNames = back.nodes.filter((n) => n.type === "tool").map((n) => n.name);
    expect(toolNames).toEqual(["redact", "ai-translate", "unredact"]);

    const redact = back.nodes.find((n) => n.name === "redact")!;
    expect(redact.stage).toBe("source-transform");

    // The spec round-trips identically through a second pass.
    const spec2 = defToSpec(back, "horizontal");
    expect(spec2.sourceTransforms?.map((s) => s.tool)).toEqual(["redact"]);
    expect(spec2.steps.map((s) => s.tool)).toEqual(["ai-translate", "unredact"]);
  });

  it("preserves a parallel group through spec → def → spec", () => {
    const spec = {
      steps: [
        { tool: "ai-translate" },
        { tool: "", parallel: [{ tool: "ai-qa" }, { tool: "term-check" }] },
      ],
    };
    const def = specToDef(spec, base, tools, "horizontal");
    const spec2 = defToSpec(def, "horizontal");

    expect(spec2.steps[0].tool).toBe("ai-translate");
    expect(spec2.steps[1].parallel).toBeDefined();
    expect(spec2.steps[1].parallel!.map((p) => p.tool).sort()).toEqual(["ai-qa", "term-check"]);
  });
});
