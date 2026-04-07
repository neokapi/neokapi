import { useMemo, useCallback, useState, useEffect, useRef } from "react";
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  Panel,
  useNodesState,
  useEdgesState,
  type NodeTypes,
  type EdgeTypes,
  type Node,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { Play, X, GitBranch, Zap, Eye, ArrowDownUp, ArrowLeftRight } from "lucide-react";
import { DotEdge } from "./edges/DotEdge";

import type { FlowEditorProps, FlowSpec, FlowStep, ToolInfo, ComponentSchema } from "./types";
import { ReaderNode } from "./nodes/ReaderNode";
import { WriterNode } from "./nodes/WriterNode";
import { ToolNode } from "./nodes/ToolNode";
import { ToolPalette } from "./ToolPalette";
import { FlowTemplateLibrary } from "./FlowTemplateLibrary";
import { cn, SchemaForm, Button, Badge, ScrollArea, PanelHeader } from "@neokapi/ui-primitives";
import { stepsToGraph, graphToSteps, type LayoutDirection } from "./conversion";
import { getCategoryStyle } from "./category";
import { suggestParallelGroups, type ParallelSuggestion } from "./parallelChecker";
import { TraceTimeline } from "./TraceTimeline";
import { PartInspector } from "./PartInspector";
import { computeNodeStats } from "./traceTypes";
import type { ToolDoc, ToolDocParam } from "./types";

const nodeTypes: NodeTypes = {
  reader: ReaderNode,
  writer: WriterNode,
  tool: ToolNode,
};

const edgeTypes: EdgeTypes = {
  dot: DotEdge,
};

// ---------------------------------------------------------------------------
// Extracted components
// ---------------------------------------------------------------------------

interface FlowToolbarProps {
  stepCount: number;
  showPreview: boolean;
  onTogglePreview: () => void;
  onRun?: (flow: FlowSpec) => void;
  flow: FlowSpec;
}

function FlowToolbar({ stepCount, showPreview, onTogglePreview, onRun, flow }: FlowToolbarProps) {
  return (
    <PanelHeader
      title={`${stepCount} step${stepCount !== 1 ? "s" : ""}`}
      className="py-1.5"
      actions={
        <>
          <Button
            variant={showPreview ? "outline" : "ghost"}
            size="xs"
            onClick={onTogglePreview}
            className={cn(showPreview && "border-accent text-accent-foreground")}
            aria-label="Toggle preview"
          >
            <Eye size={12} />
            Preview
          </Button>

          {onRun && (
            <Button size="xs" onClick={() => onRun(flow)} aria-label="Run flow">
              <Play size={12} />
              Run
            </Button>
          )}
        </>
      }
    />
  );
}

interface ParallelSuggestionBannerProps {
  suggestion: ParallelSuggestion;
  onParallelize: (suggestion: ParallelSuggestion) => void;
  onDismiss: () => void;
}

function ParallelSuggestionBanner({
  suggestion,
  onParallelize,
  onDismiss,
}: ParallelSuggestionBannerProps) {
  return (
    <PanelHeader
      className="py-1.5 bg-secondary text-[11px]"
      actions={
        <>
          <Button size="xs" onClick={() => onParallelize(suggestion)} className="text-[11px]">
            <GitBranch size={11} />
            Parallelize
          </Button>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onDismiss}
            aria-label="Dismiss suggestion"
          >
            <X size={12} className="text-muted-foreground" />
          </Button>
        </>
      }
    >
      <Zap size={13} className="text-accent-foreground shrink-0" />
      <span className="text-muted-foreground flex-1">
        <strong className="text-foreground">{suggestion.toolNames.join(", ")}</strong> can run in
        parallel &mdash; {suggestion.reason}
      </span>
    </PanelHeader>
  );
}

interface StepConfigPanelProps {
  step: { tool: string };
  toolInfo: ToolInfo | null | undefined;
  schema: ComponentSchema | null | undefined;
  doc: ToolDoc | null | undefined;
  config: Record<string, unknown>;
  onConfigChange: (config: Record<string, unknown>) => void;
  onClose: () => void;
  onRemove?: () => void;
}

