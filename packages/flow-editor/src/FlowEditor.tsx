import { useMemo, useCallback, useState } from "react";
import {
  ReactFlow,
  Controls,
  Background,
  MiniMap,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  type NodeTypes,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Play, X } from "lucide-react";

import type { FlowEditorProps, FlowSpec, ToolInfo, ComponentSchema } from "./types";
import { ReaderNode } from "./nodes/ReaderNode";
import { WriterNode } from "./nodes/WriterNode";
import { ToolNode } from "./nodes/ToolNode";
import { ToolPalette } from "./ToolPalette";
import { SchemaForm } from "./SchemaForm";
import { stepsToGraph, graphToSteps } from "./conversion";
import { getCategoryStyle } from "./category";
import { theme } from "./theme";

const nodeTypes: NodeTypes = {
  reader: ReaderNode,
  writer: WriterNode,
  tool: ToolNode,
};

/**
 * Visual flow editor with tool palette and schema-driven config panel.
 *
 * Three-column layout: Palette | Canvas | Config (when selected).
 * Tools can be added from palette via click or drag.
 * Category-colored nodes with connection ports.
 */
export function FlowEditor({
  flow,
  tools,
  onChange,
  onRun,
  onGetSchema,
  readOnly = false,
}: FlowEditorProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  // Build tool lookup map for enriching nodes with category/description
  const toolMap = useMemo(() => {
    const m = new Map<string, ToolInfo>();
    for (const t of tools) m.set(t.name, t);
    return m;
  }, [tools]);

  const initial = useMemo(() => stepsToGraph(flow, toolMap), [flow, toolMap]);
  const [nodes, , onNodesChange] = useNodesState(initial.nodes);
  const [edges, , onEdgesChange] = useEdgesState(initial.edges);

  const handleNodesChange = useCallback(
    (changes: Parameters<typeof onNodesChange>[0]) => {
      onNodesChange(changes);
    },
    [onNodesChange],
  );

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNodeId(node.id);
  }, []);

  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null);
  }, []);

  const handleAddTool = useCallback(
    (toolName: string) => {
      if (readOnly) return;
      const updated: FlowSpec = {
        ...flow,
        steps: [...flow.steps, { tool: toolName }],
      };
      onChange(updated);
    },
    [flow, onChange, readOnly],
  );

  const handleRemoveSelected = useCallback(() => {
    if (!selectedNodeId || readOnly) return;
    const toolIndex = parseInt(selectedNodeId.replace("tool-", ""), 10);
    if (isNaN(toolIndex)) return;
    const updated: FlowSpec = {
      ...flow,
      steps: flow.steps.filter((_, i) => i !== toolIndex),
    };
    onChange(updated);
    setSelectedNodeId(null);
  }, [selectedNodeId, flow, onChange, readOnly]);

  // Handle drag-and-drop from palette
  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      const toolName = e.dataTransfer.getData("application/neokapi-tool");
      if (toolName && !readOnly) {
        handleAddTool(toolName);
      }
    },
    [handleAddTool, readOnly],
  );

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = "copy";
  }, []);

  // Sync graph changes back to steps format on drag end.
  const handleNodeDragStop = useCallback(() => {
    const updated = graphToSteps(nodes);
    onChange(updated);
  }, [nodes, onChange]);

  // Config panel state
  const selectedToolIndex = selectedNodeId
    ? parseInt(selectedNodeId.replace("tool-", ""), 10)
    : NaN;
  const selectedStep = !isNaN(selectedToolIndex)
    ? flow.steps[selectedToolIndex]
    : null;
  const selectedToolInfo = selectedStep ? toolMap.get(selectedStep.tool) : null;
  const selectedSchema = selectedStep && onGetSchema
    ? onGetSchema(selectedStep.tool)
    : null;

  const handleConfigChange = useCallback(
    (config: Record<string, unknown>) => {
      if (isNaN(selectedToolIndex) || readOnly) return;
      const updated: FlowSpec = {
        ...flow,
        steps: flow.steps.map((s, i) =>
          i === selectedToolIndex ? { ...s, config } : s,
        ),
      };
      onChange(updated);
    },
    [selectedToolIndex, flow, onChange, readOnly],
  );

  return (
    <div style={{ display: "flex", height: "100%", overflow: "hidden" }}>
      {/* Tool Palette (left) */}
      {!readOnly && <ToolPalette tools={tools} onAddTool={handleAddTool} />}

      {/* Canvas (center) */}
      <div style={{ flex: 1, display: "flex", flexDirection: "column" }}>
        {/* Toolbar */}
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: 8,
            padding: "6px 12px",
            borderBottom: `1px solid ${theme.border}`,
            background: theme.bg,
          }}
        >
          <span
            style={{
              fontSize: 12,
              fontWeight: 600,
              color: theme.fgMuted,
            }}
          >
            {flow.steps.length} step{flow.steps.length !== 1 ? "s" : ""}
          </span>

          <div style={{ flex: 1 }} />

          {onRun && (
            <button
              onClick={() => onRun(flow)}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 6,
                padding: "5px 14px",
                borderRadius: 6,
                border: "none",
                background: theme.accent,
                color: theme.accentFg,
                fontSize: 12,
                fontWeight: 600,
                cursor: "pointer",
              }}
              aria-label="Run flow"
            >
              <Play size={12} />
              Run
            </button>
          )}
        </div>

        {/* Graph canvas */}
        <div style={{ flex: 1 }} onDrop={handleDrop} onDragOver={handleDragOver}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={handleNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={handleNodeClick}
            onPaneClick={handlePaneClick}
            onNodeDragStop={handleNodeDragStop}
            nodeTypes={nodeTypes}
            nodesDraggable={!readOnly}
            nodesConnectable={!readOnly}
            fitView
            fitViewOptions={{ padding: 0.3 }}
            proOptions={{ hideAttribution: true }}
            defaultEdgeOptions={{
              style: { stroke: theme.fgMuted, strokeWidth: 2 },
              animated: false,
            }}
          >
            <Controls
              showInteractive={false}
              style={{ background: theme.bgCard, borderColor: theme.border }}
            />
            <Background
              variant={BackgroundVariant.Dots}
              gap={24}
              size={1}
              color={theme.border}
            />
            <MiniMap
              nodeColor={(n) => {
                if (n.type === "reader") return "oklch(0.7 0.17 145)";
                if (n.type === "writer") return "oklch(0.7 0.13 85)";
                const cat = (n.data?.category as string) || "pipeline";
                return getCategoryStyle(cat).color;
              }}
              maskColor="oklch(0 0 0 / 0.6)"
              style={{
                background: theme.bg,
                borderColor: theme.border,
              }}
            />
          </ReactFlow>
        </div>
      </div>

      {/* Config Panel (right) */}
      {selectedStep && (
        <ConfigPanel
          step={selectedStep}
          toolInfo={selectedToolInfo}
          schema={selectedSchema}
          config={selectedStep.config || {}}
          onConfigChange={handleConfigChange}
          onClose={() => setSelectedNodeId(null)}
          onRemove={readOnly ? undefined : handleRemoveSelected}
        />
      )}
    </div>
  );
}

