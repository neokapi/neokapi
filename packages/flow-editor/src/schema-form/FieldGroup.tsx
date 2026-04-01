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
}) {
  const fields = group.fields.filter((f) => properties[f] && !properties[f].deprecated);
  if (fields.length === 0) return null;

  // Groups with <= 4 fields: always open, no collapse
  // Groups with 5+ fields: collapsible, first 2 groups default open, rest collapsed
  const isSmallGroup = fields.length <= 4;
  const defaultCollapsed = isSmallGroup ? false : (group.collapsed ?? groupIndex >= 2);
  const [collapsed, setCollapsed] = useState(defaultCollapsed);

  return (
    <div style={{ marginTop: groupIndex === 0 ? 0 : 20 }}>
      {/* Section header */}
      {isSmallGroup ? (
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
          {fields.map((key) => (
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
            />
          ))}
        </div>
      )}
    </div>
  );
}
