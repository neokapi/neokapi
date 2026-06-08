// Schema-form types.
//
// The schema language (ComponentSchema, PropertySchema, ToolMeta, IOPort, …) is
// defined once in the shared @neokapi/contract-types package, generated from the
// Go source of truth where it maps 1:1 (issue #817). Re-export it so existing
// `@neokapi/ui-primitives` importers keep resolving these names here; only the
// component-specific `SchemaFormProps` is local.

import type { SchemaFormHost } from "./host";
import type { ComponentSchema, ToolDocParam } from "@neokapi/contract-types";

export type {
  ComponentSchema,
  FormatMeta,
  IOPort,
  Side,
  ToolMeta,
  ParameterGroup,
  ConditionExpr,
  LayoutHints,
  PropertySchema,
  ToolDoc,
  ToolDocParam,
} from "@neokapi/contract-types";

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