function ConfigPanel({
  step,
  toolInfo,
  schema,
  config,
  onConfigChange,
  onClose,
  onRemove,
}: {
  step: { tool: string };
  toolInfo: ToolInfo | null | undefined;
  schema: ComponentSchema | null | undefined;
  config: Record<string, unknown>;
  onConfigChange: (config: Record<string, unknown>) => void;
  onClose: () => void;
  onRemove?: () => void;
}) {
  const category = toolInfo?.category || "pipeline";
  const catStyle = getCategoryStyle(category);
  const Icon = catStyle.icon;
  const displayName = step.tool.replace(/^okapi:/, "");

  return (
    <div
      style={{
        width: 280,
        display: "flex",
        flexDirection: "column",
        borderLeft: `1px solid ${theme.border}`,
        background: theme.bg,
        overflow: "hidden",
      }}
    >
      {/* Header */}
      <div
        style={{
          padding: "10px 12px",
          borderBottom: `1px solid ${theme.border}`,
          display: "flex",
          flexDirection: "column",
          gap: 6,
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
          <div
            style={{
              width: 3,
              height: 20,
              borderRadius: 2,
              background: catStyle.color,
              flexShrink: 0,
            }}
          />
          <div style={{ flex: 1, minWidth: 0 }}>
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 4,
                marginBottom: 2,
              }}
            >
              <Icon size={11} style={{ color: catStyle.text }} />
              <span
                style={{
                  fontSize: 9,
                  fontWeight: 700,
                  letterSpacing: "0.06em",
                  textTransform: "uppercase",
                  color: catStyle.text,
                }}
              >
                {catStyle.label}
              </span>
            </div>
            <div
              style={{
                fontSize: 14,
                fontWeight: 600,
                color: theme.fg,
              }}
            >
              {displayName}
            </div>
          </div>
          <button
            onClick={onClose}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              padding: 4,
              borderRadius: 4,
              alignSelf: "flex-start",
            }}
            aria-label="Close panel"
          >
            <X size={14} style={{ color: theme.fgMuted }} />
          </button>
        </div>

        {toolInfo?.description && (
          <div
            style={{
              fontSize: 11,
              color: theme.fgMuted,
              lineHeight: 1.4,
            }}
          >
            {toolInfo.description}
          </div>
        )}

        {/* Requirements badges */}
        {toolInfo?.requires && toolInfo.requires.length > 0 && (
          <div style={{ display: "flex", gap: 4, flexWrap: "wrap" }}>
            {toolInfo.requires.map((req) => (
              <span
                key={req}
                style={{
                  fontSize: 9,
                  padding: "2px 6px",
                  borderRadius: 4,
                  background: theme.bgSecondary,
                  color: theme.fgMuted,
                  fontWeight: 500,
                }}
              >
                {req}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Config form */}
      <div style={{ flex: 1, overflow: "auto", padding: "8px 12px" }}>
        {schema ? (
          <SchemaForm
            schema={schema}
            values={config}
            onChange={onConfigChange}
            compact
          />
        ) : (
          <div
            style={{
              fontSize: 11,
              color: theme.fgMuted,
              textAlign: "center",
              padding: "20px 0",
              fontStyle: "italic",
            }}
          >
            {toolInfo?.has_schema
              ? "Loading configuration..."
              : "No configurable parameters"}
          </div>
        )}
      </div>

      {/* Footer */}
      {onRemove && (
        <div
          style={{
            padding: "8px 12px",
            borderTop: `1px solid ${theme.border}`,
          }}
        >
          <button
            onClick={onRemove}
            style={{
              width: "100%",
              padding: "5px 0",
              borderRadius: 4,
              border: `1px solid ${theme.destructive}44`,
              background: `${theme.destructive}18`,
              color: theme.destructive,
              fontSize: 11,
              fontWeight: 500,
              cursor: "pointer",
            }}
            aria-label="Remove tool from flow"
          >
            Remove from flow
          </button>
        </div>
      )}
    </div>
  );
}
