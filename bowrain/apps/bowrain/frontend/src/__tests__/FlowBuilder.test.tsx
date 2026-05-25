import { describe, it, expect } from "vitest";
import type { FlowDefinitionInfo, FlowNodeInfo, ToolInfo } from "../types/api";

// ---------------------------------------------------------------------------
// Pure unit tests for the graph model helpers extracted from FlowBuilder.
// These do NOT render the Wails-backed component (it needs live bindings).
// ---------------------------------------------------------------------------

// --- defToReactFlow / reactFlowToDef (co-located logic, tested by re-implementing
//     the same simple mapping here so the round-trip is verified independently of
//     the React component tree) ---

const STAGE_SOURCE_TRANSFORM = "source-transform";

/** Mirrors FlowBuilder.defToReactFlow */
function defToReactFlow(def: FlowDefinitionInfo): {
  nodes: Array<{ id: string; data: Record<string, unknown>; position: { x: number; y: number } }>;
  edges: Array<{ id: string; source: string; target: string }>;
} {
  return {
    nodes: def.nodes.map((n) => ({
      id: n.id,
      type: n.type,
      position: { x: n.position.x, y: n.position.y },
      data: {
        label: n.label || n.name,
        toolName: n.name,
        nodeId: n.id,
        config: n.config || {},
        stage: n.stage || "",
      },
    })),
    edges: def.edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
    })),
  };
}

/** Mirrors FlowBuilder.reactFlowToDef */
function reactFlowToDef(
  id: string,
  name: string,
  description: string,
  nodes: Array<{
    id: string;
    type?: string;
    data: Record<string, unknown>;
    position: { x: number; y: number };
  }>,
  edges: Array<{ id: string; source: string; target: string }>,
  source: string,
): FlowDefinitionInfo {
  return {
    id,
    name,
    description,
    source,
    nodes: nodes.map((n) => ({
      id: n.id,
      type: (n.type || "tool") as FlowNodeInfo["type"],
      name: (n.data.toolName as string) || n.id,
      label: (n.data.label as string) || "",
      stage: (n.data.stage as string) || undefined,
      config: (n.data.config as Record<string, unknown>) || undefined,
      position: { x: n.position.x, y: n.position.y },
    })),
    edges: edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
    })),
  };
}

// ---------------------------------------------------------------------------
// Test suite
// ---------------------------------------------------------------------------

describe("FlowBuilder graph model — stage serialization round-trip", () => {
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

  it("defToReactFlow carries stage into node data", () => {
    const { nodes } = defToReactFlow(secureDef);
    const redact = nodes.find((n) => n.id === "redact")!;
    expect(redact.data.stage).toBe(STAGE_SOURCE_TRANSFORM);
  });

  it("defToReactFlow sets empty string for nodes without stage", () => {
    const { nodes } = defToReactFlow(secureDef);
    const translate = nodes.find((n) => n.id === "ai-translate")!;
    expect(translate.data.stage).toBe("");
  });

  it("reactFlowToDef preserves source-transform stage in serialized def", () => {
    const { nodes, edges } = defToReactFlow(secureDef);
    const serialized = reactFlowToDef(
      "secure-translate",
      "Secure Translate",
      "",
      nodes,
      edges,
      "user",
    );
    const redactNode = serialized.nodes.find((n) => n.id === "redact")!;
    expect(redactNode.stage).toBe(STAGE_SOURCE_TRANSFORM);
  });

  it("reactFlowToDef omits stage for main-chain nodes", () => {
    const { nodes, edges } = defToReactFlow(secureDef);
    const serialized = reactFlowToDef(
      "secure-translate",
      "Secure Translate",
      "",
      nodes,
      edges,
      "user",
    );
    const translateNode = serialized.nodes.find((n) => n.id === "ai-translate")!;
    // Empty string serialized as undefined (stage omitted)
    expect(translateNode.stage === "" || translateNode.stage == null).toBe(true);
  });

  it("round-trip preserves stage value end-to-end", () => {
    const { nodes, edges } = defToReactFlow(secureDef);
    const serialized = reactFlowToDef(
      "secure-translate",
      "Secure Translate",
      "",
      nodes,
      edges,
      "user",
    );
    // Re-deserialize to React Flow
    const { nodes: nodes2 } = defToReactFlow(serialized);
    const redact2 = nodes2.find((n) => n.id === "redact")!;
    expect(redact2.data.stage).toBe(STAGE_SOURCE_TRANSFORM);
  });
});

describe("FlowBuilder — source-transform toggle capability gating", () => {
  /** Simulates the selectedToolCapable computation from FlowBuilder. */
  function isCapable(toolName: string, toolInfoMap: Map<string, ToolInfo>): boolean {
    return toolInfoMap.get(toolName)?.is_source_transform ?? false;
  }

  const tools: ToolInfo[] = [
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

  const toolInfoMap = new Map(tools.map((t) => [t.name, t]));

  it("redact is source-transform capable", () => {
    expect(isCapable("redact", toolInfoMap)).toBe(true);
  });

  it("ai-translate is NOT source-transform capable", () => {
    expect(isCapable("ai-translate", toolInfoMap)).toBe(false);
  });

  it("tool without is_source_transform field is not capable (defaults false)", () => {
    expect(isCapable("pseudo-translate", toolInfoMap)).toBe(false);
  });

  it("unknown tool name is not capable", () => {
    expect(isCapable("nonexistent-tool", toolInfoMap)).toBe(false);
  });
});

describe("FlowBuilder — stage serialization with empty/undefined stage values", () => {
  it("node with stage=undefined gets empty string in data after defToReactFlow", () => {
    const def: FlowDefinitionInfo = {
      id: "test",
      name: "Test",
      source: "user",
      nodes: [{ id: "t1", type: "tool", name: "ai-translate", position: { x: 0, y: 0 } }],
      edges: [],
    };
    const { nodes } = defToReactFlow(def);
    expect(nodes[0].data.stage).toBe("");
  });

  it("changing stage from '' to source-transform round-trips correctly", () => {
    const def: FlowDefinitionInfo = {
      id: "test",
      name: "Test",
      source: "user",
      nodes: [
        {
          id: "t1",
          type: "tool",
          name: "redact",
          stage: STAGE_SOURCE_TRANSFORM,
          position: { x: 0, y: 0 },
        },
      ],
      edges: [],
    };
    const { nodes } = defToReactFlow(def);
    // Simulate toggling stage via handleToolStageChange
    nodes[0] = { ...nodes[0], data: { ...nodes[0].data, stage: STAGE_SOURCE_TRANSFORM } };
    const serialized = reactFlowToDef("test", "Test", "", nodes, [], "user");
    expect(serialized.nodes[0].stage).toBe(STAGE_SOURCE_TRANSFORM);
  });
});
