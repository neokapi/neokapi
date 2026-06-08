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
  /** Which pipeline stage this node belongs to. "" or omitted = main stage. */
  stage?: "" | "source-transform";
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
  /** Whether this tool may run in the source-transform stage (rewrite source/model). */
  isSourceTransform?: boolean;
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
  /** Leading tools that rewrite the source/model before the main steps run. */
  sourceTransforms?: FlowStep[];
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
  /** Whether the flow is read-only (built-in flows). */
  readOnly?: boolean;
  /** Called to fetch a tool's config schema. Returns null if none available. */
  onGetSchema?: (toolName: string) => ComponentSchema | null;
  /** Called to fetch documentation for a tool. Returns null if unavailable. */
  onGetDoc?: (toolName: string) => ToolDoc | null;
  /** Trace events from a running or completed flow execution. */
  traceEvents?: import("./traceTypes").TraceEvent[];
  /** Full trace data from a completed execution (includes part snapshots). */
  trace?: import("./traceTypes").FlowTrace;
}

/** Tool category identifiers with display metadata. */
export type ToolCategory =
  | "translate"
  | "validate"
  | "transform"
  | "convert"
  | "enrich"
  | "pipeline";
