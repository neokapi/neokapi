import { useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import type { ParameterGroup, PropertySchema, ToolDocParam } from "./types";
import { theme } from "./utils";
import { PropertyField } from "./PropertyField";

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
  defs,
  fieldErrors,
}: {
  group: ParameterGroup;
  groupIndex: number;
  properties: Record<string, PropertySchema>;
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  compact: boolean;
  onDrillDown?: (label: string, key: string, schema: PropertySchema, values: Record<string, unknown>) => void;
  presetValues?: Record<string, unknown>;
  paramDocs?: Record<string, ToolDocParam>;
  defs?: Record<string, PropertySchema>;
  fieldErrors?: Record<string, string | undefined>;
}) {
  const fields = group.fields.filter((f) => properties[f] && !properties[f].deprecated);
  if (fields.length === 0) return null;

  // Sort fields by ui:order if specified
  const sortedFields = [...fields].sort((a, b) => {
    const orderA = properties[a]?.["ui:order"] ?? Infinity;
    const orderB = properties[b]?.["ui:order"] ?? Infinity;
    return orderA - orderB;
  });

  // Use explicit collapsible flag, or fall back to heuristic (5+ fields)
  const isCollapsible = group.collapsible ?? fields.length > 4;
  const defaultCollapsed = isCollapsible ? (group.collapsed ?? groupIndex >= 2) : false;
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <div style={{ marginTop: groupIndex === 0 ? 0 : 20 }}>
      {/* Section header */}
      {!isCollapsible ? (
        /* Non-collapsible header */
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
          {group.label}
        </div>
      ) : (
        /* Collapsible header */
        <button
          onClick={() => setCollapsed(!collapsed)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 4,
            width: "100%",
            padding: 0,
            paddingBottom: 6,
            marginBottom: collapsed ? 0 : 10,
            background: "none",
            border: "none",
            borderBottom: `1px solid ${theme.border}`,
            cursor: "pointer",
            textAlign: "left",
          }}
        >
          {collapsed ? (
            <ChevronRight size={11} style={{ color: theme.fgMuted }} />
          ) : (
            <ChevronDown size={11} style={{ color: theme.fgMuted }} />
          )}
          <span
            style={{
              fontSize: 11,
              fontWeight: 700,
              color: theme.fgMuted,
              textTransform: "uppercase",
              letterSpacing: "0.06em",
            }}
          >
            {group.label}
          </span>
        </button>
      )}
      {!collapsed && (
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 6,
          }}
        >
          {sortedFields.map((key) => (
            <PropertyField
              key={key}
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
              defs={defs}
              error={fieldErrors?.[key]}
            />
          ))}
        </div>
      )}
    </div>
  );
}
