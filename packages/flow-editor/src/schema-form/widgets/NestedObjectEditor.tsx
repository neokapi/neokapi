import { useCallback } from "react";
import { ChevronRight } from "lucide-react";
import type { PropertySchema } from "../types";
import { theme } from "../utils";
import { PropertyField } from "../PropertyField";

export function NestedObjectEditor({
  label,
  description,
  schema,
  value,
  onChange,
  compact,
  depth,
  name,
  onDrillDown,
  defs,
  disabled = false,
}: {
  label: string;
  description?: string;
  schema: PropertySchema;
  value: Record<string, unknown> | undefined;
  onChange: (value: unknown) => void;
  compact: boolean;
  depth: number;
  name: string;
  onDrillDown?: (label: string, key: string, schema: PropertySchema, values: Record<string, unknown>) => void;
  defs?: Record<string, PropertySchema>;
  disabled?: boolean;
}) {
  const current = value ?? {};
  const properties = schema.properties || {};
  const keys = Object.keys(properties).filter((k) => !properties[k].deprecated);

  const handleFieldChange = useCallback(
    (key: string, fieldValue: unknown) => {
      onChange({ ...current, [key]: fieldValue });
    },
    [current, onChange],
  );

  // At depth >= 2, render as a drill-down row
  if (depth >= 2 && onDrillDown) {
    return (
      <button
        onClick={() => onDrillDown(label, name, schema, current)}
        style={{
          display: "flex",
          alignItems: "center",
          width: "100%",
          padding: "6px 8px",
          borderRadius: 4,
          border: `1px solid ${theme.border}`,
          background: theme.bgCard,
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        <span style={{ flex: 1, fontSize: 11, color: theme.fg }}>{label}</span>
        <span style={{ fontSize: 10, color: theme.fgMuted }}>{keys.length} fields</span>
        <ChevronRight size={12} style={{ color: theme.fgMuted, marginLeft: 4 }} />
      </button>
    );
  }

  // Depth 0: render flat, no label (the parent group is the label)
  // Depth 1: render flat, show a subtle sub-label if informative
  return (
    <div style={disabled ? { opacity: 0.4, pointerEvents: "none" } : undefined}>
      {depth === 1 && (
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 4 }}>
          {label}
        </div>
      )}
      {description && depth > 0 && (
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 4 }}>{description}</div>
      )}
      <div
        style={{
          display: "flex",
          flexDirection: "column",
          gap: compact ? 2 : 6,
        }}
      >
        {keys.map((key) => (
          <PropertyField
            key={key}
            name={key}
            schema={properties[key]}
            value={current[key]}
            onChange={(v) => handleFieldChange(key, v)}
            compact={compact}
            allValues={current}
            allProperties={properties}
            depth={depth + 1}
            onDrillDown={onDrillDown}
            defs={defs}
          />
        ))}
      </div>
    </div>
  );
}
