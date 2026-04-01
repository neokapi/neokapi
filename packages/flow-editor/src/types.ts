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
}

export interface FlowEdgeInfo {
  id: string;
  source: string;
  target: string;
}

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
}

// Schema types for tool/format configuration

export interface ComponentSchema {
  $id?: string;
  $version?: string;
  title: string;
  description?: string;
  type: string;
  "x-component"?: ComponentMeta;
  "x-format"?: FormatMeta;
  "x-tool"?: ToolMeta;
  "x-groups"?: ParameterGroup[];
  "x-apiVersion"?: string;
  "x-introducedInOkapi"?: string;
  properties?: Record<string, PropertySchema>;
  $defs?: Record<string, PropertySchema>;
}

/** Format metadata from x-format — describes a document format. */
export interface FormatMeta {
  id: string;
  extensions?: string[];
  mimeTypes?: string[];
  presets?: FormatPreset[];
}

/** A named preset from x-format.presets. */
export interface FormatPreset {
  id: string;
  name: string;
  description?: string;
  mimeType?: string;
  extensions?: string;
  parameters?: Record<string, unknown>;
  isDefault?: boolean;
}

/** Tool metadata from x-tool — neokapi tool classification. */
export interface ToolMeta {
  displayName?: string;
  description?: string;
  category?: string;
  inputs?: string[];
  outputs?: string[];
  tags?: string[];
  requires?: string[];
}

export interface ComponentMeta {
  id: string;
  type: string;
  category?: string;
  displayName?: string;
  description?: string;
  inputs?: string[];
  outputs?: string[];
  tags?: string[];
  requires?: string[];
}

export interface ParameterGroup {
  id: string;
  label: string;
  description?: string;
  collapsed?: boolean;
  fields: string[];
}

export interface PropertySchema {
  type: string;
  description?: string;
  default?: unknown;
  deprecated?: boolean;
  title?: string;
  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;
  /** Simple widget hint (legacy, superseded by x-editor when present). */
  "x-widget"?: string;
  "x-placeholder"?: string;
  "x-presets"?: Record<string, unknown>;
  "x-showIf"?: { field: string; value?: unknown; empty?: boolean };
  /** Structured UI editor metadata from Okapi EditorDescription. */
  "x-editor"?: EditorMeta;
  /** Display labels for enum values, keyed by enum value. */
  "x-enumLabels"?: Record<string, string>;
  /** Descriptions for enum values, keyed by enum value. */
  "x-enumDescriptions"?: Record<string, string>;
  /** Display order hint (lower = earlier). */
  "x-order"?: number;
  /** Original Okapi parameter name when schema uses a cleaner name. */
  "x-flattenPath"?: string;
  /** Identifies complex Okapi types (e.g. "inlineCodeFinder"). */
  "x-okapiFormat"?: string;
  /** Okapi version where this parameter was introduced. */
  "x-introducedInOkapi"?: string;
  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  /** JSON Schema $ref pointer (e.g. "#/$defs/inlineCodes"). */
  $ref?: string;
  /** Tuple items for fixed-length arrays (JSON Schema prefixItems). */
  prefixItems?: PropertySchema[];
  minItems?: number;
  maxItems?: number;
  oneOf?: PropertySchema[];
}

/**
 * Structured UI editor metadata from x-editor.
 * Mirrors the Okapi EditorDescription / AbstractPart class hierarchy.
 */
export interface EditorMeta {
  /** Widget type discriminator. */
  widget: EditorWidget;
  /** Master/slave dependency — this field is enabled/disabled by another parameter. */
  enabledBy?: {
    parameter: string;
    enabledWhenSelected: boolean;
  };
  /** Layout hints for label positioning. */
  layout?: {
    /** Show label (default true). */
    withLabel?: boolean;
    /** Label above input vs beside (default false = beside). */
    vertical?: boolean;
  };
  /** Properties for widget: "text". */
  text?: {
    password?: boolean;
    allowEmpty?: boolean;
    /** Rows for multiline textarea. Omit or 1 for single-line. */
    height?: number;
  };
  /** Properties for widget: "path". */
  path?: {
    browseTitle?: string;
    forSaveAs?: boolean;
    allowEmpty?: boolean;
    filters?: Array<{ name: string; extensions: string }>;
  };
  /** Properties for widget: "folder". */
  folder?: {
    browseTitle?: string;
  };
  /** Properties for widget: "checkList". */
  checkList?: {
    entries: Array<{ name: string; title: string; description?: string }>;
  };
}

/** Widget types from x-editor.widget. */
export type EditorWidget =
  | "checkbox"
  | "text"
  | "spin"
  | "dropdown"
  | "select"
  | "path"
  | "folder"
  | "codeFinder"
  | "checkList";

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
