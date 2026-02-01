import { useState, useCallback, useMemo, useRef } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  addEdge,
  useNodesState,
  useEdgesState,
  BackgroundVariant,
  type Connection,
  type Node,
  type Edge,
  type NodeTypes,
  type NodeProps,
  Handle,
  Position,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useFlowDefinitions, useFlowDefinitionApi, useTools } from "../hooks/useApi";
import type { FlowDefinitionInfo, FlowNodeInfo, FlowEdgeInfo, ToolInfo } from "../types/api";

// --- Custom Node Components ---

const nodeColors: Record<string, { bg: string; border: string; label: string }> = {
  reader: { bg: "#dcfce7", border: "#16a34a", label: "Input" },
  writer: { bg: "#dbeafe", border: "#2563eb", label: "Output" },
  tool: { bg: "#f3f4f6", border: "#6b7280", label: "Tool" },
};

function ReaderNode({ data }: NodeProps) {
  const colors = nodeColors.reader;
  return (
    <div
      data-testid={`flow-node-${data.nodeId}`}
      style={{
        padding: "10px 16px",
        borderRadius: 8,
        border: `2px solid ${colors.border}`,
        background: colors.bg,
        minWidth: 140,
        textAlign: "center",
        fontSize: 13,
      }}
    >
      <div style={{ fontSize: 10, color: colors.border, fontWeight: 600, marginBottom: 2 }}>
        INPUT
      </div>
      <div style={{ fontWeight: 600 }}>{(data.label as string) || "Reader"}</div>
      <div style={{ fontSize: 11, color: "#6b7280", marginTop: 2 }}>{data.formatName as string}</div>
      <Handle type="source" position={Position.Right} style={{ background: colors.border }} />
    </div>
  );
}

function WriterNode({ data }: NodeProps) {
  const colors = nodeColors.writer;
  return (
    <div
      data-testid={`flow-node-${data.nodeId}`}
      style={{
        padding: "10px 16px",
        borderRadius: 8,
        border: `2px solid ${colors.border}`,
        background: colors.bg,
        minWidth: 140,
        textAlign: "center",
        fontSize: 13,
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: colors.border }} />
      <div style={{ fontSize: 10, color: colors.border, fontWeight: 600, marginBottom: 2 }}>
        OUTPUT
      </div>
      <div style={{ fontWeight: 600 }}>{(data.label as string) || "Writer"}</div>
      <div style={{ fontSize: 11, color: "#6b7280", marginTop: 2 }}>{data.formatName as string}</div>
    </div>
  );
}

function ToolNode({ data, selected }: NodeProps) {
  const colors = nodeColors.tool;
  return (
    <div
      data-testid={`flow-node-${data.nodeId}`}
      style={{
        padding: "10px 16px",
        borderRadius: 8,
        border: `2px solid ${selected ? "var(--accent, #3b82f6)" : colors.border}`,
        background: selected ? "#eff6ff" : colors.bg,
        minWidth: 140,
        textAlign: "center",
        fontSize: 13,
        boxShadow: selected ? "0 0 0 2px rgba(59,130,246,0.3)" : "none",
      }}
    >
      <Handle type="target" position={Position.Left} style={{ background: colors.border }} />
      <div style={{ fontSize: 10, color: "#9ca3af", fontWeight: 600, marginBottom: 2 }}>
        TOOL
      </div>
      <div style={{ fontWeight: 600 }}>{(data.label as string) || (data.toolName as string)}</div>
      {data.description ? (
        <div style={{ fontSize: 11, color: "#6b7280", marginTop: 2 }}>{String(data.description)}</div>
      ) : null}
      <Handle type="source" position={Position.Right} style={{ background: colors.border }} />
    </div>
  );
}

const nodeTypes: NodeTypes = {
  reader: ReaderNode,
  writer: WriterNode,
  tool: ToolNode,
};

// --- Conversion between API types and React Flow types ---

function defToReactFlow(def: FlowDefinitionInfo): { nodes: Node[]; edges: Edge[] } {
  const nodes: Node[] = def.nodes.map((n: FlowNodeInfo) => ({
    id: n.id,
    type: n.type,
    position: { x: n.position.x, y: n.position.y },
    data: {
      label: n.label || n.name,
      toolName: n.name,
      formatName: n.name === "auto" ? "Auto-detect" : n.name,
      nodeId: n.id,
    },
  }));
  const edges: Edge[] = def.edges.map((e: FlowEdgeInfo) => ({
    id: e.id,
    source: e.source,
    target: e.target,
    animated: true,
    style: { stroke: "#94a3b8", strokeWidth: 2 },
  }));
  return { nodes, edges };
}

