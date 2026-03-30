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
  description: string;
  category: string;
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
  "x-groups"?: ParameterGroup[];
  properties?: Record<string, PropertySchema>;
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
  "x-widget"?: string;
  "x-placeholder"?: string;
  "x-presets"?: Record<string, unknown>;
  properties?: Record<string, PropertySchema>;
  items?: PropertySchema;
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
