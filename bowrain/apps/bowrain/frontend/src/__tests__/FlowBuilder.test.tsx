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
// Transformers (redact) are ordinary ordered steps — there is no stage field
// on nodes; the editor's placement pass validates a transformer's position.
//
// The full component depends on live Wails bindings, so it is covered by the
// Playwright e2e suite (e2e/flow-builder.spec.ts) rather than rendered here.
// ---------------------------------------------------------------------------

const tools: EditorToolInfo[] = [
  {
    name: "redact",
    description: "Redact sensitive content",
    category: "transform",
    isSourceTransform: true,
    recoverable: true,
  },
  {
    name: "ai-translate",
    description: "Translate with AI",
    category: "transform",
    isSourceTransform: false,
  },
  { name: "ai-qa", description: "QA check", category: "validate" },
];

describe("FlowBuilder adapter — ordered-steps round-trip", () => {
  const secureDef: FlowDefinitionInfo = {
    id: "secure-translate",
    name: "Secure Translate",
    description: "Redact then translate",
    source: "built-in",
    nodes: [
      {
        id: "redact",
        type: "tool",
        name: "redact",
        label: "Redact",
        position: { x: 250, y: 0 },
      },
      {
        id: "ai-translate",
        type: "tool",
        name: "ai-translate",
        label: "AI Translate",
        position: { x: 250, y: 260 },
      },
    ],
    edges: [{ id: "e2", source: "redact", target: "ai-translate" }],
  };

  it("defToSpec orders the redact node into steps like any other node", () => {
    const spec = defToSpec(secureDef);
    expect(spec.steps.map((s) => s.tool)).toEqual(["redact", "ai-translate"]);
  });

  it("specToDef keeps the transformer's leading position in the serialized def", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    expect(serialized.nodes.map((n) => n.name)).toEqual(["redact", "ai-translate"]);
    // The persisted chain axis is y: redact precedes ai-translate.
    expect(serialized.nodes[0].position.y).toBeLessThan(serialized.nodes[1].position.y);
  });

  it("round-trips the step order end-to-end (def → spec → def → spec)", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    const spec2 = defToSpec(serialized);
    expect(spec2.steps.map((s) => s.tool)).toEqual(["redact", "ai-translate"]);
  });

  it("serializes a tool-only graph — no reader/writer nodes (AD-026)", () => {
    const spec = defToSpec(secureDef);
    const serialized = specToDef(
      spec,
      { id: "secure-translate", name: "Secure Translate", source: "user" },
      tools,
    );
    expect(serialized.nodes.length).toBeGreaterThan(0);
    expect(serialized.nodes.every((n) => n.type === "tool")).toBe(true);
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
      recoverable: t.recoverable,
    };
  }

  const backendTools: ToolInfo[] = [
    {
      name: "redact",
      description: "Redact sensitive content",
      category: "transform",
      is_source_transform: true,
      recoverable: true,
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

  it("maps recoverable through for recoverable transformers", () => {
    const mapped = backendTools.map(toEditorTool);
    expect(mapped.find((t) => t.name === "redact")!.recoverable).toBe(true);
    expect(mapped.find((t) => t.name === "ai-translate")!.recoverable).toBeUndefined();
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
  });
});
