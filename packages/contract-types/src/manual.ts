// Hand-authored contract types — the superset "envelope" types that the UI
// layers extend beyond what Go models, so they have no 1:1 Go struct to generate
// from (see issue #817). They build on the generated atoms in ./contract.gen.ts.
//
// `ComponentSchema` and `PropertySchema` are the reconciled UNION of every
// consumer's historical copy: the schema language emitted by native formats/tools
// (core/schema), the okapi-bridge `x-okapi-*` extensions, and the JSON-Schema
// structural fields ($ref, $defs, oneOf, additionalProperties, …). Keeping the
// union here means a single declaration nothing has to diverge from.

import type { FormatMeta, LayoutHints, OptionItem, PathAnnotation, ToolMeta } from "./contract.gen";

/**
 * Condition expression for `ui:visible` / `ui:enabled`. Supports simple field
 * comparisons and compound AND/OR/NOT. Mirrors core/schema.ConditionExpr, but
 * expressed as a discriminated union (its natural TS form).
 */
export type ConditionExpr =
  | { field: string; eq: unknown }
  | { field: string; empty: boolean }
  | { all: ConditionExpr[] }
  | { any: ConditionExpr[] }
  | { not: ConditionExpr };

/**
 * A UI grouping of parameters. Hand-authored (not generated) because its
 * `ui:visible` gate is a {@link ConditionExpr} — used for master-detail
 * rendering of tool groups, where a whole section is shown only when its
 * condition holds. Mirrors core/schema.ParameterGroup.
 */
export interface ParameterGroup {
  id: string;
  label: string;
  description?: string;
  collapsible?: boolean;
  collapsed?: boolean;
  icon?: string;
  fields: string[];
  "ui:visible"?: ConditionExpr;
}

/**
 * One branch of a cascading select: options offered only when `when` holds.
 * Mirrors core/schema.ConditionalOptions (see {@link PropertySchema}'s
 * `ui:option-sets`).
 */
export interface ConditionalOptions {
  when?: ConditionExpr;
  options: OptionItem[];
}

/**
 * A single parameter's schema. Union of the native schema language
 * (core/schema.PropertySchema), the okapi-bridge `x-okapi-*` extensions, and the
 * JSON-Schema structural fields the bridge emits.
 */
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
  options?: OptionItem[];

  // Cascading select: options that depend on another field's value. The UI uses
  // the matching set's options; flat consumers fall back to `options`.
  "ui:option-sets"?: ConditionalOptions[];

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

  // Okapi bridge extensions (x-okapi- prefix, only in bridge schemas)
  "x-okapi-flatten-path"?: string;
  "x-okapi-format"?: string;

  // Path annotation for resource/file path properties
  "x-path"?: PathAnnotation;

  // JSON Schema structural
  properties?: Record<string, PropertySchema>;
  additionalProperties?: PropertySchema | boolean;
  items?: PropertySchema;
  required?: string[];
  $ref?: string;
  prefixItems?: PropertySchema[];
  minItems?: number;
  maxItems?: number;
  oneOf?: PropertySchema[];
}

/**
 * A JSON Schema for a component's parameters (format or tool). Union of the
 * native schema language and the bridge/JSON-Schema extensions.
 *
 * Extension namespaces:
 *   - `ui:*`       — UI rendering hints (widget, visibility, layout, groups)
 *   - (no prefix)  — neokapi data/metadata (formatMeta, toolMeta, presets)
 *   - `x-okapi-*`  — Okapi bridge internals
 */
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

/** Per-parameter documentation; handed to SchemaForm's `paramDocs` unchanged. */
export interface ToolDocParam {
  description?: string;
  /** Alias for description used in okapi-bridge doc files. */
  help?: string;
  notes?: string[];
  introducedIn?: string;
  dependsOn?: Array<{ property: string; condition: string }>;
  /** Allowed values description. */
  values?: string;
  examples?: string[];
  /** Cross-reference to related wiki page or parameter. */
  seeAlso?: string;
}

/** Documentation for a tool or format, loaded from plugin docs. */
export interface ToolDoc {
  /** Display name (e.g. "JSON Filter", "Batch Translation Step"). */
  displayName?: string;
  /** Rich overview paragraph describing the tool's purpose and behavior. */
  overview?: string;
  /** Per-parameter documentation keyed by parameter path. */
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