function StepConfigPanel({
  step,
  toolInfo,
  schema,
  doc,
  config,
  onConfigChange,
  onClose,
  onRemove,
}: StepConfigPanelProps) {
  const [showDocs, setShowDocs] = useState(false);
  const category = toolInfo?.category || "pipeline";
  const catStyle = getCategoryStyle(category);
  const Icon = catStyle.icon;
  const displayName = toolInfo?.display_name || step.tool;

  // Local config state -- owns the values to prevent parent re-renders from
  // resetting inputs. Syncs to parent via debounced onConfigChange.
  const [localConfig, setLocalConfig] = useState(config);
  const syncTimerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  // Re-initialize when the selected tool changes (not on every config update).
  const toolRef = useRef(step.tool);
  if (step.tool !== toolRef.current) {
    toolRef.current = step.tool;
    setLocalConfig(config);
  }

  const handleLocalChange = useCallback(
    (newConfig: Record<string, unknown>) => {
      setLocalConfig(newConfig);
      clearTimeout(syncTimerRef.current);
      syncTimerRef.current = setTimeout(() => onConfigChange(newConfig), 300);
    },
    [onConfigChange],
  );

  // Flush on unmount.
  useEffect(() => {
    return () => clearTimeout(syncTimerRef.current);
  }, []);

  return (
    <div
      className="flex flex-col border-l border-border bg-background overflow-hidden"
      style={{ width: 280, minWidth: 280, maxWidth: 280 }}
    >
      {/* Header */}
      <div className="px-3 py-2.5 border-b border-border flex flex-col gap-1.5">
        <div className="flex items-center gap-1.5">
          <div className="w-[3px] h-5 rounded-sm shrink-0" style={{ background: catStyle.color }} />
          <div className="flex-1 min-w-0">
            <div className="flex items-center gap-1 mb-0.5">
              <Icon size={11} style={{ color: catStyle.text }} />
              <span
                className="text-[9px] font-bold tracking-wide uppercase"
                style={{ color: catStyle.text }}
              >
                {catStyle.label}
              </span>
            </div>
            <div className="text-sm font-semibold text-foreground">{displayName}</div>
          </div>
          <Button
            variant="ghost"
            size="icon-xs"
            onClick={onClose}
            className="self-start"
            aria-label="Close panel"
          >
            <X size={14} className="text-muted-foreground" />
          </Button>
        </div>

        {/* Description -- prefer doc overview, fall back to ToolInfo.description */}
        {(doc?.overview || toolInfo?.description) && (
          <div
            className={cn(
              "text-[11px] text-muted-foreground leading-relaxed",
              !showDocs && "line-clamp-3",
            )}
          >
            {doc?.overview || toolInfo?.description}
          </div>
        )}

        {/* IO contract info */}
        {(toolInfo?.cardinality ||
          toolInfo?.produces?.length ||
          toolInfo?.side_effects?.length) && (
          <div className="flex gap-1 flex-wrap items-center">
            {toolInfo.cardinality && (
              <span
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px] font-mono font-semibold"
                style={{
                  background:
                    toolInfo.cardinality === "bilingual"
                      ? "oklch(0.55 0.15 250 / 0.1)"
                      : toolInfo.cardinality === "multilingual"
                        ? "oklch(0.55 0.15 320 / 0.1)"
                        : "oklch(0.5 0.02 0 / 0.06)",
                  color:
                    toolInfo.cardinality === "bilingual"
                      ? "oklch(0.55 0.15 250)"
                      : toolInfo.cardinality === "multilingual"
                        ? "oklch(0.55 0.15 320)"
                        : "var(--muted-foreground)",
                }}
              >
                {toolInfo.cardinality}
              </span>
            )}
            {toolInfo.default_locale && (
              <span
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px] font-mono"
                style={{
                  background: "oklch(0.6 0.12 290 / 0.1)",
                  color: "oklch(0.55 0.12 290)",
                }}
              >
                default: {toolInfo.default_locale}
              </span>
            )}
            {toolInfo.side_effects?.map((se) => (
              <span
                key={se}
                className="rounded px-1.5 py-0 h-4 inline-flex items-center text-[9px]"
                style={{
                  background: "oklch(0.65 0.12 85 / 0.1)",
                  color: "oklch(0.55 0.12 85)",
                }}
              >
                ⚡ {se}
              </span>
            ))}
          </div>
        )}

        {/* Requirements badges + docs toggle */}
        <div className="flex gap-1 flex-wrap items-center">
          {toolInfo?.requires?.map((req) => (
            <Badge key={req} variant="secondary" className="text-[9px] px-1.5 py-0 h-4">
              {req}
            </Badge>
          ))}
          {doc && (
            <Button
              variant={showDocs ? "outline" : "ghost"}
              size="xs"
              onClick={() => setShowDocs((v) => !v)}
              className={cn("ml-auto text-[9px] h-5 px-2", showDocs && "border-ring text-ring")}
            >
              {showDocs ? "Hide Docs" : "Docs"}
            </Button>
          )}
          {doc?.wikiUrl && (
            <a
              href={doc.wikiUrl}
              target="_blank"
              rel="noopener noreferrer"
              className="text-[9px] text-muted-foreground no-underline px-1"
              title="Open wiki documentation"
            >
              Wiki ↗
            </a>
          )}
        </div>
      </div>

      {/* Docs panel (collapsible) */}
      {showDocs && doc && (
        <ScrollArea className="max-h-[260px] border-b border-border text-[11px] leading-relaxed">
          <div className="px-3 py-2">
            <DocsSidebar doc={doc} />
          </div>
        </ScrollArea>
      )}

      {/* Config form */}
      <ScrollArea className="flex-1">
        <div className="px-3 py-2">
          {schema ? (
            <SchemaForm
              schema={schema}
              values={localConfig}
              onChange={handleLocalChange}
              compact
              hideHeader
              paramDocs={doc?.parameters}
            />
          ) : (
            <div className="text-[11px] text-muted-foreground text-center py-5 italic">
              {toolInfo?.has_schema ? "Loading configuration..." : "No configurable parameters"}
            </div>
          )}
        </div>
      </ScrollArea>

      {/* Footer */}
      {onRemove && (
        <div className="px-3 py-2 border-t border-border">
          <Button
            variant="destructive"
            size="sm"
            className="w-full"
            onClick={onRemove}
            aria-label="Remove tool from flow"
          >
            Remove from flow
          </Button>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main FlowEditor
// ---------------------------------------------------------------------------

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
  onGetDoc,
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
  const [layoutDirection, setLayoutDirection] = useState<LayoutDirection>("vertical");

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

  // Topology key: only changes when the graph structure changes (tools added/removed/reordered),
  // NOT when config changes. This prevents the graph from resetting on every config edit.
  const topologyKey = useMemo(() => {
    const extractTools = (steps: FlowStep[]): string => {
      return steps
        .map((s) => {
          if (s.parallel && s.parallel.length > 0) {
            return `[${s.parallel.map((p) => p.tool).join(",")}]`;
          }
          return s.tool;
        })
        .join("→");
    };
    return extractTools(flow.steps);
  }, [flow.steps]);

  // eslint-disable-next-line react-hooks/exhaustive-deps -- intentionally keyed on topology, not flow
  const initial = useMemo(
    () => stepsToGraph(flow, toolMap, layoutDirection),
    [topologyKey, toolMap, layoutDirection],
  );

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

  // Ref for remove handler -- breaks circular dependency with enrichedNodes.
  const removeNodeRef = useRef<(nodeId: string) => void>(() => {});

  // React Flow instance — used to fit view after adding tools.
  const reactFlowRef = useRef<ReactFlowInstance | null>(null);

  // Enrich nodes with execution state and remove handler.
  const enrichedNodes = useMemo(() => {
    return initial.nodes.map((n) => {
      const stats = nodeStats?.get(n.id);
      const extra: Record<string, unknown> = {};
      if (stats) {
        extra.execState = stats.hasError
          ? "error"
          : stats.partsProcessed > 0
            ? "complete"
            : undefined;
        extra.partCount = stats.partsProcessed;
      }
      if (!readOnly && n.type === "tool") {
        extra.onRemove = () => removeNodeRef.current(n.id);
      }
      if (Object.keys(extra).length === 0) return n;
      return { ...n, data: { ...n.data, ...extra } };
    });
  }, [initial.nodes, nodeStats, readOnly]);

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

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      setSelectedNodeId(node.id);
      // If we have trace data, also open the part inspector for this node.
      if (trace && node.type === "tool") {
        setInspectingNodeId(node.id);
      }
    },
    [trace],
  );

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
      const newIndex = flow.steps.length;
      const updated: FlowSpec = {
        ...flow,
        steps: [...flow.steps, { tool: toolName }],
      };
      onChange(updated);
      // Auto-select the new tool so the config panel opens immediately.
      setSelectedNodeId(`tool-${newIndex}`);
      // Fit the entire graph into view so the new node is visible.
      requestAnimationFrame(() => {
        reactFlowRef.current?.fitView({ padding: 0.3, duration: 300 });
      });
    },
    [flow, onChange, readOnly],
  );

  const handleRemoveNode = useCallback(
    (nodeId: string) => {
      if (readOnly) return;
      const toolIndex = parseInt(nodeId.replace("tool-", ""), 10);
      if (isNaN(toolIndex)) return;
      const updated: FlowSpec = {
        ...flow,
        steps: flow.steps.filter((_, i) => i !== toolIndex),
      };
      onChange(updated);
      if (selectedNodeId === nodeId) setSelectedNodeId(null);
    },
    [flow, onChange, readOnly, selectedNodeId],
  );
  removeNodeRef.current = handleRemoveNode;

  const handleRemoveSelected = useCallback(() => {
    if (selectedNodeId) handleRemoveNode(selectedNodeId);
  }, [selectedNodeId, handleRemoveNode]);

  // Keyboard shortcut: Delete/Backspace removes selected tool node.
  useEffect(() => {
    if (readOnly) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Delete" || e.key === "Backspace") {
        // Don't intercept if user is typing in an input/textarea.
        const tag = (e.target as HTMLElement)?.tagName;
        if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return;
        if (selectedNodeId?.startsWith("tool-")) {
          e.preventDefault();
          handleRemoveSelected();
        }
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [readOnly, selectedNodeId, handleRemoveSelected]);

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
    const updated = graphToSteps(nodes, layoutDirection);
    onChange(updated);
  }, [nodes, onChange, layoutDirection]);

  // Connection validation -- only allow connecting compatible port types.
  const isValidConnection = useCallback(
    (connection: { source: string | null; target: string | null }) => {
      const sourceNode = nodes.find((n) => n.id === connection.source);
      const targetNode = nodes.find((n) => n.id === connection.target);
      if (!sourceNode || !targetNode) return true;
      const srcOutputs = sourceNode.data.outputs as string[] | undefined;
      const tgtInputs = targetNode.data.inputs as string[] | undefined;
      if (!srcOutputs || !tgtInputs) return true; // no metadata = allow
      return srcOutputs.some((o) => tgtInputs.includes(o));
    },
    [nodes],
  );

  // Config panel state -- resolve from node data, not step index,
  // because node IDs don't map 1:1 to step indices when parallel steps exist.
  const selectedNode = selectedNodeId ? nodes.find((n) => n.id === selectedNodeId) : null;
  const selectedToolName = selectedNode?.data?.toolName as string | undefined;
  const selectedStep = selectedToolName ? findStepByTool(flow.steps, selectedToolName) : null;
  const selectedStepIndex = selectedStep ? findStepIndex(flow.steps, selectedToolName!) : NaN;
  const selectedToolInfo = selectedToolName ? toolMap.get(selectedToolName) : null;
  const selectedSchema = selectedToolName && onGetSchema ? onGetSchema(selectedToolName) : null;
  const selectedDoc = selectedToolName && onGetDoc ? onGetDoc(selectedToolName) : null;

  const handleConfigChange = useCallback(
    (config: Record<string, unknown>) => {
      if (isNaN(selectedStepIndex) || readOnly) return;
      const updated: FlowSpec = {
        ...flow,
        steps: flow.steps.map((s, i) => (i === selectedStepIndex ? { ...s, config } : s)),
      };
      onChange(updated);
    },
    [selectedStepIndex, flow, onChange, readOnly],
  );

  // Template library covers the full editor when shown.
  if (showTemplates) {
    return (
      <div className="flex h-full overflow-auto bg-background">
        <FlowTemplateLibrary
          onSelect={handleSelectTemplate}
          onDismiss={() => setDismissedTemplates(true)}
        />
      </div>
    );
  }

  return (
    <div className="flex h-full overflow-hidden">
      {/* Tool Palette (left) */}
      {!readOnly && <ToolPalette tools={tools} onAddTool={handleAddTool} />}

      {/* Canvas (center) */}
      <div className="flex-1 flex flex-col">
        {/* Toolbar */}
        <FlowToolbar
          stepCount={flow.steps.length}
          showPreview={showPreview}
          onTogglePreview={() => setShowPreview((p) => !p)}
          onRun={onRun}
          flow={flow}
        />

        {/* Parallelization suggestion banner */}
        {suggestions.length > 0 && (
          <ParallelSuggestionBanner
            suggestion={suggestions[0]}
            onParallelize={handleParallelize}
            onDismiss={() => setDismissedSuggestions(true)}
          />
        )}

        {/* Graph canvas */}
        {!showTemplates && (
          <div className="flex-1" onDrop={handleDrop} onDragOver={handleDragOver}>
            <ReactFlow
              nodes={nodes}
              edges={edges}
              onNodesChange={handleNodesChange}
              onEdgesChange={onEdgesChange}
              onNodeClick={handleNodeClick}
              onPaneClick={handlePaneClick}
              onNodeDragStop={handleNodeDragStop}
              onInit={(instance) => {
                reactFlowRef.current = instance;
              }}
              isValidConnection={isValidConnection}
              nodeTypes={nodeTypes}
              edgeTypes={edgeTypes}
              nodesDraggable={!readOnly}
              nodesConnectable={!readOnly}
              fitView
              fitViewOptions={{ padding: 0.3 }}
              proOptions={{ hideAttribution: true }}
              defaultEdgeOptions={{
                style: { stroke: "var(--muted-foreground)", strokeWidth: 2 },
              }}
            >
              <Background
                variant={BackgroundVariant.Dots}
                gap={24}
                size={1}
                color="var(--border)"
              />
              <Panel position="bottom-left">
                <Button
                  variant="outline"
                  size="icon-xs"
                  onClick={() => {
                    setLayoutDirection((d) => (d === "vertical" ? "horizontal" : "vertical"));
                  }}
                  title={
                    layoutDirection === "vertical"
                      ? "Switch to horizontal layout"
                      : "Switch to vertical layout"
                  }
                >
                  {layoutDirection === "vertical" ? (
                    <ArrowLeftRight size={12} />
                  ) : (
                    <ArrowDownUp size={12} />
                  )}
                </Button>
              </Panel>
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
          <div className="border-t border-border bg-background px-4 py-3">
            <div className="flex items-center gap-1.5 mb-2">
              <Eye size={12} className="text-accent-foreground" />
              <span className="text-[11px] font-semibold text-foreground">Preview</span>
            </div>
            <div className="text-[11px] text-muted-foreground italic text-center py-3">
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
        <StepConfigPanel
          step={selectedStep}
          toolInfo={selectedToolInfo}
          schema={selectedSchema}
          doc={selectedDoc}
          config={selectedStep.config || {}}
          onConfigChange={handleConfigChange}
          onClose={() => setSelectedNodeId(null)}
          onRemove={readOnly ? undefined : handleRemoveSelected}
        />
      )}
    </div>
  );
}

