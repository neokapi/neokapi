import { useMemo, useCallback, useState, useEffect, useRef } from "react";
import { t } from "@neokapi/kapi-react/runtime";
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
import "./flowEditor.css";
import { Play, Plus, X, GitBranch, Zap, Loader2, Lock } from "lucide-react";
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
import { ParallelGroupNode } from "./nodes/ParallelGroupNode";
import { IoContract } from "./nodes/PortChip";
import { ToolPalette } from "./ToolPalette";
import { FlowTemplateLibrary } from "./FlowTemplateLibrary";
import { FlowLegend } from "./FlowLegend";
import {
  cn,
  SchemaForm,
  Button,
  Badge,
  ScrollArea,
  PanelHeader,
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@neokapi/ui-primitives";
import {
  serpentineGraph,
  graphToSteps,
  centerAlignRows,
  estimateNodeHeight,
  SERP_COL_W,
  SERP_ROW_H,
  type EndpointGeom,
  type ParallelBranch,
} from "./conversion";
import {
  resolveStepLocation,
  stepAtLocation,
  updateStepAtLocation,
  removeStepAtLocation,
  type NodeStepData,
} from "./stepResolve";
import { createDebouncedSync, type DebouncedSync } from "./debouncedSync";
import { computeUnmet } from "./ioGraph";
import { computePlacement, type PlacementDiagnostic } from "./placement";
import { hasRedactionWrap, wrapWithRedaction, unwrapRedaction } from "./redactionWrap";
import { getCategoryStyle } from "./category";
import { suggestParallelGroups, type ParallelSuggestion } from "./parallelChecker";
import { TracePanel } from "./TracePanel";
import { RunInspectorPanel } from "./RunInspectorPanel";
import { EndpointInspectorPanel } from "./EndpointInspectorPanel";
import { computeNodeStats } from "./traceTypes";
// (PartInspector and TraceTimeline remain exported from the package for hosts
// that render trace data outside the editor; in-editor, the run data lives ON
// the graph — node duration/part badges, edge traversal counts — plus the
// RunInspectorPanel.)
import {
  remapEventsToEditor,
  activeEditorNodes,
  stepToolCounts,
  nodeSpans,
  edgeTransits,
} from "./traceSelectors";
import type { ToolDoc, ToolDocParam } from "./types";

const nodeTypes: NodeTypes = {
  tool: ToolNode,
  source: EndpointNode,
  sink: EndpointNode,
  parallel: ParallelGroupNode,
};

/** Top-left margin for the content within the canvas (px, at 100% zoom). */
const CANVAS_MARGIN = 80;

const edgeTypes: EdgeTypes = {
  dot: DotEdge,
};

// ---------------------------------------------------------------------------
// Extracted components
// ---------------------------------------------------------------------------

interface FlowToolbarProps {
  stepCount: number;
  onRun?: (flow: FlowSpec) => void;
  runDisabled?: boolean;
  flow: FlowSpec;
  /** Whether the flow currently has the redaction wrap (redact … unredact). */
  redacted?: boolean;
  /** Toggle the redaction wrap; absent in read-only flows. */
  onToggleRedaction?: () => void;
}

function FlowToolbar({
  stepCount,
  onRun,
  runDisabled,
  flow,
  redacted,
  onToggleRedaction,
}: FlowToolbarProps) {
  return (
    <PanelHeader
      title={`${stepCount} step${stepCount !== 1 ? "s" : ""}`}
      className="py-1.5"
      actions={
        <>
          {onToggleRedaction && (
            <Button
              variant={redacted ? "outline" : "ghost"}
              size="xs"
              onClick={onToggleRedaction}
              className={cn(redacted && "border-[oklch(0.6_0.2_15)] text-[oklch(0.6_0.2_15)]")}
              title={
                redacted
                  ? "Remove redaction: stop protecting sensitive content"
                  : "Protect sensitive content: wrap the flow with redact … unredact"
              }
            >
              <Lock size={12} />
              {redacted ? t("Protected") : t("Protect")}
            </Button>
          )}

          {onRun && (
            <Button
              size="xs"
              onClick={() => onRun(flow)}
              disabled={runDisabled}
              aria-label="Run flow"
            >
              {runDisabled ? <Loader2 size={12} className="animate-spin" /> : <Play size={12} />}
              {runDisabled ? t("Running...") : t("Run")}
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
  /**
   * Project-level preset for this tool (the recipe's defaults.tools entry).
   * The engine merges it under the step's own config — the step wins per key —
   * so the panel shows the inherited values and flags the overridden ones.
   */
  preset?: Record<string, unknown>;
  /** Required input ports nothing upstream produces (requirement analysis). */
  unmet?: string[];
  /** Transformer placement diagnostics for this step (AD-006 placement pass). */
  placement?: PlacementDiagnostic[];
  onConfigChange: (config: Record<string, unknown>) => void;
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
  preset,
  unmet,
  placement,
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

        {/* Typed IO contract: what this tool reads → writes */}
        {((toolInfo?.consumes && toolInfo.consumes.length > 0) ||
          (toolInfo?.produces && toolInfo.produces.length > 0)) && (
          <div className="flex items-center gap-1.5">
            <span className="text-[9px] uppercase tracking-wide text-muted-foreground">IO</span>
            <IoContract
              consumes={toolInfo?.consumes}
              produces={toolInfo?.produces}
              max={8}
              showLabels
            />
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
              {showDocs ? t("Hide Docs") : t("Docs")}
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

        {/* Unmet-requirement guidance: a required input nothing upstream produces. */}
        {unmet && unmet.length > 0 && (
          <div
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={{
              borderColor: "oklch(0.62 0.17 45 / 0.5)",
              background: "oklch(0.62 0.17 45 / 0.08)",
              color: "oklch(0.5 0.15 45)",
            }}
          >
            <span className="font-semibold">Missing input: </span>
            this tool needs <span className="font-mono">{unmet.join(", ")}</span>, but nothing
            earlier in the flow produces {unmet.length > 1 ? "them" : "it"}. Add a tool that
            produces {unmet.length > 1 ? "these" : "it"} before this step.
          </div>
        )}

        {/* Project preset (defaults.tools): inherited defaults the engine
            merges under this step's config — the step wins per key. */}
        {preset && Object.keys(preset).length > 0 && (
          <div
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={{
              borderColor: "oklch(0.6 0.12 290 / 0.5)",
              background: "oklch(0.6 0.12 290 / 0.08)",
              color: "oklch(0.5 0.12 290)",
            }}
          >
            <span className="font-semibold">Project preset: </span>
            this tool inherits defaults from the project recipe (
            <span className="font-mono">defaults.tools</span>). Values set here override them per
            key.
            <div className="mt-1 flex flex-col gap-0.5">
              {Object.entries(preset).map(([k, v]) => {
                const overridden = config[k] !== undefined;
                return (
                  <div
                    key={k}
                    className={cn("font-mono text-[9px]", overridden && "line-through opacity-60")}
                  >
                    {k}: {typeof v === "string" ? v : JSON.stringify(v)}
                    {overridden && (
                      <span className="ml-1 not-italic no-underline">(overridden)</span>
                    )}
                  </div>
                );
              })}
            </div>
          </div>
        )}

        {/* Transformer placement diagnostics (AD-006): the same errors the build
            gate rejects with, surfaced while composing. */}
        {placement?.map((d, i) => (
          <div
            key={`${d.rule}-${i}`}
            className="mt-2 rounded-md border px-2 py-1.5 text-[10px] leading-snug"
            style={
              d.severity === "error"
                ? {
                    borderColor: "oklch(0.55 0.2 25 / 0.5)",
                    background: "oklch(0.55 0.2 25 / 0.08)",
                    color: "oklch(0.45 0.18 25)",
                  }
                : {
                    borderColor: "oklch(0.62 0.17 45 / 0.5)",
                    background: "oklch(0.62 0.17 45 / 0.08)",
                    color: "oklch(0.5 0.15 45)",
                  }
            }
          >
            <span className="font-semibold">
              {d.severity === "error" ? t("Placement error: ") : t("Placement: ")}
            </span>
            {d.message}
          </div>
        ))}
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
              {toolInfo?.has_schema
                ? t("Loading configuration...")
                : t("No configurable parameters")}
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
  runDisabled,
  onGetSchema,
  onGetDoc,
  readOnly = false,
  traceEvents,
  trace,
  onTraceDismiss,
  projectPresets,
  renderEndpointPanel,
  focusRequest,
  renderStepConfigPanel,
}: FlowEditorProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [dismissedSuggestions, setDismissedSuggestions] = useState(false);
  const [dismissedTemplates, setDismissedTemplates] = useState(false);

  const showTemplates = !readOnly && !dismissedTemplates && flow.steps.length === 0;
  const [addOpen, setAddOpen] = useState(false);

  // Run review: the trace from running THIS flow plays back on the canvas.
  // The cursor windows the (editor-remapped) events; a selected node shows the
  // run inspector by default, flippable to the config panel. Play state lives
  // here (the transport is controlled) so edges animate only while playing.
  const [traceCursor, setTraceCursor] = useState<number | null>(null);
  const [tracePlaying, setTracePlaying] = useState(false);
  const [panelMode, setPanelMode] = useState<"inspect" | "configure">("inspect");
  const [traceDismissed, setTraceDismissed] = useState(false);

  // The editor-remapped event stream for the loaded trace (trace node ids →
  // `tool-<stepIndex>`), or the host-supplied pre-mapped events.
  const runEvents = useMemo(() => {
    if (trace && !traceDismissed) return remapEventsToEditor(trace, stepToolCounts(flow.steps));
    return traceEvents ?? null;
  }, [trace, traceDismissed, traceEvents, flow.steps]);

  // A fresh trace starts fully played (the run just happened); scrubbing
  // rewinds it. Editing the flow invalidates the review.
  useEffect(() => {
    setTraceCursor(trace ? null : null);
    setTracePlaying(false);
    setTraceDismissed(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [trace]);

  // Host-driven focus (lesson steps): apply once per nonce — select the
  // requested node/endpoint (opening its panel) or clear the selection.
  useEffect(() => {
    if (!focusRequest) return;
    setSelectedNodeId(focusRequest.select);
    setPanelMode(focusRequest.mode ?? "inspect");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusRequest?.nonce]);
  const cursor = traceCursor ?? runEvents?.length ?? 0;

  // Measured node heights, by id, used to center-align each row (stations differ
  // in height — a parallel group is taller than a tool node — so the connecting
  // edges only run straight once every node in a row shares a centerline).
  // Unknown on first paint (nodes render top-aligned, seeded by estimate), then
  // captured from React Flow's measurements and fed back via heightVersion.
  const heightCache = useRef<Map<string, number>>(new Map());
  const [heightVersion, setHeightVersion] = useState(0);

  // Canvas width drives how many columns the serpentine layout wraps at.
  const canvasRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(4);
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const measure = () => {
      const w = el.clientWidth;
      // Fit as many columns as the width allows (content is anchored top-left
      // with CANVAS_MARGIN), so short flows stay on one row.
      if (w > 0) setColumns(Math.max(1, Math.floor((w - CANVAS_MARGIN) / SERP_COL_W)));
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

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
    () => serpentineGraph(flow, toolMap, columns),
    [topologyKey, toolMap, columns],
  );

  // Per-node trace stats for the execution overlay, windowed to the playback
  // cursor so scrubbing replays the run on the canvas.
  const windowedEvents = useMemo(
    () => (runEvents ? runEvents.slice(0, cursor) : null),
    [runEvents, cursor],
  );
  const nodeStats = useMemo(
    () => (windowedEvents ? computeNodeStats(windowedEvents) : null),
    [windowedEvents],
  );
  // Nodes with parts in flight at the cursor (entered, not yet exited).
  const activeNodes = useMemo(
    () => (runEvents ? activeEditorNodes(runEvents, cursor) : null),
    [runEvents, cursor],
  );
  // Per-node wall-clock span (first enter → last exit) at the cursor, shown as
  // the node's duration badge.
  const spans = useMemo(
    () => (runEvents ? nodeSpans(runEvents, cursor) : null),
    [runEvents, cursor],
  );

  // Refs for per-node handlers -- break the circular dependency with enrichedNodes.
  const removeNodeRef = useRef<(nodeId: string) => void>(() => {});

  // React Flow instance — used to fit view after adding tools.
  const reactFlowRef = useRef<ReactFlowInstance | null>(null);

  // Requirement analysis: which non-optional consumed ports has nothing upstream
  // produced? Surfaced as the per-node "needs …" warning + config-panel guidance.
  const unmet = useMemo(() => computeUnmet(flow, toolMap), [flow, toolMap]);
  const unmetFor = useCallback(
    (data: Record<string, unknown>): string[] | undefined => {
      let u: string[] | undefined;
      if (typeof data.stepIndex === "number") {
        u = unmet.steps[data.stepIndex];
      }
      return u && u.length > 0 ? u : undefined;
    },
    [unmet],
  );

  // Transformer placement diagnostics (AD-006): the client-side mirror of the
  // build gate's placement pass, rendered inline on the offending node.
  const placement = useMemo(() => computePlacement(flow, toolMap), [flow, toolMap]);
  const placementFor = useCallback(
    (data: Record<string, unknown>): PlacementDiagnostic[] | undefined => {
      if (typeof data.stepIndex !== "number") return undefined;
      const diags = placement.filter((d) => d.stepIndex === data.stepIndex);
      return diags.length > 0 ? diags : undefined;
    },
    [placement],
  );

  // Enrich nodes with execution state and remove handler.
  const enrichedNodes = useMemo(() => {
    return initial.nodes.map((n) => {
      const stats = nodeStats?.get(n.id);
      const extra: Record<string, unknown> = {};
      if (stats) {
        extra.execState = stats.hasError
          ? "error"
          : activeNodes?.has(n.id)
            ? "active"
            : stats.partsProcessed > 0
              ? "complete"
              : undefined;
        extra.partCount = stats.partsProcessed;
        const span = spans?.get(n.id);
        if (span !== undefined) extra.spanUs = span;
      } else if (activeNodes?.has(n.id)) {
        extra.execState = "active";
      }
      if (n.type === "tool" || n.type === "parallel") {
        const u = unmetFor(n.data);
        if (u) extra.unmet = u;
        const p = placementFor(n.data);
        if (p) extra.placement = p;
        const toolName = n.data.toolName as string | undefined;
        if (toolName && projectPresets?.[toolName]) extra.hasPreset = true;
      }
      if (!readOnly && n.type === "tool") {
        extra.onRemove = () => removeNodeRef.current(n.id);
      }
      if (!readOnly && n.type === "parallel") {
        extra.onRemove = () => removeNodeRef.current(n.id);
        extra.onSelectBranch = (branchIndex: number) =>
          setSelectedNodeId(`${n.id}::b${branchIndex}`);
      }
      // A lesson step pointing at this node draws a highlight ring (a branch
      // selection like `tool-2::b1` highlights its parent group node).
      const focusTarget = focusRequest?.select;
      if (focusTarget && (focusTarget === n.id || focusTarget.startsWith(`${n.id}::`))) {
        extra.lessonFocus = true;
      }
      if (Object.keys(extra).length === 0) return n;
      return { ...n, data: { ...n.data, ...extra } };
    });
  }, [
    initial.nodes,
    nodeStats,
    activeNodes,
    spans,
    readOnly,
    unmetFor,
    placementFor,
    projectPresets,
    focusRequest?.select,
  ]);

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
    const targets = new Set(initial.edges.map((e) => e.target));
    const sources = new Set(initial.edges.map((e) => e.source));
    const roots = toolNodes.filter((n) => !targets.has(n.id));
    const leaves = toolNodes.filter((n) => !sources.has(n.id));

    // The serpentine layout supplies its own endpoint geometry (positions + the
    // handle side that faces the flow). `initial.nodes` here are all flow nodes,
    // so `ends` is always present.
    const ends = (initial as { ends?: { source: EndpointGeom; sink: EndpointGeom } }).ends!;
    const srcPos = { x: ends.source.x, y: ends.source.y };
    const sinkPos = { x: ends.sink.x, y: ends.sink.y };

    const endpointData = (role: "source" | "sink", binding?: string) => ({
      role,
      binding: parseBinding(binding),
      readOnly,
      handlePosition: role === "source" ? ends.source.handlePosition : ends.sink.handlePosition,
      onBindingChange: role === "source" ? handleSourceChange : handleSinkChange,
      // With a host-supplied inspector, the pill grows an Inspect satellite
      // that opens the endpoint panel (selection drives the right overlay).
      onInspect: renderEndpointPanel
        ? () => setSelectedNodeId(role === "source" ? "endpoint-source" : "endpoint-sink")
        : undefined,
      lessonFocus: focusRequest?.select === `endpoint-${role}` || undefined,
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

    // Center-align each row so the edges run straight. Seeded with an estimate
    // for the first paint; heightVersion re-runs this with measured heights.
    const aligned = centerAlignRows(
      [...enrichedNodes, sourceNode, sinkNode],
      SERP_ROW_H,
      (n) => heightCache.current.get(n.id) ?? estimateNodeHeight(n),
    );

    // A wrap edge (carriage return: it drops to a lower row and sweeps back
    // left) is routed THROUGH the inter-row gap rather than at the source/target
    // Y midpoint — otherwise a tall parallel's bottom dips past the midpoint and
    // the edge cuts through the group. `wrapCenterY` is the middle of the gap
    // below the source row; DotEdge feeds it to getSmoothStepPath as centerY.
    const boundsOf = (id: string) => {
      const n = aligned.find((x) => x.id === id);
      if (!n) return null;
      const h = heightCache.current.get(id) ?? estimateNodeHeight(n);
      return { x: n.position.x, top: n.position.y, bottom: n.position.y + h };
    };
    const routeWrap = (e: Edge): Edge => {
      const s = boundsOf(e.source);
      const t = boundsOf(e.target);
      if (s && t && t.top >= s.bottom && t.x < s.x) {
        return { ...e, data: { ...e.data, wrapCenterY: (s.bottom + t.top) / 2 } };
      }
      return e;
    };

    return {
      displayNodes: aligned,
      displayEdges: [...initial.edges, ...endpointEdges].map(routeWrap),
    };
  }, [
    enrichedNodes,
    initial.nodes,
    initial.edges,
    readOnly,
    flow.source,
    flow.sink,
    handleSourceChange,
    handleSinkChange,
    renderEndpointPanel,
    focusRequest?.select,
    heightVersion,
  ]);

  const [nodes, setNodes, onNodesChange] = useNodesState(displayNodes);
  const [edges, setEdges, onEdgesChange] = useEdgesState(displayEdges);

  // Sync graph when flow prop changes (e.g., tool added, template selected).
  useEffect(() => {
    setNodes(displayNodes);
    setEdges(displayEdges);
  }, [displayNodes, displayEdges, setNodes, setEdges]);

  // Node content (and therefore height) changes with topology and wrap width;
  // drop stale measurements so rows re-measure and re-center.
  useEffect(() => {
    heightCache.current.clear();
    setHeightVersion((v) => v + 1);
  }, [topologyKey, columns]);

  // Capture React Flow's measured heights; bump heightVersion (→ re-run the
  // center-align in displayNodes) only when a height actually changes, so this
  // settles after the first measured frame instead of looping.
  useEffect(() => {
    let changed = false;
    for (const n of nodes) {
      const h = n.measured?.height;
      if (h && heightCache.current.get(n.id) !== h) {
        heightCache.current.set(n.id, h);
        changed = true;
      }
    }
    if (changed) setHeightVersion((v) => v + 1);
  }, [nodes]);

  // Run metadata ON the edges, literal to the trace: `traversed` is how many
  // parts crossed the edge at the cursor, `transit` how many are mid-hop on it
  // right now (between exiting the source and entering the target). DotEdge
  // animates a dot per in-transit part only while PLAYING — paused is a frozen
  // frame — so the dots match the parts actually flowing, never a decoration.
  const transits = useMemo(
    () => (runEvents ? edgeTransits(runEvents, cursor) : null),
    [runEvents, cursor],
  );
  const flowEdges = useMemo(() => {
    if (!runEvents || runEvents.length === 0) return edges;
    return edges.map((e) => {
      const traversed = nodeStats?.get(e.source)?.partsProcessed ?? 0;
      const transit = transits?.get(`${e.source}→${e.target}`) ?? 0;
      if (traversed === 0 && transit === 0) return e;
      return {
        ...e,
        data: {
          ...e.data,
          ...(traversed > 0 ? { traversed } : {}),
          ...(transit > 0 ? { transit } : {}),
          ...(transit > 0 && tracePlaying ? { flowing: true } : {}),
        },
      };
    });
  }, [edges, runEvents, nodeStats, transits, tracePlaying]);

  const handleNodesChange = useCallback(
    (changes: Parameters<typeof onNodesChange>[0]) => {
      onNodesChange(changes);
    },
    [onNodesChange],
  );

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Source/Sink are bindings, not steps — they have their own dropdown UI
    // and never open the tool config panel.
    if (node.type !== "tool" && node.type !== "parallel") return;
    // A parallel group selects its first branch (branch rows select their own).
    if (node.type === "parallel") {
      setSelectedNodeId(`${node.id}::b0`);
      return;
    }
    setSelectedNodeId(node.id);
    // With a run loaded, a node opens its run inspector first; the panel's
    // Configure button flips to the config form.
    setPanelMode("inspect");
  }, []);

  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null);
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
      const newNodeIndex = flow.steps.length;
      const updated: FlowSpec = {
        ...flow,
        steps: [...flow.steps, { tool: toolName }],
      };
      onChange(updated);
      // Close the browse modal (if open) and auto-select the new tool so the
      // config panel opens immediately.
      setAddOpen(false);
      setSelectedNodeId(`tool-${newNodeIndex}`);
      // Re-anchor the content top-left at 100% (keeps Source pinned top-left).
      requestAnimationFrame(() => {
        reactFlowRef.current?.setViewport(
          { x: CANVAS_MARGIN, y: CANVAS_MARGIN, zoom: 1 },
          { duration: 300 },
        );
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
  // bindings are not nodes, so carry them through unchanged. (Inert under the
  // serpentine layout, which is non-draggable, but kept defensive.)
  const handleNodeDragStop = useCallback(() => {
    const updated = graphToSteps(nodes, {
      source: flow.source,
      sink: flow.sink,
    });
    onChange(updated);
  }, [nodes, onChange, flow.source, flow.sink]);

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
  // duplicate-tool nodes and parallel branches distinct. A selection of the form
  // "<groupId>::b<i>" targets branch i of a parallel group node.
  const sepIdx = selectedNodeId?.indexOf("::b") ?? -1;
  const selGroupId = sepIdx >= 0 ? selectedNodeId!.slice(0, sepIdx) : selectedNodeId;
  const selBranchIndex = sepIdx >= 0 ? Number(selectedNodeId!.slice(sepIdx + 3)) : undefined;
  const selectedNode = selGroupId ? (nodes.find((n) => n.id === selGroupId) ?? null) : null;
  const selectedBranch =
    selBranchIndex !== undefined && selectedNode?.type === "parallel"
      ? (selectedNode.data.branches as ParallelBranch[] | undefined)?.[selBranchIndex]
      : undefined;

  // The node data used for FlowSpec resolution: a branch resolves to its parent
  // group's stepIndex + branchIndex; otherwise the node's own data.
  const resolveData: NodeStepData | undefined = selectedBranch
    ? { stepIndex: selectedNode!.data.stepIndex, branchIndex: selBranchIndex }
    : (selectedNode?.data as NodeStepData | undefined);

  const selectedToolName = (selectedBranch?.toolName ??
    (selectedNode?.type === "parallel" ? undefined : selectedNode?.data?.toolName)) as
    | string
    | undefined;

  const selectedLocation = useMemo(
    () => resolveStepLocation(resolveData),
    // resolveData is rebuilt each render; key on its resolvable fields.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [resolveData?.stepIndex, resolveData?.branchIndex],
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
    <div className="relative flex h-full overflow-hidden">
      {/* Canvas (full width — palette and config are overlays, not flex siblings,
          so selecting a node or browsing tools never reflows the graph). */}
      <div className="flex-1 flex flex-col min-w-0">
        {/* Toolbar */}
        <FlowToolbar
          stepCount={flow.steps.length}
          onRun={onRun}
          runDisabled={runDisabled}
          flow={flow}
          redacted={hasRedactionWrap(flow)}
          onToggleRedaction={
            readOnly
              ? undefined
              : () =>
                  onChange(hasRedactionWrap(flow) ? unwrapRedaction(flow) : wrapWithRedaction(flow))
          }
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
          <div
            ref={canvasRef}
            className={cn("flex-1", "nk-flow-anim")}
            onDrop={handleDrop}
            onDragOver={handleDragOver}
          >
            <ReactFlow
              nodes={nodes}
              edges={flowEdges}
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
              nodesDraggable={false}
              nodesConnectable={!readOnly}
              // Anchor the content top-left (Source at the top-left) instead of
              // centering it, at a fixed 100% zoom. Zoom is locked (min == max
              // == 1): no scroll/pinch/double-click zoom; users pan to navigate.
              defaultViewport={{ x: CANVAS_MARGIN, y: CANVAS_MARGIN, zoom: 1 }}
              minZoom={1}
              maxZoom={1}
              zoomOnScroll={false}
              zoomOnPinch={false}
              zoomOnDoubleClick={false}
              panOnScroll
              panOnDrag
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

              {/* Add-tool affordance — opens the browse modal. Keeps the canvas
                  full-width (no permanent palette sidebar). */}
              {!readOnly && (
                <Panel position="top-left">
                  <Button size="sm" onClick={() => setAddOpen(true)} aria-label="Add tool">
                    <Plus size={14} />
                    Add tool
                  </Button>
                </Panel>
              )}

              <Panel position="top-right">
                <FlowLegend />
              </Panel>
            </ReactFlow>
          </div>
        )}

        {/* Run playback (bottom of canvas column): one slim transport row —
            the run data itself lives ON the graph (node duration/part badges,
            edge traversal counts), so the canvas stays visible. */}
        {runEvents && runEvents.length > 0 && (
          <TracePanel
            events={runEvents}
            cursor={cursor}
            onCursorChange={setTraceCursor}
            playing={tracePlaying}
            onPlayingChange={setTracePlaying}
            durationUs={trace?.durationUs}
            onClose={() => {
              setTraceDismissed(true);
              setTracePlaying(false);
              setTraceCursor(null);
              onTraceDismiss?.();
            }}
          />
        )}
      </div>

      {/* Endpoint inspector — same floating right overlay, but for the Source /
          Sink terminals: the host renders the body (input content model,
          written output) via renderEndpointPanel. */}
      {renderEndpointPanel &&
        (selectedNodeId === "endpoint-source" || selectedNodeId === "endpoint-sink") && (
          <div className="absolute right-0 top-0 bottom-0 z-20 shadow-[-8px_0_24px_oklch(0_0_0/0.25)]">
            <EndpointInspectorPanel
              role={selectedNodeId === "endpoint-source" ? "source" : "sink"}
              onClose={() => setSelectedNodeId(null)}
            >
              {renderEndpointPanel(selectedNodeId === "endpoint-source" ? "source" : "sink", () =>
                setSelectedNodeId(null),
              )}
            </EndpointInspectorPanel>
          </div>
        )}

      {/* Right panel — a floating overlay pinned to the right of the canvas,
          NOT a flex sibling, so opening it never resizes the graph. With a run
          loaded, the run inspector opens first (what passed through this step,
          and what it attached); Configure flips to the config form. */}
      {selectedStep &&
        (trace && !traceDismissed && panelMode === "inspect" && selectedLocation ? (
          <div className="absolute right-0 top-0 bottom-0 z-20 shadow-[-8px_0_24px_oklch(0_0_0/0.25)]">
            <RunInspectorPanel
              key={`inspect-${selectedNodeId}`}
              trace={trace}
              stepToolCounts={stepToolCounts(flow.steps)}
              stepIndex={selectedLocation.index}
              stepLabel={selectedToolName ?? `step ${selectedLocation.index + 1}`}
              onConfigure={readOnly ? undefined : () => setPanelMode("configure")}
              onClose={() => setSelectedNodeId(null)}
            />
          </div>
        ) : (
          <div className="absolute right-0 top-0 bottom-0 z-20 shadow-[-8px_0_24px_oklch(0_0_0/0.25)]">
            {/* A host can replace the config panel for specific tools (e.g.
                the lab mounts a code editor for `script`); null falls back to
                the schema-driven default. */}
            {(selectedToolName &&
              renderStepConfigPanel?.({
                toolName: selectedToolName,
                step: selectedStep,
                config: selectedStep.config || {},
                onConfigChange: handleConfigChange,
                onClose: () => setSelectedNodeId(null),
                onRemove: readOnly ? undefined : handleRemoveSelected,
              })) || (
              <StepConfigPanel
                key={selectedNodeId}
                step={selectedStep}
                toolInfo={selectedToolInfo}
                schema={selectedSchema}
                doc={selectedDoc}
                config={selectedStep.config || {}}
                preset={selectedToolName ? projectPresets?.[selectedToolName] : undefined}
                unmet={selectedNode ? unmetFor(selectedNode.data) : undefined}
                placement={selectedNode ? placementFor(selectedNode.data) : undefined}
                onConfigChange={handleConfigChange}
                onClose={() => setSelectedNodeId(null)}
                onRemove={readOnly ? undefined : handleRemoveSelected}
              />
            )}
          </div>
        ))}

      {/* Browse-and-add tools — a modal so the canvas stays full-width. */}
      {!readOnly && (
        <Dialog open={addOpen} onOpenChange={setAddOpen}>
          <DialogContent className="max-w-md gap-0 overflow-hidden p-0">
            <DialogHeader className="px-4 pb-2 pt-4">
              <DialogTitle className="text-sm">Add a tool</DialogTitle>
            </DialogHeader>
            <div className="h-[60vh]">
              <ToolPalette tools={tools} onAddTool={handleAddTool} embedded />
            </div>
          </DialogContent>
        </Dialog>
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
