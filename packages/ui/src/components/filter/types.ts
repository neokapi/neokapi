/**
 * Component Parameter Schema Types
 *
 * These types match the JSON Schema format produced by the okapi-bridge
 * transformation pipeline, with x-* extensions for UI rendering.
 *
 * Both formats and tools use the same schema format. FormatSchema is a
 * specialization of ComponentSchema with format-specific metadata.
 */

/** Generalized component schema — works for both formats and tools. */
export interface ComponentSchema {
  $id: string;
  $version?: string;
  title: string;
  description?: string;
  type: string;
  "x-component"?: ComponentMeta;
  "x-format"?: FormatMeta;
  "x-tool"?: ToolMeta;
  "x-groups"?: ParameterGroup[];
  properties: Record<string, PropertySchema>;
}

/** Component metadata (for tools, formats, etc.) */
export interface ComponentMeta {
  id: string;
  type: string; // "format" | "tool"
  category?: string; // "transform" | "validate" | "enrich" | "convert" | "pipeline"
  displayName?: string;
  description?: string;
}

/** Format-specific schema. */
export interface FormatSchema extends ComponentSchema {
  "x-format": FormatMeta;
}

export interface FormatMeta {
  id: string;
  extensions?: string[];
  mimeTypes?: string[];
  presets?: FormatPreset[];
}

export interface FormatPreset {
  id: string;
  name: string;
  description?: string;
  parameters?: Record<string, unknown>;
  isDefault?: boolean;
}

/** Tool metadata from x-tool. */
export interface ToolMeta {
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
  collapsed?: boolean;
  fields: string[];
}

export interface PropertySchema {
  type: string;
  description?: string;
  default?: unknown;
  deprecated?: boolean;
  enum?: string[];

  // Nested object properties
  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  required?: string[];

  // UI hints (x-* extensions)
  "x-widget"?: WidgetType;
  "x-placeholder"?: string;
  "x-presets"?: Record<string, unknown>;
  "x-okapiFormat"?: string;
  "x-order"?: number;
  "x-showIf"?: ShowIfCondition;
}

export type WidgetType =
  | "regexBuilder"
  | "codeFinderRules"
  | "tagList"
  | "numberList"
  | "simplifierRulesEditor"
  | "filterSelector";

export interface ShowIfCondition {
  field: string;
  value: unknown;
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