function reactFlowToDef(
  id: string,
  name: string,
  description: string,
  nodes: Node[],
  edges: Edge[],
  source: string,
): FlowDefinitionInfo {
  return {
    id,
    name,
    description,
    source,
    nodes: nodes.map((n) => ({
      id: n.id,
      type: (n.type || "tool") as "tool" | "reader" | "writer",
      name: (n.data.toolName as string) || n.id,
      label: (n.data.label as string) || "",
      position: { x: n.position.x, y: n.position.y },
    })),
    edges: edges.map((e) => ({
      id: e.id,
      source: e.source,
      target: e.target,
    })),
  };
}

// --- Tool Palette ---

function ToolPalette({ tools, onAddTool }: { tools: ToolInfo[]; onAddTool: (tool: ToolInfo) => void }) {
  return (
    <div
      data-testid="tool-palette"
      style={{
        padding: 12,
        borderBottom: "1px solid var(--border, #e5e7eb)",
        display: "flex",
        gap: 8,
        flexWrap: "wrap",
        alignItems: "center",
      }}
    >
      <span style={{ fontSize: 12, fontWeight: 600, color: "var(--text-secondary, #6b7280)" }}>
        Add Tool:
      </span>
      {tools.map((tool) => (
        <button
          key={tool.name}
          data-testid={`add-tool-${tool.name}`}
          onClick={() => onAddTool(tool)}
          style={{
            padding: "4px 10px",
            fontSize: 12,
            border: "1px solid var(--border, #d1d5db)",
            borderRadius: 6,
            background: "var(--bg-primary, #fff)",
            color: "var(--text-primary, #111)",
            cursor: "pointer",
          }}
        >
          {tool.name}
        </button>
      ))}
    </div>
  );
}

// --- Flow List ---

