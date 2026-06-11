import { useState, useCallback, useMemo } from "react";
import { cn } from "../../lib/utils";
import { ComponentSchema, FormatSchema, FormatParamsValue } from "./types";

import { ParameterGroupSection } from "./ParameterGroupSection";
import { ParameterField } from "./ParameterField";

interface FilterConfigEditorProps {
  /** The filter or tool schema */
  schema: FormatSchema | ComponentSchema;
  /** Current parameter values */
  value: FormatParamsValue;
  /** Called when any parameter changes */
  onChange: (params: FormatParamsValue) => void;
  /** Optional CSS class */
  className?: string;
}

/**
 * FilterConfigEditor renders a dynamic form for filter or tool parameters
 * based on the JSON Schema with x-groups, x-widget, and x-showIf extensions.
 *
 * Supports: primitives, nested objects, dynamic maps, arrays, code finder
 * rules, simplifier rules, element/attribute rules editors, and JSON fallback.
 *
 * Also exported as SchemaConfigEditor for tool use cases.
 */
export function FilterConfigEditor({
  schema,
  value,
  onChange,
  className,
}: FilterConfigEditorProps) {
  const groups = schema["ui:groups"] ?? [];
  const properties = schema.properties ?? {};

  // Track which groups are collapsed
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    groups.forEach((g) => {
      if (g.collapsed) initial.add(g.id);
    });
    return initial;
  });

  const toggleGroup = useCallback((groupId: string) => {
    setCollapsedGroups((prev: Set<string>) => {
      const next = new Set(prev);
      if (next.has(groupId)) {
        next.delete(groupId);
      } else {
        next.add(groupId);
      }
      return next;
    });
  }, []);

  // Identify ungrouped fields
  const groupedFields = useMemo(() => {
    const set = new Set<string>();
    groups.forEach((g) => g.fields.forEach((f) => set.add(f)));
    return set;
  }, [groups]);

  const ungroupedFields = useMemo(() => {
    return Object.keys(properties).filter(
      (f) => !groupedFields.has(f) && !properties[f].deprecated,
    );
  }, [properties, groupedFields]);

  const handleChange = useCallback(
    (fieldName: string, fieldValue: unknown) => {
      onChange({ ...value, [fieldName]: fieldValue });
    },
    [value, onChange],
  );

  return (
    <div className={cn("space-y-4", className)}>
      {/* Grouped parameters */}
      {groups.map((group) => (
        <ParameterGroupSection
          key={group.id}
          group={group}
          properties={properties}
          values={value}
          collapsed={collapsedGroups.has(group.id)}
          onToggle={() => toggleGroup(group.id)}
          onChange={handleChange}
        />
      ))}

      {/* Ungrouped parameters */}
      {ungroupedFields.length > 0 && (
        <div className="space-y-3">
          {groups.length > 0 && (
            <h3 className="text-sm font-medium text-muted-foreground">Other Parameters</h3>
          )}
          {ungroupedFields.map((fieldName: string) => (
            <ParameterField
              key={fieldName}
              name={fieldName}
              schema={properties[fieldName]}
              value={value[fieldName]}
              onChange={(v) => handleChange(fieldName, v)}
              allValues={value}
            />
          ))}
        </div>
      )}
    </div>
  );
}

/** Alias for tool/step config editing use cases. */
export const SchemaConfigEditor = FilterConfigEditor;

export default FilterConfigEditor;
