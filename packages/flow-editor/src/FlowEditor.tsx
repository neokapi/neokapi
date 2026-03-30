import { useMemo, useCallback, useState, useEffect } from "react";
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  useNodesState,
  useEdgesState,
  type NodeTypes,
  type Node,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Play, X, GitBranch, Zap, Eye, RefreshCw } from "lucide-react";

import type { FlowEditorProps, FlowSpec, FlowStep, ToolInfo, ComponentSchema } from "./types";
import { ReaderNode } from "./nodes/ReaderNode";
import { WriterNode } from "./nodes/WriterNode";
import { ToolNode } from "./nodes/ToolNode";
import { ToolPalette } from "./ToolPalette";
import { FlowTemplateLibrary } from "./FlowTemplateLibrary";
import { SchemaForm } from "./SchemaForm";
import { stepsToGraph, graphToSteps } from "./conversion";
import { getCategoryStyle } from "./category";
import { suggestParallelGroups, type ParallelSuggestion } from "./parallelChecker";
import { TraceTimeline } from "./TraceTimeline";
import { PartInspector } from "./PartInspector";
import { PreviewPanel } from "./PreviewPanel";
import { computeNodeStats } from "./traceTypes";
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
  traceEvents,
  trace,
}: FlowEditorProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [dismissedSuggestions, setDismissedSuggestions] = useState(false);
  const [dismissedTemplates, setDismissedTemplates] = useState(false);

  const showTemplates = !readOnly && !dismissedTemplates && flow.steps.length === 0;
  const [inspectingNodeId, setInspectingNodeId] = useState<string | null>(null);
  const [showPreview, setShowPreview] = useState(false);

  // Build tool lookup map for enriching nodes with category/description
  const toolMap = useMemo(() => {
    const m = new Map<string, ToolInfo>();
    for (const t of tools) m.set(t.name, t);
    return m;
  }, [tools]);

  // Analyze flow for parallelization opportunities.
  const suggestions = useMemo(
    () => (readOnly || dismissedSuggestions ? [] : suggestParallelGroups(flow, toolMap)),
    [flow, toolMap, readOnly, dismissedSuggestions],
  );

  const initial = useMemo(() => stepsToGraph(flow, toolMap), [flow, toolMap]);

  // Compute per-node trace stats for execution state overlay.
  const nodeStats = useMemo(
    () => (traceEvents ? computeNodeStats(traceEvents) : null),
    [traceEvents],
  );

  const nodeNames = useMemo(() => {
    const m = new Map<string, string>();
    for (const n of initial.nodes) {
      if (n.type === "tool") m.set(n.id, String(n.data.toolName ?? n.data.label));
    }
    return m;
  }, [initial.nodes]);

  // Enrich nodes with execution state from trace data.
  const enrichedNodes = useMemo(() => {
    if (!nodeStats) return initial.nodes;
    return initial.nodes.map((n) => {
      const stats = nodeStats.get(n.id);
      if (!stats) return n;
      return {
        ...n,
        data: {
          ...n.data,
          execState: stats.hasError ? "error" : stats.partsProcessed > 0 ? "complete" : undefined,
          partCount: stats.partsProcessed,
        },
      };
    });
  }, [initial.nodes, nodeStats]);

  const [nodes, setNodes, onNodesChange] = useNodesState(enrichedNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(initial.edges);

  // Sync graph when flow prop changes (e.g., tool added, template selected).
  useEffect(() => {
    setNodes(enrichedNodes);
    setEdges(initial.edges);
  }, [enrichedNodes, initial.edges, setNodes, setEdges]);

  const handleNodesChange = useCallback(
    (changes: Parameters<typeof onNodesChange>[0]) => {
      onNodesChange(changes);
    },
    [onNodesChange],
  );

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedNodeId(node.id);
    // If we have trace data, also open the part inspector for this node.
    if (trace && node.type === "tool") {
      setInspectingNodeId(node.id);
    }
  }, [trace]);

  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null);
    setInspectingNodeId(null);
  }, []);

  const handleSelectTemplate = useCallback(
    (spec: FlowSpec) => {
      if (readOnly) return;
      onChange(spec);
      setDismissedTemplates(true);
    },
    [onChange, readOnly],
  );

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

  // Parallelize: convert sequential steps at given indices into a single parallel step.
  const handleParallelize = useCallback(
    (suggestion: ParallelSuggestion) => {
      if (readOnly) return;
      const indices = new Set(suggestion.stepIndices);
      const parallelBranches: FlowStep[] = [];
      const newSteps: FlowStep[] = [];
      let inserted = false;

      for (let i = 0; i < flow.steps.length; i++) {
        if (indices.has(i)) {
          parallelBranches.push(flow.steps[i]);
          if (!inserted) {
            // Insert the parallel group at the position of the first branch.
            newSteps.push({ tool: "", parallel: parallelBranches });
            inserted = true;
          }
        } else {
          newSteps.push(flow.steps[i]);
        }
      }
      // Update the parallel reference (it was pushed before all branches were added).
      if (inserted) {
        const pStep = newSteps.find((s) => s.parallel === parallelBranches);
        if (pStep) pStep.parallel = [...parallelBranches];
      }

      onChange({ ...flow, steps: newSteps });
    },
    [flow, onChange, readOnly],
  );

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

  // Connection validation — only allow connecting compatible port types.
  const isValidConnection = useCallback((connection: { source: string | null; target: string | null }) => {
    const sourceNode = nodes.find((n) => n.id === connection.source);
    const targetNode = nodes.find((n) => n.id === connection.target);
    if (!sourceNode || !targetNode) return true;
    const srcOutputs = sourceNode.data.outputs as string[] | undefined;
    const tgtInputs = targetNode.data.inputs as string[] | undefined;
    if (!srcOutputs || !tgtInputs) return true; // no metadata = allow
    return srcOutputs.some((o) => tgtInputs.includes(o));
  }, [nodes]);

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

          <button
            onClick={() => setShowPreview((p) => !p)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 4,
              padding: "5px 10px",
              borderRadius: 6,
              border: `1px solid ${showPreview ? theme.accent : theme.border}`,
              background: showPreview ? `${theme.accent}18` : "transparent",
              color: showPreview ? theme.accent : theme.fgMuted,
              fontSize: 12,
              fontWeight: 500,
              cursor: "pointer",
            }}
            aria-label="Toggle preview"
          >
            <Eye size={12} />
            Preview
          </button>

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

        {/* Parallelization suggestion banner */}
        {suggestions.length > 0 && (
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 8,
              padding: "6px 12px",
              borderBottom: `1px solid ${theme.border}`,
              background: theme.bgSecondary,
              fontSize: 11,
            }}
          >
            <Zap size={13} style={{ color: theme.accent, flexShrink: 0 }} />
            <span style={{ color: theme.fgMuted, flex: 1 }}>
              <strong style={{ color: theme.fg }}>{suggestions[0].toolNames.join(", ")}</strong>
              {" "}can run in parallel &mdash; {suggestions[0].reason}
            </span>
            <button
              onClick={() => handleParallelize(suggestions[0])}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 4,
                padding: "3px 10px",
                borderRadius: 4,
                border: "none",
                background: theme.accent,
                color: theme.accentFg,
                fontSize: 11,
                fontWeight: 600,
                cursor: "pointer",
                whiteSpace: "nowrap",
              }}
            >
              <GitBranch size={11} />
              Parallelize
            </button>
            <button
              onClick={() => setDismissedSuggestions(true)}
              style={{
                background: "none",
                border: "none",
                cursor: "pointer",
                padding: 2,
              }}
              aria-label="Dismiss suggestion"
            >
              <X size={12} style={{ color: theme.fgMuted }} />
            </button>
          </div>
        )}

        {/* Template library (shown when flow is empty) */}
        {showTemplates && (
          <div style={{ flex: 1, overflow: "auto", background: theme.bg }}>
            <FlowTemplateLibrary
              onSelect={handleSelectTemplate}
              onDismiss={() => setDismissedTemplates(true)}
            />
          </div>
        )}

        {/* Graph canvas */}
        {!showTemplates && (
        <div style={{ flex: 1 }} onDrop={handleDrop} onDragOver={handleDragOver}>
          <ReactFlow
            nodes={nodes}
            edges={edges}
            onNodesChange={handleNodesChange}
            onEdgesChange={onEdgesChange}
            onNodeClick={handleNodeClick}
            onPaneClick={handlePaneClick}
            onNodeDragStop={handleNodeDragStop}
            isValidConnection={isValidConnection}
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
            <Background
              variant={BackgroundVariant.Dots}
              gap={24}
              size={1}
              color={theme.border}
            />
          </ReactFlow>
        </div>
        )}

        {/* Trace timeline (bottom of canvas column) */}
        {traceEvents && traceEvents.length > 0 && (
          <TraceTimeline
            events={traceEvents}
            nodeNames={nodeNames}
            totalDurationUs={trace?.durationUs}
          />
        )}

        {/* Preview panel (bottom of canvas column) */}
        {showPreview && (
          <div
            style={{
              borderTop: `1px solid ${theme.border}`,
              background: theme.bg,
              padding: "12px 16px",
            }}
          >
            <div style={{ display: "flex", alignItems: "center", gap: 6, marginBottom: 8 }}>
              <Eye size={12} style={{ color: theme.accent }} />
              <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg }}>
                Preview
              </span>
            </div>
            <div style={{ fontSize: 11, color: theme.fgMuted, fontStyle: "italic", textAlign: "center", padding: "12px 0" }}>
              Connect to a running project to preview
            </div>
          </div>
        )}
      </div>

      {/* Part Inspector (right, when inspecting a traced node) */}
      {trace && inspectingNodeId && (
        <PartInspector
          nodeId={inspectingNodeId}
          nodeName={nodeNames.get(inspectingNodeId) ?? inspectingNodeId}
          parts={trace.parts}
        />
      )}

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