/** Find a step by tool name, searching into parallel branches. */
function findStepByTool(steps: FlowStep[], toolName: string): FlowStep | null {
  for (const s of steps) {
    if (s.tool === toolName) return s;
    if (s.parallel) {
      for (const p of s.parallel) {
        if (p.tool === toolName) return p;
      }
    }
  }
  return null;
}

/** Find the flat index of a step (or its parent parallel group) by tool name. */
function findStepIndex(steps: FlowStep[], toolName: string): number {
  for (let i = 0; i < steps.length; i++) {
    if (steps[i].tool === toolName) return i;
    if (steps[i].parallel) {
      for (const p of steps[i].parallel!) {
        if (p.tool === toolName) return i;
      }
    }
  }
  return NaN;
}

// ---------------------------------------------------------------------------
// Inline documentation sidebar for the config panel
// ---------------------------------------------------------------------------

function DocsSidebar({ doc }: { doc: ToolDoc }) {
  const params = doc.parameters ? Object.entries(doc.parameters) : [];
  const hasExamples = doc.examples && doc.examples.length > 0;
  const hasLimitations = doc.limitations && doc.limitations.length > 0;
  const hasNotes = doc.processingNotes && doc.processingNotes.length > 0;

  return (
    <div className="flex flex-col gap-2.5">
      {/* Parameters */}
      {params.length > 0 && (
        <DocSection title="Parameters">
          <div className="flex flex-col gap-1.5">
            {params.map(([key, p]) => (
              <DocParamRow key={key} name={key} param={p} />
            ))}
          </div>
        </DocSection>
      )}

      {/* Examples */}
      {hasExamples && (
        <DocSection title="Examples">
          {doc.examples!.map((ex, i) => (
            <div
              key={i}
              className={cn(
                "px-2 py-1.5 rounded bg-secondary",
                i < doc.examples!.length - 1 && "mb-1",
              )}
            >
              <div className="font-semibold text-[10px] text-foreground">{ex.title}</div>
              {ex.description && (
                <div className="text-[10px] text-muted-foreground mt-0.5">{ex.description}</div>
              )}
              {ex.input && (
                <pre className="text-[9px] font-mono bg-background rounded-sm px-1.5 py-1 mt-1 overflow-auto max-h-[60px] whitespace-pre-wrap text-foreground">
                  {ex.input}
                </pre>
              )}
            </div>
          ))}
        </DocSection>
      )}

      {/* Limitations */}
      {hasLimitations && (
        <DocSection title="Limitations">
          {doc.limitations!.map((lim, i) => (
            <div
              key={i}
              className="text-[10px] text-muted-foreground pl-2 mb-0.5"
              style={{ borderLeft: "2px solid color-mix(in oklch, var(--ring) 30%, transparent)" }}
            >
              {lim}
            </div>
          ))}
        </DocSection>
      )}

      {/* Processing Notes */}
      {hasNotes && (
        <DocSection title="Notes">
          {doc.processingNotes!.map((note, i) => (
            <div
              key={i}
              className="text-[10px] text-muted-foreground pl-2 mb-0.5"
              style={{
                borderLeft: "2px solid color-mix(in oklch, var(--accent) 40%, transparent)",
              }}
            >
              {note}
            </div>
          ))}
        </DocSection>
      )}
    </div>
  );
}

