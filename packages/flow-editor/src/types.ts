// Flow editor types — shared between Kapi and Bowrain Desktop.
//
// The IO-contract and schema-language types (Side, IOPort, ComponentSchema,
// ToolMeta, …) live in the shared @neokapi/contract-types package (issue #817).
// They are re-exported here so existing `@neokapi/flow-editor` importers keep
// resolving them from this module.
export type {
  Side,
  IOPort,
  ComponentSchema,
  FormatMeta,
  ToolMeta,
  ParameterGroup,
  ConditionExpr,
  LayoutHints,
  PropertySchema,
  ToolDoc,
  ToolDocParam,
  // Canonical vocabularies generated from Go (drift-checked).
  ToolCategory,
  OverlayType,
  AnnotationType,
  PortType,
} from "@neokapi/contract-types";

import type { ComponentSchema, IOPort, ToolDoc } from "@neokapi/contract-types";

export interface FlowDefinitionInfo {
  id: string;
  name: string;
  description?: string;
  nodes: FlowNodeInfo[];
  edges: FlowEdgeInfo[];
  /** Provenance of the flow definition (built-in, user, or project). */
  source: "built-in" | "user" | "project";
  /**
   * Where content enters/leaves the flow, as wire-format string *locators*
   * (AD-026). Nested under `binding` because `source` on the definition is
   * already the provenance field. Each value is a string: `file` | `store` |
   * `none`, or an interchange format id (`xliff` | `po` | `tmx` | `tbx`).
   * Omitted = file binding (the default). Parse/format via the codecs in
   * defAdapter.ts (`parseBinding` / `formatBinding`).
   */
  binding?: { source?: string; sink?: string };
}

/**
 * A flow I/O binding (AD-026) — the *internal* UI representation. A flow owns
 * no I/O; reader/writer are no longer graph nodes. Where content enters (source)
 * and leaves (sink) is expressed as a binding alongside the steps spec, not as a
 * node.
 *
 * On the wire this is a string locator (see `FlowSpec.source` / `sink` and
 * `FlowDefinitionInfo.binding`); the editor parses it into this object for the
 * endpoint pickers and serializes it back. Kinds:
 *  - `file`        — read/write the project's files (the default when omitted)
 *  - `store`       — read/write the content store
 *  - `interchange` — read/write a bilingual interchange format (xliff, po, …);
 *                    `format` names which one
 *  - `none`        — no binding (e.g. a flow that only annotates in place)
 */
export type FlowBindingKind = "file" | "store" | "interchange" | "none";

export interface FlowBinding {
  kind: FlowBindingKind;
  /** Interchange format id (e.g. "xliff", "po") — only when kind is "interchange". */
  format?: string;
}

export interface FlowNodeInfo {
  id: string;
  type: "tool";
  name: string;
  label?: string;
  config?: Record<string, unknown>;
  position: { x: number; y: number };
}

export interface FlowEdgeInfo {
  id: string;
  source: string;
  target: string;
}

export type LocaleCardinality = "monolingual" | "bilingual" | "multilingual";

export interface ToolInfo {
  name: string;
  display_name?: string;
  description: string;
  category: string;
  source?: string;
  has_schema?: boolean;
  tags?: string[];
  requires?: string[];
  cardinality?: LocaleCardinality;
  default_locale?: string;
  /** Ports the tool reads upstream (non-optional = a requirement). */
  consumes?: IOPort[];
  /** Ports the tool writes. */
  produces?: IOPort[];
  side_effects?: string[];
  /**
   * Whether this tool is a transformer — it may rewrite the source (AD-006).
   * Transformers are ordinary ordered steps; the placement pass validates
   * their position (see placement.ts).
   */
  isSourceTransform?: boolean;
  /**
   * A recoverable transformer vaults the originals it removes and restores
   * them later (redaction); the placement pass holds it to the remote-egress
   * rule.
   */
  recoverable?: boolean;
}

export interface FlowStep {
  tool: string;
  config?: Record<string, unknown>;
  label?: string;
  /** Fan-out: run these steps in parallel instead of sequentially. */
  parallel?: FlowStep[];
}

export interface FlowSpec {
  description?: string;
  steps: FlowStep[];
  /**
   * Where content enters the flow, as a wire-format string locator (AD-026). A
   * flow owns no I/O; this names the input: `file` | `store` | `none`, or an
   * interchange format id (`xliff` | `po` | `tmx` | `tbx`). Omitted = `file`
   * (the default). Parse/format via `parseBinding` / `formatBinding`.
   */
  source?: string;
  /**
   * Where content leaves the flow, as a wire-format string locator (AD-026).
   * Same value space as `source`. Omitted = `file` (the default).
   */
  sink?: string;
}

