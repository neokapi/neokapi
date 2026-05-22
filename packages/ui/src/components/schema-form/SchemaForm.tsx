import { useState, useCallback, useMemo } from "react";
import { cn } from "../../lib/utils";
import type { ParameterGroup, PropertySchema, SchemaFormProps } from "./types";
import { PropertyField } from "./PropertyField";
import { FieldGroup } from "./FieldGroup";
import { RetryPolicySection } from "./widgets/RetryPolicy";
import { SchemaFormHostProvider } from "./host";

export function SchemaForm({
  schema,
  values,
  onChange,
  compact = false,
  presetValues,
  paramDocs,
  readOnly,
  hideHeader = false,
  host,
}: SchemaFormProps) {
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
      if (readOnly) return;
      onChange({ ...values, [key]: value });
    },
    [values, onChange, readOnly],
  );

  // Drill-down navigation for deeply nested objects
  const [drillInto, setDrillInto] = useState<{ key: string; label: string } | null>(null);

  const handleDrillDown = useCallback(
    (label: string, key: string, _schema: PropertySchema, _values: Record<string, unknown>) => {
      setDrillInto({ key, label });
    },
    [],
  );

  if (drillInto) {
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
      setDrillInto(null);
      return null;
    }

    const drillProperties = targetSchema.properties;
    const drillKeys = Object.keys(drillProperties).filter((k) => !drillProperties[k].deprecated);

    const handleDrillFieldChange = (fieldKey: string, fieldValue: unknown) => {
      const newDrillValue = { ...targetValue, [fieldKey]: fieldValue };
      let result: Record<string, unknown> = newDrillValue;
      for (let i = keyParts.length - 1; i >= 0; i--) {
        const parentVal =
          i === 0
            ? values
            : (() => {
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
      <SchemaFormHostProvider host={host}>
        <div className="flex flex-col">
          {/* Breadcrumb */}
          <div className="flex items-center gap-1 flex-wrap mb-2.5">
            <button
              type="button"
              onClick={() => setDrillInto(null)}
              className="text-xs text-muted-foreground hover:text-foreground"
            >
              Root
            </button>
            <span className="text-xs text-muted-foreground">&rsaquo;</span>
            <span className="text-xs font-semibold">{drillInto.label}</span>
          </div>

          {targetSchema.description && (
            <p className="text-xs text-muted-foreground mb-1.5">{targetSchema.description}</p>
          )}

          <div className={cn("flex flex-col", compact ? "gap-0.5" : "gap-1.5")}>
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
              />
            ))}
          </div>
        </div>
      </SchemaFormHostProvider>
    );
  }

  const formatMeta = schema.formatMeta;
  const toolMeta = schema.toolMeta;

  return (
    <SchemaFormHostProvider host={host}>
      <div className="flex flex-col">
        {/* Format/tool header */}
        {!hideHeader && (schema.title || formatMeta || toolMeta) && (
          <div className="pb-3 mb-3 border-b border-border/40">
            {schema.title && (
              <h3 className="text-sm font-semibold text-foreground">{schema.title}</h3>
            )}
            {schema.description && (
              <p className="mt-1 text-xs text-muted-foreground">{schema.description}</p>
            )}
            {formatMeta && (formatMeta.extensions?.length || formatMeta.mimeTypes?.length) && (
              <div className="mt-2 flex flex-wrap gap-1.5">
                {formatMeta.extensions?.map((ext: string) => (
                  <span
                    key={ext}
                    className="rounded bg-secondary px-1.5 py-0.5 text-[10px] font-medium text-secondary-foreground"
                  >
                    {ext}
                  </span>
                ))}
                {formatMeta.mimeTypes?.slice(0, 2).map((mt: string) => (
                  <span
                    key={mt}
                    className="rounded bg-accent px-1.5 py-0.5 text-[10px] text-muted-foreground"
                  >
                    {mt}
                  </span>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Grouped fields */}
        {groups.map((group: ParameterGroup, groupIndex: number) => (
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
          />
        ))}

        {/* Ungrouped fields */}
        {ungrouped.length > 0 && (
          <div className={cn(groups.length > 0 && "mt-5")}>
            {groups.length > 0 && ungrouped.length > 0 && (
              <div className="flex items-center pb-1.5 mb-2.5 border-b">
                <span className="text-xs font-bold text-muted-foreground uppercase tracking-wider">
                  Other
                </span>
              </div>
            )}
            <div className={cn("flex flex-col", compact ? "gap-0.5" : "gap-1.5")}>
              {ungrouped.map((key: string) => (
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
                />
              ))}
            </div>
          </div>
        )}

        {/* Retry Policy */}
        {schema.toolMeta?.requires?.includes("retryable") && (
          <RetryPolicySection values={values} onChange={onChange} compact={compact} />
        )}
      </div>
    </SchemaFormHostProvider>
  );
}
