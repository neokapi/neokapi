// Flow editor types — shared between Kapi and Bowrain Desktop.

export interface FlowDefinitionInfo {
  id: string;
  name: string;
  description?: string;
  nodes: FlowNodeInfo[];
  edges: FlowEdgeInfo[];
  source: "built-in" | "user" | "project";
}

export interface FlowNodeInfo {
  id: string;
  type: "tool" | "reader" | "writer";
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
  inputs?: string[];
  outputs?: string[];
  tags?: string[];
  requires?: string[];
  cardinality?: LocaleCardinality;
  default_locale?: string;
  produces?: string[];
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
  inputs?: string[];
  outputs?: string[];
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
