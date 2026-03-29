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
import { Play, Plus } from "lucide-react";

import type { FlowEditorProps, FlowSpec } from "./types";
import { ReaderNode } from "./nodes/ReaderNode";
import { WriterNode } from "./nodes/WriterNode";
import { ToolNode } from "./nodes/ToolNode";
import { stepsToGraph, graphToSteps } from "./conversion";

const nodeTypes: NodeTypes = {
  reader: ReaderNode,
  writer: WriterNode,
  tool: ToolNode,
};

/**
 * Visual flow editor component.
 *
 * Renders a steps-based FlowSpec as an auto-laid-out React Flow graph.
 * Tools can be added from the palette, reordered by dragging, and removed.
 * Changes are reported via `onChange` in steps format.
 */
export function FlowEditor({
  flow,
  tools,
  onChange,
  onRun,
  readOnly = false,
}: FlowEditorProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);

  const initial = useMemo(() => stepsToGraph(flow), [flow]);
  const [nodes, setNodes, onNodesChange] = useNodesState(initial.nodes);
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

  // Sync graph changes back to steps format on drag end.
  const handleNodeDragStop = useCallback(() => {
    const updated = graphToSteps(nodes);
    onChange(updated);
  }, [nodes, onChange]);

  const selectedTool = selectedNodeId
    ? flow.steps[parseInt(selectedNodeId.replace("tool-", ""), 10)]
    : null;

  return (
    <div className="flex h-full flex-col">
      {/* Toolbar */}
      <div className="flex items-center gap-2 border-b border-border px-3 py-2">
        {!readOnly && (
          <div className="flex flex-wrap gap-1">
            {tools.map((t) => (
              <button
                key={t.name}
                onClick={() => handleAddTool(t.name)}
                className="flex items-center gap-1 rounded border border-border px-2 py-1 text-xs hover:bg-accent"
                aria-label={`Add ${t.name} tool`}
              >
                <Plus size={10} />
                {t.name}
              </button>
            ))}
          </div>
        )}

        <div className="flex-1" />

        {onRun && (
          <button
            onClick={() => onRun(flow)}
            className="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90"
            aria-label="Run flow"
          >
            <Play size={12} />
            Run
          </button>
        )}
      </div>

      {/* Graph canvas */}
      <div className="flex-1">
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
        >
          <Controls />
          <Background variant={BackgroundVariant.Dots} gap={20} size={1} />
          <MiniMap
            nodeColor={(n) =>
              n.type === "reader"
                ? "#22c55e"
                : n.type === "writer"
                  ? "#60a5fa"
                  : "#64748b"
            }
            maskColor="rgba(0, 0, 0, 0.5)"
          />
        </ReactFlow>
      </div>

      {/* Selected node info */}
      {selectedTool && (
        <div className="border-t border-border p-3">
          <div className="flex items-center justify-between">
            <div>
              <span className="text-sm font-medium">{selectedTool.tool}</span>
              {selectedTool.config && (
                <span className="ml-2 text-xs text-muted-foreground">
                  {Object.keys(selectedTool.config).length} config values
                </span>
              )}
            </div>
            {!readOnly && (
              <button
                onClick={handleRemoveSelected}
                className="rounded px-2 py-1 text-xs text-destructive hover:bg-destructive/10"
                aria-label="Remove selected tool"
              >
                Remove
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
