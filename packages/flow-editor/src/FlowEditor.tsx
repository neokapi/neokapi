import { useMemo, useCallback, useState, useEffect, useRef } from "react";
import {
  ReactFlow,
  Background,
  BackgroundVariant,
  Panel,
  MarkerType,
  useNodesState,
  useEdgesState,
  type NodeTypes,
  type EdgeTypes,
  type Node,
  type Edge,
  type ReactFlowInstance,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import {
  Play,
  X,
  GitBranch,
  Zap,
  Eye,
  ArrowDownUp,
  ArrowLeftRight,
  Loader2,
  Layers,
} from "lucide-react";
import { DotEdge } from "./edges/DotEdge";

import type {
  FlowEditorProps,
  FlowSpec,
  FlowStep,
  FlowBinding,
  ToolInfo,
  ComponentSchema,
  IOPort,
} from "./types";
import { parseBinding, formatBinding } from "./defAdapter";
import { ToolNode } from "./nodes/ToolNode";
import { EndpointNode } from "./nodes/EndpointNode";
import { ToolPalette } from "./ToolPalette";
import { FlowTemplateLibrary } from "./FlowTemplateLibrary";
import { FlowLegend } from "./FlowLegend";
import { cn, SchemaForm, Button, Badge, ScrollArea, PanelHeader } from "@neokapi/ui-primitives";
import { stepsToGraph, graphToSteps, type LayoutDirection } from "./conversion";
import {
  resolveStepLocation,
  stepAtLocation,
  updateStepAtLocation,
  removeStepAtLocation,
  type NodeStepData,
} from "./stepResolve";
import { createDebouncedSync, type DebouncedSync } from "./debouncedSync";
import { getCategoryStyle } from "./category";
import { suggestParallelGroups, type ParallelSuggestion } from "./parallelChecker";
import { TraceTimeline } from "./TraceTimeline";
import { PartInspector } from "./PartInspector";
import { computeNodeStats } from "./traceTypes";
import type { ToolDoc, ToolDocParam } from "./types";

const nodeTypes: NodeTypes = {
  tool: ToolNode,
  source: EndpointNode,
  sink: EndpointNode,
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
  runDisabled?: boolean;
  flow: FlowSpec;
}

function FlowToolbar({
  stepCount,
  showPreview,
  onTogglePreview,
  onRun,
  runDisabled,
  flow,
}: FlowToolbarProps) {
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
            <Button
              size="xs"
              onClick={() => onRun(flow)}
              disabled={runDisabled}
              aria-label="Run flow"
            >
              {runDisabled ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              {runDisabled ? "Running..." : "Run"}
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

/** Accent color for source-transform stage — matches ToolNode constant. */
const SOURCE_TRANSFORM_PANEL_COLOR = "oklch(0.68 0.16 250)";
const SOURCE_TRANSFORM_PANEL_BG = "oklch(0.68 0.16 250 / 0.10)";

interface StepConfigPanelProps {
  step: { tool: string };
  toolInfo: ToolInfo | null | undefined;
  schema: ComponentSchema | null | undefined;
  doc: ToolDoc | null | undefined;
  config: Record<string, unknown>;
  /** Whether this step currently runs in the source-transform stage. */
  isSourceTransformStage: boolean;
  onConfigChange: (config: Record<string, unknown>) => void;
  onStageToggle: (asSourceTransform: boolean) => void;
  onClose: () => void;
  onRemove?: () => void;
}

// Exported for colocated tests (not re-exported from the package index).
export function StepConfigPanel({
  step,
  toolInfo,
  schema,
  doc,
  config,
  isSourceTransformStage,
  onConfigChange,
  onStageToggle,
  onClose,
  onRemove,
}: StepConfigPanelProps) {
  const [showDocs, setShowDocs] = useState(false);
  const category = toolInfo?.category || "pipeline";
  const catStyle = getCategoryStyle(category);
  const Icon = catStyle.icon;
  const displayName = toolInfo?.display_name || step.tool;
  const canBeSourceTransform = !!toolInfo?.isSourceTransform;

  // Local config state -- owns the values to prevent parent re-renders from
  // resetting inputs. Syncs to parent via a debounced controller.
  const [localConfig, setLocalConfig] = useState(config);

  // Keep the controller's emit target pointing at the latest onConfigChange
  // without re-creating the controller (which would drop a pending timer).
  const onConfigChangeRef = useRef(onConfigChange);
  onConfigChangeRef.current = onConfigChange;
  const syncRef = useRef<DebouncedSync<Record<string, unknown>>>(undefined);
  if (!syncRef.current) {
    syncRef.current = createDebouncedSync((cfg) => onConfigChangeRef.current(cfg), 300);
  }

  // Re-initialize when the selected tool changes (not on every config update).
  // (The panel is also keyed on the selected node id, so it normally remounts.)
  const toolRef = useRef(step.tool);
  if (step.tool !== toolRef.current) {
    toolRef.current = step.tool;
    setLocalConfig(config);
  }

  const handleLocalChange = useCallback((newConfig: Record<string, unknown>) => {
    setLocalConfig(newConfig);
    syncRef.current!.schedule(newConfig);
  }, []);

  // Flush any pending debounced edit on unmount / close so the last sub-300ms
  // edit is not dropped. With `key={selectedNodeId}` on the panel, switching
  // selection remounts it, which flushes the previous selection's pending edit.
  useEffect(() => {
    const sync = syncRef.current!;
    return () => sync.flush();
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

      {/* Source-transform stage toggle */}
      <div className="px-3 py-2 border-t border-border">
        <div className="flex items-center gap-2">
          <button
            type="button"
            role="checkbox"
            aria-checked={isSourceTransformStage}
            disabled={!canBeSourceTransform}
            onClick={() => canBeSourceTransform && onStageToggle(!isSourceTransformStage)}
            className={cn(
              "relative inline-flex h-4 w-7 shrink-0 cursor-pointer items-center rounded-full transition-colors duration-150",
              isSourceTransformStage
                ? "border border-[oklch(0.68_0.16_250)]"
                : "border border-border bg-muted",
              !canBeSourceTransform && "opacity-40 cursor-not-allowed",
            )}
            style={
              isSourceTransformStage
                ? {
                    background: SOURCE_TRANSFORM_PANEL_BG,
                    borderColor: SOURCE_TRANSFORM_PANEL_COLOR,
                  }
                : undefined
            }
            title={
              canBeSourceTransform
                ? "Toggle: run this tool in the source-transform stage (before main steps)"
                : "This tool can't rewrite source"
            }
          >
            <span
              className="pointer-events-none inline-block h-2.5 w-2.5 rounded-full bg-current shadow transition-transform duration-150"
              style={{
                transform: isSourceTransformStage ? "translateX(14px)" : "translateX(1px)",
                color: isSourceTransformStage
                  ? SOURCE_TRANSFORM_PANEL_COLOR
                  : "var(--muted-foreground)",
              }}
            />
          </button>
          <div className="flex-1 min-w-0">
            <div
              className="text-[10px] font-semibold leading-none flex items-center gap-1"
              style={{ color: isSourceTransformStage ? SOURCE_TRANSFORM_PANEL_COLOR : undefined }}
            >
              <Layers size={9} />
              Source transform
            </div>
            <div className="text-[9px] text-muted-foreground mt-0.5">
              {canBeSourceTransform
                ? "Settles the model before main tools run"
                : "This tool can't rewrite source"}
            </div>
          </div>
        </div>
      </div>

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
  runDisabled,
  onGetSchema,
  onGetDoc,
  readOnly = false,
  traceEvents,
  trace,
}: FlowEditorProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [dismissedSuggestions, setDismissedSuggestions] = useState(false);
  const [dismissedTemplates, setDismissedTemplates] = useState(false);

  const showTemplates =
    !readOnly &&
    !dismissedTemplates &&
    flow.steps.length === 0 &&
    (flow.sourceTransforms?.length ?? 0) === 0;
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
    const stPart = (flow.sourceTransforms ?? []).map((s) => `ST:${s.tool}`).join("→");
    const mainPart = extractTools(flow.steps);
    return stPart ? `${stPart}|${mainPart}` : mainPart;
  }, [flow.steps, flow.sourceTransforms]);

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

  const handleSourceChange = useCallback(
    (binding: FlowBinding) => {
      if (readOnly) return;
      const locator = formatBinding(binding);
      const next: FlowSpec = { ...flow };
      if (locator) next.source = locator;
      else delete next.source;
      onChange(next);
    },
    [flow, onChange, readOnly],
  );

  const handleSinkChange = useCallback(
    (binding: FlowBinding) => {
      if (readOnly) return;
      const locator = formatBinding(binding);
      const next: FlowSpec = { ...flow };
      if (locator) next.sink = locator;
      else delete next.sink;
      onChange(next);
    },
    [flow, onChange, readOnly],
  );

  // Inject Source/Sink as graph nodes wired to the chain's roots/leaves, so the
  // pipeline renders as one continuous flow (graphToSteps ignores non-"tool"
  // nodes, so these never round-trip into steps).
  const { displayNodes, displayEdges } = useMemo(() => {
    const toolNodes = initial.nodes;
    if (toolNodes.length === 0) {
      return { displayNodes: enrichedNodes, displayEdges: initial.edges };
    }
    const isVertical = layoutDirection === "vertical";
    const primary = (n: Node) => (isVertical ? n.position.y : n.position.x);
    const cross = (n: Node) => (isVertical ? n.position.x : n.position.y);
    const targets = new Set(initial.edges.map((e) => e.target));
    const sources = new Set(initial.edges.map((e) => e.source));
    const roots = toolNodes.filter((n) => !targets.has(n.id));
    const leaves = toolNodes.filter((n) => !sources.has(n.id));

    const minPrimary = Math.min(...toolNodes.map(primary));
    const maxPrimary = Math.max(...toolNodes.map(primary));
    const crossCenter = roots.length ? cross(roots[0]) : 200;

    const srcPos = isVertical
      ? { x: crossCenter, y: minPrimary - 96 }
      : { x: minPrimary - 220, y: crossCenter };
    const sinkPos = isVertical
      ? { x: crossCenter, y: maxPrimary + 132 }
      : { x: maxPrimary + 240, y: crossCenter };

    const endpointData = (role: "source" | "sink", binding?: string) => ({
      role,
      binding: parseBinding(binding),
      readOnly,
      layoutDirection,
      onBindingChange: role === "source" ? handleSourceChange : handleSinkChange,
    });

    const sourceNode: Node = {
      id: "endpoint-source",
      type: "source",
      position: srcPos,
      data: endpointData("source", flow.source),
      draggable: false,
      deletable: false,
      selectable: false,
    };
    const sinkNode: Node = {
      id: "endpoint-sink",
      type: "sink",
      position: sinkPos,
      data: endpointData("sink", flow.sink),
      draggable: false,
      deletable: false,
      selectable: false,
    };

    const endpointEdges: Edge[] = [
      ...roots.map((r) => ({
        id: `e-source-${r.id}`,
        source: "endpoint-source",
        target: r.id,
        type: "dot",
        markerEnd: {
          type: MarkerType.Arrow,
          width: 16,
          height: 16,
          color: "var(--muted-foreground)",
        },
      })),
      ...leaves.map((l) => ({
        id: `e-${l.id}-sink`,
        source: l.id,
        target: "endpoint-sink",
        type: "dot",
        markerEnd: {
          type: MarkerType.Arrow,
          width: 16,
          height: 16,
          color: "var(--muted-foreground)",
        },
      })),
    ];

    return {
      displayNodes: [...enrichedNodes, sourceNode, sinkNode],
      displayEdges: [...initial.edges, ...endpointEdges],
    };
  }, [
    enrichedNodes,
    initial.nodes,
    initial.edges,
    layoutDirection,
    readOnly,
    flow.source,
    flow.sink,
    handleSourceChange,
    handleSinkChange,
  ]);

  const [nodes, setNodes, onNodesChange] = useNodesState(displayNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(displayEdges);

  // Sync graph when flow prop changes (e.g., tool added, template selected).
  useEffect(() => {
    setNodes(displayNodes);
    setEdges(displayEdges);
  }, [displayNodes, displayEdges, setNodes, setEdges]);

  const handleNodesChange = useCallback(
    (changes: Parameters<typeof onNodesChange>[0]) => {
      onNodesChange(changes);
    },
    [onNodesChange],
  );

  const handleNodeClick = useCallback(
    (_: React.MouseEvent, node: Node) => {
      // Source/Sink are bindings, not steps — they have their own dropdown UI
      // and never open the tool config panel.
      if (node.type !== "tool") return;
      setSelectedNodeId(node.id);
      // If we have trace data, also open the part inspector for this node.
      if (trace) {
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
      // Node IDs are assigned globally across sourceTransforms + steps, so the
      // new main-stage node gets index = stCount + steps.length.
      const stCount = flow.sourceTransforms?.length ?? 0;
      const newNodeIndex = stCount + flow.steps.length;
      const updated: FlowSpec = {
        ...flow,
        steps: [...flow.steps, { tool: toolName }],
      };
      onChange(updated);
      // Auto-select the new tool so the config panel opens immediately.
      setSelectedNodeId(`tool-${newNodeIndex}`);
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
      // Resolve the node by IDENTITY (its position in the FlowSpec, carried on
      // node.data) so deleting one node never drops siblings that share the
      // same tool name, and removing a parallel branch keeps the others.
      const node = nodes.find((n) => n.id === nodeId);
      const loc = resolveStepLocation(node?.data as NodeStepData | undefined);
      if (!loc) return;

      onChange(removeStepAtLocation(flow, loc));
      if (selectedNodeId === nodeId) setSelectedNodeId(null);
    },
    [flow, nodes, onChange, readOnly, selectedNodeId],
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

  // Sync graph changes back to steps format on drag end. The source/sink
  // bindings are not nodes, so carry them through unchanged.
  const handleNodeDragStop = useCallback(() => {
    const updated = graphToSteps(nodes, layoutDirection, {
      source: flow.source,
      sink: flow.sink,
    });
    onChange(updated);
  }, [nodes, onChange, layoutDirection, flow.source, flow.sink]);

  // Update a source/sink binding (from the endpoint pickers). The pickers work
  // in the internal FlowBinding object; serialize it to the wire-format string
  // locator (`file` → omitted, the default). Spreading then deleting keeps the
  // file default out of the spec entirely.
  // Connection validation -- only allow connecting compatible port types.
  const isValidConnection = useCallback(
    (connection: { source: string | null; target: string | null }) => {
      const sourceNode = nodes.find((n) => n.id === connection.source);
      const targetNode = nodes.find((n) => n.id === connection.target);
      if (!sourceNode || !targetNode) return true;
      // A connection is meaningful when the source produces at least one port
      // the target consumes (matched by type@side). Missing metadata or a
      // target that consumes nothing (a pass-through) is permitted.
      const srcProduces = sourceNode.data.produces as IOPort[] | undefined;
      const tgtConsumes = targetNode.data.consumes as IOPort[] | undefined;
      if (!srcProduces || !tgtConsumes || tgtConsumes.length === 0) return true;
      const produced = new Set(srcProduces.map((f) => `${f.type}@${f.side ?? "source"}`));
      return tgtConsumes.some((c) => produced.has(`${c.type}@${c.side ?? "source"}`));
    },
    [nodes],
  );

  // Config panel state -- resolve the selected node by IDENTITY (its position in
  // the FlowSpec, carried on node.data), never by tool name. This keeps
  // duplicate-tool nodes and parallel branches distinct.
  const selectedNode = selectedNodeId ? nodes.find((n) => n.id === selectedNodeId) : null;
  const selectedToolName = selectedNode?.data?.toolName as string | undefined;
  const selectedIsSTStage = selectedNode?.data?.stage === "source-transform";

  const selectedLocation = useMemo(
    () => resolveStepLocation(selectedNode?.data as NodeStepData | undefined),
    [selectedNode],
  );

  const selectedStep = useMemo(
    () => (selectedLocation ? stepAtLocation(flow, selectedLocation) : null),
    [selectedLocation, flow],
  );

  const selectedToolInfo = selectedToolName ? toolMap.get(selectedToolName) : null;
  const selectedSchema = selectedToolName && onGetSchema ? onGetSchema(selectedToolName) : null;
  const selectedDoc = selectedToolName && onGetDoc ? onGetDoc(selectedToolName) : null;

  const handleConfigChange = useCallback(
    (config: Record<string, unknown>) => {
      if (readOnly || !selectedLocation) return;
      onChange(updateStepAtLocation(flow, selectedLocation, (s) => ({ ...s, config })));
    },
    [selectedLocation, flow, onChange, readOnly],
  );

  // Toggle a tool between the source-transform stage and the main stage,
  // resolving the exact step by identity (not tool name).
  const handleStageToggle = useCallback(
    (asSourceTransform: boolean) => {
      if (readOnly || !selectedLocation) return;
      const step = stepAtLocation(flow, selectedLocation);
      if (!step) return;
      // Parallel branches can't move stages (the toggle is disabled for them).
      if (selectedLocation.branchIndex !== undefined) return;

      if (asSourceTransform && !selectedLocation.isSourceTransform) {
        // Move from steps → sourceTransforms
        const updated: FlowSpec = {
          ...flow,
          sourceTransforms: [...(flow.sourceTransforms ?? []), { ...step }],
          steps: flow.steps.filter((_, i) => i !== selectedLocation.index),
        };
        onChange(updated);
        setSelectedNodeId(null);
      } else if (!asSourceTransform && selectedLocation.isSourceTransform) {
        // Move from sourceTransforms → steps
        const newST = (flow.sourceTransforms ?? []).filter((_, i) => i !== selectedLocation.index);
        const updated: FlowSpec = {
          ...flow,
          sourceTransforms: newST.length > 0 ? newST : undefined,
          steps: [{ ...step }, ...flow.steps],
        };
        onChange(updated);
        setSelectedNodeId(null);
      }
    },
    [readOnly, selectedLocation, flow, onChange],
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
          stepCount={(flow.sourceTransforms?.length ?? 0) + flow.steps.length}
          showPreview={showPreview}
          onTogglePreview={() => setShowPreview((p) => !p)}
          onRun={onRun}
          runDisabled={runDisabled}
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

              {/* Source / Sink are now graph nodes (see the endpoint injection
                  above), connected to the chain's roots/leaves by edges. */}

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

              <Panel position="top-right">
                <FlowLegend />
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
          key={selectedNodeId}
          step={selectedStep}
          toolInfo={selectedToolInfo}
          schema={selectedSchema}
          doc={selectedDoc}
          config={selectedStep.config || {}}
          isSourceTransformStage={selectedIsSTStage}
          onConfigChange={handleConfigChange}
          onStageToggle={handleStageToggle}
          onClose={() => setSelectedNodeId(null)}
          onRemove={readOnly ? undefined : handleRemoveSelected}
        />
      )}
    </div>
  );
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