function FlowList({
  definitions,
  activeId,
  onSelect,
  onNew,
}: {
  definitions: FlowDefinitionInfo[];
  activeId: string | null;
  onSelect: (def: FlowDefinitionInfo) => void;
  onNew: () => void;
}) {
  return (
    <div
      data-testid="flow-list"
      style={{
        width: 240,
        borderRight: "1px solid var(--border, #e5e7eb)",
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
      }}
    >
      <div
        style={{
          padding: "12px 16px",
          borderBottom: "1px solid var(--border, #e5e7eb)",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
        }}
      >
        <span style={{ fontWeight: 600, fontSize: 14 }}>Flows</span>
        <button
          data-testid="new-flow-btn"
          onClick={onNew}
          style={{
            padding: "4px 10px",
            fontSize: 12,
            border: "1px solid var(--border, #d1d5db)",
            borderRadius: 6,
            background: "var(--accent, #3b82f6)",
            color: "#fff",
            cursor: "pointer",
          }}
        >
          + New
        </button>
      </div>
      <div style={{ flex: 1, overflow: "auto", padding: "4px 0" }}>
        {definitions.map((def) => (
          <button
            key={def.id}
            data-testid={`flow-item-${def.id}`}
            onClick={() => onSelect(def)}
            style={{
              width: "100%",
              padding: "10px 16px",
              textAlign: "left",
              border: "none",
              borderLeft: activeId === def.id ? "3px solid var(--accent, #3b82f6)" : "3px solid transparent",
              background: activeId === def.id ? "var(--bg-tertiary, #f3f4f6)" : "transparent",
              cursor: "pointer",
              fontSize: 13,
            }}
          >
            <div style={{ fontWeight: 500 }}>{def.name}</div>
            <div style={{ fontSize: 11, color: "var(--text-secondary, #6b7280)", marginTop: 2 }}>
              {def.source} &middot; {def.nodes.filter((n) => n.type === "tool").length} tool(s)
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

// --- Main FlowBuilder Component ---

export function FlowBuilder() {
  const { definitions, refresh } = useFlowDefinitions();
  const { saveFlowDefinition, deleteFlowDefinition } = useFlowDefinitionApi();
  const { tools } = useTools();

  const [activeDef, setActiveDef] = useState<FlowDefinitionInfo | null>(null);
  const [editName, setEditName] = useState("");
  const [editDescription, setEditDescription] = useState("");
  const [dirty, setDirty] = useState(false);
  const nodeCounter = useRef(0);

  const [nodes, setNodes, onNodesChange] = useNodesState<Node>([]);
  const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

  const isBuiltIn = activeDef?.source === "built-in";

  const onConnect = useCallback(
    (connection: Connection) => {
      setEdges((eds) => addEdge(connection, eds).map((e) => ({ ...e, animated: true, style: { stroke: "#94a3b8", strokeWidth: 2 } })));
      setDirty(true);
    },
    [setEdges],
  );

  const handleSelect = useCallback(
    (def: FlowDefinitionInfo) => {
      setActiveDef(def);
      setEditName(def.name);
      setEditDescription(def.description || "");
      const { nodes: n, edges: e } = defToReactFlow(def);
      setNodes(n);
      setEdges(e);
      setDirty(false);
    },
    [setNodes, setEdges],
  );

  const handleNew = useCallback(() => {
    const id = `custom-flow-${Date.now()}`;
    const def: FlowDefinitionInfo = {
      id,
      name: "New Flow",
      description: "",
      source: "user",
      nodes: [
        { id: "reader", type: "reader", name: "auto", label: "Input", position: { x: 0, y: 100 } },
        { id: "writer", type: "writer", name: "auto", label: "Output", position: { x: 500, y: 100 } },
      ],
      edges: [],
    };
    setActiveDef(def);
    setEditName(def.name);
    setEditDescription("");
    const { nodes: n, edges: e } = defToReactFlow(def);
    setNodes(n);
    setEdges(e);
    setDirty(true);
  }, [setNodes, setEdges]);

  const handleAddTool = useCallback(
    (tool: ToolInfo) => {
      nodeCounter.current++;
      const id = `tool-${nodeCounter.current}-${Date.now()}`;
      // Place new tool roughly in the middle
      const maxX = nodes.reduce((max, n) => Math.max(max, n.position.x), 0);
      const newNode: Node = {
        id,
        type: "tool",
        position: { x: Math.min(maxX + 200, 400), y: 100 },
        data: {
          label: tool.name,
          toolName: tool.name,
          description: tool.description,
          nodeId: id,
        },
      };
      setNodes((prev) => [...prev, newNode]);
      setDirty(true);
    },
    [nodes, setNodes],
  );

  const handleSave = useCallback(async () => {
    if (!activeDef) return;
    const def = reactFlowToDef(activeDef.id, editName, editDescription, nodes, edges, "user");
    try {
      const saved = await saveFlowDefinition(def);
      setActiveDef(saved);
      setDirty(false);
      refresh();
    } catch (e) {
      console.error("Save flow failed:", e);
    }
  }, [activeDef, editName, editDescription, nodes, edges, saveFlowDefinition, refresh]);

  const handleDelete = useCallback(async () => {
    if (!activeDef || isBuiltIn) return;
    try {
      await deleteFlowDefinition(activeDef.id);
      setActiveDef(null);
      setNodes([]);
      setEdges([]);
      refresh();
    } catch (e) {
      console.error("Delete flow failed:", e);
    }
  }, [activeDef, isBuiltIn, deleteFlowDefinition, setNodes, setEdges, refresh]);

  const handleNodesChange = useCallback(
    (...args: Parameters<typeof onNodesChange>) => {
      onNodesChange(...args);
      setDirty(true);
    },
    [onNodesChange],
  );

  const handleEdgesChange = useCallback(
    (...args: Parameters<typeof onEdgesChange>) => {
      onEdgesChange(...args);
      setDirty(true);
    },
    [onEdgesChange],
  );

  const miniMapNodeColor = useMemo(
    () => (node: Node) => {
      switch (node.type) {
        case "reader":
          return nodeColors.reader.border;
        case "writer":
          return nodeColors.writer.border;
        default:
          return nodeColors.tool.border;
      }
    },
    [],
  );

  return (
    <div data-testid="flow-builder" style={{ display: "flex", flex: 1, minHeight: 0, borderRadius: 8, border: "1px solid var(--border, #e5e7eb)", overflow: "hidden" }}>
      <FlowList
        definitions={definitions}
        activeId={activeDef?.id || null}
        onSelect={handleSelect}
        onNew={handleNew}
      />
      <div style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
        {activeDef ? (
          <>
            {/* Toolbar */}
            <div
              data-testid="flow-toolbar"
              style={{
                padding: "8px 16px",
                borderBottom: "1px solid var(--border, #e5e7eb)",
                display: "flex",
                gap: 12,
                alignItems: "center",
              }}
            >
              <input
                data-testid="flow-name-input"
                value={editName}
                onChange={(e) => { setEditName(e.target.value); setDirty(true); }}
                disabled={isBuiltIn}
                style={{
                  fontWeight: 600,
                  fontSize: 16,
                  border: isBuiltIn ? "none" : "1px solid var(--border, #d1d5db)",
                  borderRadius: 4,
                  padding: "4px 8px",
                  background: isBuiltIn ? "transparent" : "var(--bg-primary, #fff)",
                  flex: 1,
                  maxWidth: 300,
                }}
              />
              <input
                data-testid="flow-description-input"
                value={editDescription}
                onChange={(e) => { setEditDescription(e.target.value); setDirty(true); }}
                placeholder="Description..."
                disabled={isBuiltIn}
                style={{
                  fontSize: 13,
                  border: isBuiltIn ? "none" : "1px solid var(--border, #d1d5db)",
                  borderRadius: 4,
                  padding: "4px 8px",
                  background: isBuiltIn ? "transparent" : "var(--bg-primary, #fff)",
                  flex: 1,
                  color: "var(--text-secondary, #6b7280)",
                }}
              />
              <span
                style={{
                  fontSize: 11,
                  padding: "2px 8px",
                  borderRadius: 4,
                  background: isBuiltIn ? "#dbeafe" : "#dcfce7",
                  color: isBuiltIn ? "#2563eb" : "#16a34a",
                  fontWeight: 600,
                }}
              >
                {activeDef.source}
              </span>
              {!isBuiltIn && (
                <>
                  <button
                    data-testid="save-flow-btn"
                    onClick={handleSave}
                    disabled={!dirty}
                    style={{
                      padding: "6px 14px",
                      fontSize: 13,
                      fontWeight: 600,
                      border: "none",
                      borderRadius: 6,
                      background: dirty ? "var(--accent, #3b82f6)" : "#94a3b8",
                      color: "#fff",
                      cursor: dirty ? "pointer" : "default",
                    }}
                  >
                    Save
                  </button>
                  <button
                    data-testid="delete-flow-btn"
                    onClick={handleDelete}
                    style={{
                      padding: "6px 14px",
                      fontSize: 13,
                      border: "1px solid #ef4444",
                      borderRadius: 6,
                      background: "transparent",
                      color: "#ef4444",
                      cursor: "pointer",
                    }}
                  >
                    Delete
                  </button>
                </>
              )}
            </div>
            {/* Tool palette (only for editable flows) */}
            {!isBuiltIn && <ToolPalette tools={tools} onAddTool={handleAddTool} />}
            {/* React Flow canvas */}
            <div style={{ flex: 1, minHeight: 0 }}>
              <ReactFlow
                nodes={nodes}
                edges={edges}
                onNodesChange={handleNodesChange}
                onEdgesChange={handleEdgesChange}
                onConnect={onConnect}
                nodeTypes={nodeTypes}
                fitView
                nodesDraggable={!isBuiltIn}
                nodesConnectable={!isBuiltIn}
                elementsSelectable={!isBuiltIn}
                deleteKeyCode={isBuiltIn ? null : "Backspace"}
                proOptions={{ hideAttribution: true }}
              >
                <Background variant={BackgroundVariant.Dots} gap={16} size={1} />
                <Controls />
                <MiniMap nodeColor={miniMapNodeColor} style={{ height: 80 }} />
              </ReactFlow>
            </div>
          </>
        ) : (
          <div
            data-testid="flow-empty-state"
            style={{
              flex: 1,
              display: "flex",
              alignItems: "center",
              justifyContent: "center",
              color: "var(--text-secondary, #6b7280)",
              fontSize: 14,
            }}
          >
            Select a flow from the list or create a new one
          </div>
        )}
      </div>
    </div>
  );
}
