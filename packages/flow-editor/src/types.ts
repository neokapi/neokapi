// Flow editor types — shared between Kapi Desktop and Bowrain Desktop.

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
}

export interface FlowStep {
  tool: string;
  config?: Record<string, unknown>;
  label?: string;
}

export interface FlowSpec {
  description?: string;
  steps: FlowStep[];
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
}
