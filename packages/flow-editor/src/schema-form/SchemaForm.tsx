import { useState, useCallback, useMemo } from "react";
import type { PropertySchema } from "./types";
import type { SchemaFormProps } from "./types";
import { theme } from "./utils";
import { PropertyField } from "./PropertyField";
import { FieldGroup } from "./FieldGroup";
import { RetryPolicySection } from "./widgets/RetryPolicy";
import { useSchemaToZod } from "./hooks/useSchemaToZod";
import { useValidation } from "./hooks/useValidation";

/**
 * Schema-driven configuration form.
 * Auto-generates form fields from a ComponentSchema, respecting groups,
 * types, defaults, enums, validation constraints, nested objects, arrays,
 * dynamic maps, and x-widget hints.
 */
export function SchemaForm({ schema, values, onChange, compact = false, presetValues, paramDocs }: SchemaFormProps) {
  const defs = schema.$defs;

  // Zod validation: convert schema properties to Zod and validate values
  const zodSchema = useSchemaToZod(schema.properties);
  const fieldErrors = useValidation(zodSchema, values);

  const { properties, groups, ungrouped } = useMemo(() => {
    const props = schema.properties || {};
    const grps = schema["ui:groups"] || [];
    const grouped = new Set(grps.flatMap((g) => g.fields));
    const ungrp = Object.keys(props)
      .filter((k) => !grouped.has(k) && !props[k].deprecated)
      .sort((a, b) => {
        const orderA = props[a]?.["ui:order"] ?? Infinity;
        const orderB = props[b]?.["ui:order"] ?? Infinity;
        return orderA - orderB;
      });
    return { properties: props, groups: grps, ungrouped: ungrp };
  }, [schema]);

  const handleChange = useCallback(
    (key: string, value: unknown) => {
      onChange({ ...values, [key]: value });
    },
    [values, onChange],
  );

  // Drill-down navigation for deeply nested objects (depth >= 2)
  const [drillInto, setDrillInto] = useState<{ key: string; label: string } | null>(null);

  const handleDrillDown = useCallback(
    (label: string, key: string, _schema: PropertySchema, _values: Record<string, unknown>) => {
      setDrillInto({ key, label });
    },
    [],
  );

  // When drilled into a nested object, render only that object's fields
  if (drillInto) {
    // Walk the dot-separated key path to find the schema and value
    const keyParts = drillInto.key.split(".");
    let targetSchema: PropertySchema | undefined;
    let targetValue: Record<string, unknown> = values;
    let parentSchema: Record<string, PropertySchema> = properties;

    for (const part of keyParts) {
      targetSchema = parentSchema[part];
      if (!targetSchema) break;
      targetValue = (targetValue[part] as Record<string, unknown>) ?? {};
      parentSchema = targetSchema.properties || {};
    }

    if (!targetSchema?.properties) {
      // Fallback: reset drill if schema not found
      setDrillInto(null);
      return null;
    }

    const drillProperties = targetSchema.properties;
    const drillKeys = Object.keys(drillProperties).filter((k) => !drillProperties[k].deprecated);

    const handleDrillFieldChange = (fieldKey: string, fieldValue: unknown) => {
      // Rebuild nested value back to root
      const newDrillValue = { ...targetValue, [fieldKey]: fieldValue };
      // Walk backwards through key parts to reconstruct
      let result: Record<string, unknown> = newDrillValue;
      for (let i = keyParts.length - 1; i >= 0; i--) {
        const parentVal = i === 0 ? values : (() => {
          let v: Record<string, unknown> = values;
          for (let j = 0; j < i; j++) {
            v = (v[keyParts[j]] as Record<string, unknown>) ?? {};
          }
          return v;
        })();
        result = { ...parentVal, [keyParts[i]]: result };
      }
      onChange(result);
    };

    return (
      <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>
        {/* Breadcrumb */}
        <div style={{ display: "flex", alignItems: "center", gap: 4, flexWrap: "wrap", marginBottom: 10 }}>
          <button
            onClick={() => setDrillInto(null)}
            style={{
              background: "none",
              border: "none",
              cursor: "pointer",
              padding: 0,
              fontSize: 10,
              color: theme.fgMuted,
            }}
          >
            Root
          </button>
          <span style={{ fontSize: 9, color: theme.fgMuted }}>&rsaquo;</span>
          <span
            style={{
              fontSize: 10,
              color: theme.fg,
              fontWeight: 600,
            }}
          >
            {drillInto.label}
          </span>
        </div>

        {/* Drilled-into object's fields at depth 0 */}
        {targetSchema.description && (
          <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 6 }}>
            {targetSchema.description}
          </div>
        )}
        <div style={{ display: "flex", flexDirection: "column", gap: compact ? 2 : 6 }}>
          {drillKeys.map((key) => (
            <PropertyField
              key={key}
              name={key}
              schema={drillProperties[key]}
              value={targetValue[key]}
              onChange={(v) => handleDrillFieldChange(key, v)}
              compact={compact}
              allValues={targetValue}
              allProperties={drillProperties}
              depth={0}
              onDrillDown={handleDrillDown}
              defs={defs}
            />
          ))}
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 0 }}>
      {/* Grouped fields */}
      {groups.map((group, groupIndex) => (
        <FieldGroup
          key={group.id}
          group={group}
          groupIndex={groupIndex}
          properties={properties}
          values={values}
          onChange={handleChange}
          compact={compact}
          onDrillDown={handleDrillDown}
          presetValues={presetValues}
          paramDocs={paramDocs}
          defs={defs}
          fieldErrors={fieldErrors}
        />
      ))}

      {/* Ungrouped fields */}
      {ungrouped.length > 0 && (
        <div style={{ marginTop: groups.length > 0 ? 20 : 0 }}>
          {groups.length > 0 && ungrouped.length > 0 && (
            <div
              style={{
                fontSize: 11,
                fontWeight: 700,
                color: theme.fgMuted,
                textTransform: "uppercase",
                letterSpacing: "0.06em",
                borderBottom: `1px solid ${theme.border}`,
                paddingBottom: 6,
                marginBottom: 10,
              }}
            >
              Other
            </div>
          )}
          <div style={{ display: "flex", flexDirection: "column", gap: compact ? 2 : 6 }}>
            {ungrouped.map((key) => (
              <PropertyField
                key={key}
                name={key}
                schema={properties[key]}
                value={values[key]}
                onChange={(v) => handleChange(key, v)}
                compact={compact}
                allValues={values}
                allProperties={properties}
                onDrillDown={handleDrillDown}
                presetValues={presetValues}
                docParam={paramDocs?.[key]}
                defs={defs}
                error={fieldErrors[key]}
              />
            ))}
          </div>
        </div>
      )}

      {/* Retry Policy section — only for tools that opt in via requires: ["retryable"] */}
      {schema.toolMeta?.requires?.includes("retryable") && (
        <RetryPolicySection values={values} onChange={onChange} compact={compact} />
      )}
    </div>
  );
}