/** Props for the FlowEditor component — fully decoupled from any backend. */
export interface FlowEditorProps {
  /** The flow to display/edit, in steps format. */
  flow: FlowSpec;
  /** Available tools for the tool palette. */
  tools: ToolInfo[];
  /** Called when the flow is modified. */
  onChange: (flow: FlowSpec) => void;
  /** Called when the user requests to run the flow. */
  onRun?: (flow: FlowSpec) => void;
  /** Whether the run button is disabled (e.g. a flow is already running). */
  runDisabled?: boolean;
  /**
   * A run is actually in flight — drives the Run button's spinner/"Running…"
   * label. `runDisabled` alone can also mean "engine not ready" or
   * "read-only replay", which must not read as running.
   */
  running?: boolean;
  /** Whether the flow is read-only (built-in flows). */
  readOnly?: boolean;
  /** Called to fetch a tool's config schema. Returns null if none available. */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  /** Called to fetch documentation for a tool. Returns null if unavailable. */
  onGetDoc?: (toolName: string) => ToolDoc | null;
  /** Trace events from a running or completed flow execution. */
  traceEvents?: import("./traceTypes").TraceEvent[];
  /**
   * Full trace data from a completed execution of THIS flow (part snapshots
   * included). When set, the editor enters run review: a playback transport
   * replays the events on the designed nodes, and selecting a node opens its
   * run inspector (the blocks that passed through, with the overlay/annotation
   * delta the step produced).
   */
  trace?: import("./traceTypes").FlowTrace;
  /** Called when the user dismisses the loaded run (clears review mode). */
  onTraceDismiss?: () => void;
  /**
   * Project-level tool presets (the recipe's defaults.tools): per-tool config
   * defaults the engine merges under each step's own config (the step wins per
   * key). The editor badges preset-backed nodes and the config panel shows the
   * inherited values with override indicators.
   */
  projectPresets?: Record<string, Record<string, unknown>>;
  /**
   * Host-rendered inspector content for the Source / Sink endpoint nodes. When
   * provided, each endpoint pill gains an Inspect affordance that opens the
   * editor's right overlay panel with this content — e.g. the content-model
   * tree the reader produces from the bound input (source), or the written
   * output with a round-trip diff (sink). The flow editor supplies the panel
   * chrome; the host supplies only the body.
   */
  renderEndpointPanel?: (role: "source" | "sink", close: () => void) => import("react").ReactNode;
  /**
   * Host-driven focus (e.g. a guided lesson step): when `nonce` changes the
   * editor applies the request — selecting a tool node (`tool-<i>`, opening
   * its run inspector or config panel per `mode`) or an endpoint
   * (`endpoint-source` / `endpoint-sink`, opening the endpoint inspector), or
   * clearing the selection (`select: null`). The focused node is also drawn
   * with a highlight ring so a lesson can point at it.
   */
  focusRequest?: FlowFocusRequest;
  /**
   * Host-supplied replacement for a step's config panel. Called when a tool
   * node's configuration opens; returning non-null renders that panel (in the
   * editor's right overlay) instead of the schema-driven default — e.g. the
   * lab mounts a code editor for the `script` tool. Return null to keep the
   * default panel.
   */
  renderStepConfigPanel?: (ctx: StepConfigRenderContext) => import("react").ReactNode | null;
  /**
   * Host-rendered lesson callout, shown as an overlay INSIDE the canvas
   * (bottom-left) on sm+ screens so the lesson sits next to the nodes it
   * points at instead of consuming page height above the editor. Hidden on
   * phones — stack the same content above the editor there instead.
   */
  lessonPanel?: import("react").ReactNode;
}

/** Context passed to renderStepConfigPanel (see FlowEditorProps). */
export interface StepConfigRenderContext {
  toolName: string;
  step: FlowStep;
  config: Record<string, unknown>;
  onConfigChange: (config: Record<string, unknown>) => void;
  onClose: () => void;
  onRemove?: () => void;
}

/** One host-driven focus application (see FlowEditorProps.focusRequest). */
export interface FlowFocusRequest {
  /** Re-apply trigger: the editor applies the request once per nonce value. */
  nonce: number;
  /** Node id to select (`tool-<i>`, `endpoint-source`, `endpoint-sink`), or null to clear. */
  select: string | null;
  /** Panel to open for a tool node (default "inspect"; pre-run falls back to config). */
  mode?: "inspect" | "configure";
}
