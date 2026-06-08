/**
 * Filter (format) config editor types.
 *
 * The schema language (ComponentSchema, PropertySchema, ToolMeta, IOPort, …) is
 * shared across all consumers and lives in @neokapi/contract-types, generated
 * from Go where it maps 1:1 (issue #817). Re-export it here; only the
 * filter-component-specific types (FormatSchema, CodeFinderRulesValue,
 * FormatParamsValue) are local.
 */

import type { ComponentSchema, FormatMeta } from "@neokapi/contract-types";

export type {
  ComponentSchema,
  FormatMeta,
  IOPort,
  ToolMeta,
  ParameterGroup,
  ConditionExpr,
  PropertySchema,
} from "@neokapi/contract-types";

/** Format-specific schema — a ComponentSchema with required format metadata. */
export interface FormatSchema extends ComponentSchema {
  formatMeta: FormatMeta;
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
