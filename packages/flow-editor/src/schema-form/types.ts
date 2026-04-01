export type {
  ComponentSchema,
  PropertySchema,
  ParameterGroup,
  ToolDocParam,
  ConditionExpr,
} from "../types";

export interface SchemaFormProps {
  schema: import("../types").ComponentSchema;
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact?: boolean;
  /** When provided, fields whose value differs from the preset show a colored indicator dot. */
  presetValues?: Record<string, unknown>;
  /** Rich parameter documentation keyed by parameter path (e.g. "extraction.extractAll"). */
  paramDocs?: Record<string, import("../types").ToolDocParam>;
}
