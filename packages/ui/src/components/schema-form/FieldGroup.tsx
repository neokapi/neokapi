import { cn } from "../../lib/utils";
import { FormFieldGroup } from "../ui/form";
import type { ParameterGroup, PropertySchema, ToolDocParam } from "./types";
import { PropertyField } from "./PropertyField";
import { evaluateCondition } from "./hooks/useConditionalVisibility";

export function FieldGroup({
  group,
  groupIndex,
  properties,
  values,
  onChange,
  compact,
  onDrillDown,
  presetValues,
  paramDocs,
  fieldErrors,
}: {
  group: ParameterGroup;
  groupIndex: number;
  properties: Record<string, PropertySchema>;
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  compact?: boolean;
  onDrillDown?: (
    label: string,
    key: string,
    schema: PropertySchema,
    values: Record<string, unknown>,
  ) => void;
  presetValues?: Record<string, unknown>;
  paramDocs?: Record<string, ToolDocParam>;
  fieldErrors?: Record<string, string | undefined>;
}) {
  // Drop fields that are absent, deprecated, or hidden by their ui:visible
  // condition. Hiding when none remain keeps a conditionally-populated group
  // (e.g. a backend's parameters that only apply when that backend is selected)
  // from rendering an empty header.
  const fields = group.fields.filter(
    (f) =>
      properties[f] &&
      !properties[f].deprecated &&
      evaluateCondition(properties[f]["ui:visible"], values, properties),
  );
  if (fields.length === 0) return null;

  const sortedFields = [...fields].sort((a, b) => {
    const orderA = properties[a]?.["ui:order"] ?? Infinity;
    const orderB = properties[b]?.["ui:order"] ?? Infinity;
    return orderA - orderB;
  });

  const isCollapsible = group.collapsible ?? fields.length > 4;
  const defaultCollapsed = isCollapsible ? (group.collapsed ?? groupIndex >= 2) : false;

  const maxColumns = Math.max(
    1,
    ...sortedFields.map((k) => properties[k]?.["ui:layout"]?.columns ?? 1),
  );
  const useGrid = maxColumns > 1;

  return (
    <FormFieldGroup
      label={group.label}
      description={group.description}
      collapsible={isCollapsible}
      defaultCollapsed={defaultCollapsed}
      className={cn(groupIndex > 0 && "mt-5")}
    >
      <div
        className={cn(useGrid ? "grid gap-1.5" : "flex flex-col gap-1.5", compact && "gap-0.5")}
        style={useGrid ? { gridTemplateColumns: `repeat(${maxColumns}, 1fr)` } : undefined}
      >
        {sortedFields.map((key) => {
          const columns = properties[key]?.["ui:layout"]?.columns ?? 1;
          return (
            <div
              key={key}
              style={useGrid && columns > 1 ? { gridColumn: `span ${columns}` } : undefined}
            >
              <PropertyField
                name={key}
                schema={properties[key]}
                value={values[key]}
                onChange={(v) => onChange(key, v)}
                compact={compact}
                allValues={values}
                allProperties={properties}
                onDrillDown={onDrillDown}
                presetValues={presetValues}
                docParam={paramDocs?.[key]}
                error={fieldErrors?.[key]}
              />
            </div>
          );
        })}
      </div>
    </FormFieldGroup>
  );
}
