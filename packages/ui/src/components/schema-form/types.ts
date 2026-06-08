// Schema-form types — shared across all consumers.
// Canonical definitions for the schema language used by filters, tools, and formats.

import type { SchemaFormHost } from "./host";

export interface ComponentSchema {
  $id?: string;
  $version?: string;
  title: string;
  description?: string;
  type: string;

  formatMeta?: FormatMeta;
  toolMeta?: ToolMeta;
  presets?: Record<string, Record<string, unknown>>;

  "ui:groups"?: ParameterGroup[];

  properties?: Record<string, PropertySchema>;
}

export interface FormatMeta {
  id: string;
  extensions?: string[];
  mimeTypes?: string[];
}

/** One entry of a tool's IO contract (mirrors core/schema.IOPort). */
export interface IOPort {
  type: string;
  side?: "source" | "target";
  optional?: boolean;
  layer?: string;
}

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

export type ConditionExpr =
  | { field: string; eq: unknown }
  | { field: string; empty: boolean }
  | { all: ConditionExpr[] }
  | { any: ConditionExpr[] }
  | { not: ConditionExpr };

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

  enum?: unknown[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  maxLength?: number;

  options?: { value: unknown; label: string }[];

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

  "x-okapi-flatten-path"?: string;
  "x-okapi-format"?: string;
  "x-path"?: {
    type?: string;
    role?: string;
    resourceKind?: string;
    accepts?: string[];
    browseTitle?: string;
    forSaveAs?: boolean;
    filters?: Array<{ name: string; extensions: string }>;
  };

  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  prefixItems?: PropertySchema[];
  minItems?: number;
  maxItems?: number;
  oneOf?: PropertySchema[];
}

export interface ToolDoc {
  displayName?: string;
  overview?: string;
  parameters?: Record<string, ToolDocParam>;
  limitations?: string[];
  processingNotes?: string[];
  examples?: Array<{ title: string; description?: string; input?: string; output?: string }>;
  wikiUrl?: string;
}

export interface ToolDocParam {
  description?: string;
  help?: string;
  notes?: string[];
  introducedIn?: string;
  dependsOn?: Array<{ property: string; condition: string }>;
  values?: string;
  seeAlso?: string;
}

export interface SchemaFormProps {
  schema: ComponentSchema;
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact?: boolean;
  presetValues?: Record<string, unknown>;
  paramDocs?: Record<string, ToolDocParam>;
  /** Named resources for ResourcePicker integration (e.g., TM, termbase). */
  resources?: Record<string, Array<{ name: string; path?: string; entryCount?: number }>>;
  /** Context for path resolution (project dir, output dir). */
  resourceContext?: { projectDir: string; outputDir: string };
  readOnly?: boolean;
  /** Hide the schema title/description header (useful when the parent already shows it). */
  hideHeader?: boolean;
  /**
   * Host-injected capabilities for widgets that cannot assume a runtime
   * environment — file/folder browsing and credential lookup. Provided by the
   * embedding app (e.g. kapi-desktop wires Wails dialogs); omitted on the docs
   * website, where the affected widgets degrade to plain text inputs.
   */
  host?: SchemaFormHost;
}