function DocSection({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div>
      <div className="text-[9px] font-bold uppercase tracking-wide text-muted-foreground mb-1">
        {title}
      </div>
      {children}
    </div>
  );
}

function DocParamRow({ name, param }: { name: string; param: ToolDocParam }) {
  return (
    <div className="px-2 py-1 rounded bg-secondary">
      <div className="flex items-center gap-1 mb-0.5">
        <code
          className="text-[10px] font-semibold px-1.5 py-px rounded-sm"
          style={{
            color: "var(--ring)",
            background: "color-mix(in oklch, var(--ring) 8%, transparent)",
          }}
        >
          {name}
        </code>
        {param.introducedIn && (
          <span
            className="text-[8px] px-1 py-px rounded-sm text-muted-foreground font-medium"
            style={{ background: "color-mix(in oklch, var(--accent) 20%, transparent)" }}
          >
            {param.introducedIn}
          </span>
        )}
      </div>
      <div className="text-[10px] text-muted-foreground leading-snug">{param.description}</div>
      {param.notes?.map((note, i) => (
        <div key={i} className="text-[9px] text-muted-foreground mt-0.5 italic opacity-80">
          {note}
        </div>
      ))}
      {param.dependsOn?.map((dep, i) => (
        <div key={i} className="text-[9px] mt-0.5 flex items-center gap-0.5">
          <GitBranch size={8} className="text-muted-foreground" />
          <code className="font-semibold text-muted-foreground">{dep.property}</code>
          <span className="text-muted-foreground opacity-70">{dep.condition}</span>
        </div>
      ))}
    </div>
  );
}
