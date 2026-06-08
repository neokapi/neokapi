/**
 * Component Parameter Schema Types
 *
 * These types match the schema language produced by the okapi-bridge
 * transformation pipeline, with ui: extensions for UI rendering.
 *
 * Both formats and tools use the same schema format. FormatSchema is a
 * specialization of ComponentSchema with format-specific metadata.
 */

/** Generalized component schema — works for both formats and tools. */
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
  $defs?: Record<string, PropertySchema>;
}

/** Format-specific schema. */
export interface FormatSchema extends ComponentSchema {
  formatMeta: FormatMeta;
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

/** Tool metadata. */
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
 */
export type ConditionExpr =
  | { field: string; eq: unknown }
  | { field: string; empty: boolean }
  | { all: ConditionExpr[] }
  | { any: ConditionExpr[] }
  | { not: ConditionExpr };

export interface PropertySchema {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  deprecated?: boolean;
  enum?: unknown[];
  minimum?: number;
  maximum?: number;

  // Nested object properties
  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  required?: string[];
  $ref?: string;

  // UI hints (ui: prefix)
  "ui:widget"?: string;
  "ui:placeholder"?: string;
  "ui:presets"?: Record<string, unknown>;
  "ui:order"?: number;
  "ui:visible"?: ConditionExpr;
  "ui:enabled"?: ConditionExpr;
  "ui:enum-labels"?: Record<string, string>;
  "ui:enum-descriptions"?: Record<string, string>;
  "ui:introduced-in"?: string;

  // Okapi bridge extensions
  "x-okapi-flatten-path"?: string;
  "x-okapi-format"?: string;
}

/**
 * CodeFinderRules value structure (clean JSON format).
 */
export interface CodeFinderRulesValue {
  rules: Array<{ pattern: string }>;
  sample?: string;
  useAllRulesWhenTesting?: boolean;
}

/**
 * Format parameters value type.
 */
export type FormatParamsValue = Record<string, unknown>;
