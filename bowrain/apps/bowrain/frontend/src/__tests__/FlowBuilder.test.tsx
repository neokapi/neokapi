import { describe, it, expect } from "vitest";
import { defToSpec, specToDef, type ToolInfo as EditorToolInfo } from "@neokapi/flow-editor";
import type { FlowDefinitionInfo, ToolInfo } from "../types/api";

// ---------------------------------------------------------------------------
// FlowBuilder now renders the shared @neokapi/flow-editor <FlowEditor> and
// bridges the backend's node/edge FlowDefinitionInfo to the editor's FlowSpec
// via the shared defToSpec / specToDef adapter. These tests exercise that
// adapter against bowrain's FlowDefinitionInfo type (the contract the Wails
// FlowStore round-trips), plus the snake_case → camelCase ToolInfo mapping the
// component performs before handing tools to the editor.
//
// The full component depends on live Wails bindings, so it is covered by the
// Playwright e2e suite (e2e/flow-builder.spec.ts) rather than rendered here.
// ---------------------------------------------------------------------------

const STAGE_SOURCE_TRANSFORM = "source-transform";

const tools: EditorToolInfo[] = [
  {
    name: "redact",
    description: "Redact sensitive content",
    category: "transform",
    isSourceTransform: true,
  },
  {
    name: "ai-translate",
    description: "Translate with AI",
    category: "transform",
    isSourceTransform: false,
  },
  { name: "ai-qa", description: "QA check", category: "validate" },
];

describe("FlowBuilder adapter — stage serialization round-trip", () => {
  const secureDef: FlowDefinitionInfo = {
    id: "secure-translate",
    name: "Secure Translate",
    description: "Redact then translate",
    source: "built-in",
    nodes: [
      { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
      {
        id: "redact",
        type: "tool",
        name: "redact",
        label: "Redact",
        stage: STAGE_SOURCE_TRANSFORM,
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
    edges: [
      { id: "e1", source: "reader", target: "redact" },
      { id: "e2", source: "redact", target: "ai-translate" },
      { id: "e3", source: "ai-translate", target: "writer" },
    ],
  };

  it("defToSpec collects the redact node into sourceTransforms", () => {
    const spec = defToSpec(secureDef);
    expect(spec.sourceTransforms?.map((s) => s.tool)).toEqual(["redact"]);
    expect(spec.steps.map((s) => s.tool)).toEqual(["ai-translate"]);
  });

  it("specToDef preserves source-transform stage in the serialized def", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    const redactNode = serialized.nodes.find((n) => n.name === "redact")!;
    expect(redactNode.stage).toBe(STAGE_SOURCE_TRANSFORM);
  });

  it("specToDef omits stage for main-chain nodes", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    const translateNode = serialized.nodes.find((n) => n.name === "ai-translate")!;
    expect(translateNode.stage == null).toBe(true);
  });

  it("round-trips the stage value end-to-end (def → spec → def → spec)", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    const spec2 = defToSpec(serialized);
    expect(spec2.sourceTransforms?.map((s) => s.tool)).toEqual(["redact"]);
    expect(spec2.steps.map((s) => s.tool)).toEqual(["ai-translate"]);
  });

  it("reader and writer nodes serialize with the 'auto' name", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    expect(serialized.nodes.find((n) => n.type === "reader")!.name).toBe("auto");
    expect(serialized.nodes.find((n) => n.type === "writer")!.name).toBe("auto");
  });
});

describe("FlowBuilder — ToolInfo snake_case → camelCase mapping", () => {
  /** Mirrors the editorTools mapping performed inside FlowBuilder. */
  function toEditorTool(t: ToolInfo): EditorToolInfo {
    return {
      name: t.name,
      description: t.description,
      category: t.category,
      isSourceTransform: t.is_source_transform,
    };
  }

  const backendTools: ToolInfo[] = [
    {
      name: "redact",
      description: "Redact sensitive content",
      category: "transform",
      is_source_transform: true,
    },
    {
      name: "ai-translate",
      description: "Translate with AI",
      category: "transform",
      is_source_transform: false,
    },
    { name: "pseudo-translate", description: "Pseudo translate", category: "transform" },
  ];

  it("maps is_source_transform=true to isSourceTransform=true", () => {
    const mapped = backendTools.map(toEditorTool);
    expect(mapped.find((t) => t.name === "redact")!.isSourceTransform).toBe(true);
  });

  it("maps is_source_transform=false to isSourceTransform=false", () => {
    const mapped = backendTools.map(toEditorTool);
    expect(mapped.find((t) => t.name === "ai-translate")!.isSourceTransform).toBe(false);
  });

  it("leaves isSourceTransform undefined when the field is absent", () => {
    const mapped = backendTools.map(toEditorTool);
    expect(mapped.find((t) => t.name === "pseudo-translate")!.isSourceTransform).toBeUndefined();
  });
});

describe("FlowBuilder — empty / new flow", () => {
  it("an empty flow definition produces an empty step list", () => {
    const def: FlowDefinitionInfo = {
      id: "new",
      name: "New",
      source: "user",
      nodes: [],
      edges: [],
    };
    const spec = defToSpec(def);
    expect(spec.steps).toEqual([]);
    expect(spec.sourceTransforms).toBeUndefined();
  });
});
