// Flow editor types — shared between Kapi and Bowrain Desktop.

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

export type Side = "source" | "target";

/**
 * One entry of a tool's IO contract (consumes/produces): a typed
 * stand-off layer, the side it pertains to, and — for consumed facets —
 * whether it is optional (graceful degradation) vs a hard requirement.
 */
export interface IOPort {
  type: string;
  side?: Side;
  optional?: boolean;
  layer?: string;
}

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

// ─── Schema Language Types ─────────────────────────────────────────────────
//
// Three extension namespaces:
//   ui:*         — UI rendering hints (widget, visible, enabled, layout, groups)
//   (no prefix)  — neokapi data/metadata (formatMeta, toolMeta, presets)
//   x-okapi-*    — Okapi bridge internals (flatten-path, format, kind)

export interface ComponentSchema {
  $id?: string;
  $version?: string;
  title: string;
  description?: string;
  type: string;

  // Data/metadata fields (no prefix)
  formatMeta?: FormatMeta;
  toolMeta?: ToolMeta;
  presets?: Record<string, Record<string, unknown>>;

  // UI extensions
  "ui:groups"?: ParameterGroup[];

  // Properties
  properties?: Record<string, PropertySchema>;
  $defs?: Record<string, PropertySchema>;
}

/** Format identification metadata. */
export interface FormatMeta {
  id: string;
  extensions?: string[];
  mimeTypes?: string[];
}

/** Tool identification and classification metadata. */
export interface ToolMeta {
  id?: string;
  displayName?: string;
  description?: string;
  category?: string;
  consumes?: IOPort[];
  produces?: IOPort[];
  tags?: string[];
  requires?: string[];
}

export interface ParameterGroup {
  id: string;
  label: string;
  description?: string;
  collapsible?: boolean;
  collapsed?: boolean;
  icon?: string;
  fields: string[];
}

/**
 * Condition expression for ui:visible and ui:enabled.
 * Supports simple field comparisons and compound AND/OR/NOT.
 */
export type ConditionExpr =
  | { field: string; eq: unknown }
  | { field: string; empty: boolean }
  | { all: ConditionExpr[] }
  | { any: ConditionExpr[] }
  | { not: ConditionExpr };

/** Layout hints for a field. */
export interface LayoutHints {
  hideLabel?: boolean;
  vertical?: boolean;
  columns?: number;
}

export interface PropertySchema {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  deprecated?: boolean;

  // Validation constraints
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;

  // Labeled enum options (consolidated from enum + ui:enum-labels)
  options?: { value: unknown; label: string }[];

  // UI rendering hints (ui: prefix)
  "ui:widget"?: string;
  "ui:widget-options"?: Record<string, unknown>;
  "ui:placeholder"?: string;
  "ui:presets"?: Record<string, unknown>;
  "ui:visible"?: ConditionExpr;
  "ui:enabled"?: ConditionExpr;
  "ui:layout"?: LayoutHints;
  /** @deprecated Use options instead */
  "ui:enum-labels"?: Record<string, string>;
  "ui:enum-descriptions"?: Record<string, string>;
  "ui:order"?: number;
  "ui:deprecated-message"?: string;
  "ui:introduced-in"?: string;

  // Okapi bridge extensions (x-okapi- prefix)
  "x-okapi-flatten-path"?: string;
  "x-okapi-format"?: string;

  // JSON Schema structural
  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  $ref?: string;
  prefixItems?: PropertySchema[];
  minItems?: number;
  maxItems?: number;
  oneOf?: PropertySchema[];
}

/** Documentation for a tool or format, loaded from plugin docs. */
export interface ToolDoc {
  /** Display name (e.g. "JSON Filter", "Batch Translation Step"). */
  displayName?: string;
  /** Rich overview paragraph describing the tool's purpose and behavior. */
  overview?: string;
  /** Per-parameter documentation keyed by parameter path (e.g. "extraction.extractAll"). */
  parameters?: Record<string, ToolDocParam>;
  /** Known limitations. */
  limitations?: string[];
  /** Processing notes / tips. */
  processingNotes?: string[];
  /** Usage examples. */
  examples?: Array<{ title: string; description?: string; input?: string; output?: string }>;
  /** URL to external wiki/docs page. */
  wikiUrl?: string;
}

export interface ToolDocParam {
  description?: string;
  /** Alias for description used in okapi-bridge doc files. */
  help?: string;
  notes?: string[];
  introducedIn?: string;
  dependsOn?: Array<{ property: string; condition: string }>;
  /** Allowed values description. */
  values?: string;
  /** Cross-reference to related wiki page or parameter. */
  seeAlso?: string;
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
