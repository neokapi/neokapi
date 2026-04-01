import { useState, useCallback } from "react";
import {
  ChevronDown,
  ChevronRight,
  X,
} from "lucide-react";
import type { PropertySchema } from "../types";
import { theme, inputStyle, removeButtonStyle } from "../utils";
import { PropertyField } from "../PropertyField";
import { InferredObjectEditor, JsonInlineEditor } from "./JsonEditor";

export function MapEditor({
  label,
  description,
  value,
  itemSchema,
  onChange,
  compact,
  depth,
  keyPlaceholder = "key",
}: {
  label: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  itemSchema?: PropertySchema;
  onChange: (value: unknown) => void;
  compact: boolean;
  depth: number;
  keyPlaceholder?: string;
}) {
  const [newKey, setNewKey] = useState("");
  const current = value ?? {};
  const entries = Object.entries(current);

  const handleAdd = useCallback(() => {
    const key = newKey.trim();
    if (!key || key in current) return;
    const defaultVal = itemSchema?.type === "object" ? {} : itemSchema?.type === "array" ? [] : "";
    onChange({ ...current, [key]: defaultVal });
    setNewKey("");
  }, [newKey, current, itemSchema, onChange]);

  const handleRemove = useCallback(
    (key: string) => {
      const next = { ...current };
      delete next[key];
      onChange(next);
    },
    [current, onChange],
  );

  const handleEntryChange = useCallback(
    (key: string, entryValue: unknown) => {
      onChange({ ...current, [key]: entryValue });
    },
    [current, onChange],
  );

  return (
    <div>
      {/* Sub-label */}
      <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>{label}</div>
      {description && (
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 6 }}>
          {description}
        </div>
      )}

      {/* Entries */}
      {entries.map(([key, val]) => (
        <MapEntry
          key={key}
          entryKey={key}
          value={val}
          itemSchema={itemSchema}
          onChange={(v) => handleEntryChange(key, v)}
          onRemove={() => handleRemove(key)}
          compact={compact}
          depth={depth}
        />
      ))}

      {/* Add new entry */}
      <div
        style={{
          display: "flex",
          gap: 4,
          padding: "4px 0",
          alignItems: "center",
        }}
      >
        <input
          type="text"
          value={newKey}
          placeholder={keyPlaceholder}
          onChange={(e) => setNewKey(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleAdd();
          }}
          style={{
            ...inputStyle(compact),
            flex: 1,
            fontSize: 10,
          }}
        />
        <button
          onClick={handleAdd}
          disabled={!newKey.trim() || newKey.trim() in current}
          style={{
            background: "none",
            border: "none",
            cursor: newKey.trim() && !(newKey.trim() in current) ? "pointer" : "not-allowed",
            padding: 0,
            fontSize: 10,
            color: theme.accent,
            fontWeight: 600,
            opacity: newKey.trim() && !(newKey.trim() in current) ? 1 : 0.4,
            whiteSpace: "nowrap",
          }}
        >
          + Add {keyPlaceholder}
        </button>
      </div>
    </div>
  );
}

export function MapEntry({
  entryKey,
  value,
  itemSchema,
  onChange,
  onRemove,
  compact,
  depth,
}: {
  entryKey: string;
  value: unknown;
  itemSchema?: PropertySchema;
  onChange: (value: unknown) => void;
  onRemove: () => void;
  compact: boolean;
  depth: number;
}) {
  const [expanded, setExpanded] = useState(false);
  const isComplex =
    itemSchema?.properties || itemSchema?.type === "object" || itemSchema?.type === "array";

  // Also treat values that are actually objects/arrays as complex,
  // even if the schema doesn't declare additionalProperties.
  const valueIsObject = value !== null && typeof value === "object" && !Array.isArray(value);
  const valueIsArray = Array.isArray(value);
  const effectivelyComplex = isComplex || valueIsObject || valueIsArray;

  // Simple value — horizontal row with text input
  if (!effectivelyComplex) {
    return (
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 6,
          borderBottom: `1px solid ${theme.border}`,
          padding: "6px 0",
        }}
      >
        <span
          style={{
            fontSize: 11,
            fontWeight: 600,
            color: theme.accent,
            fontFamily: "var(--font-mono, ui-monospace, monospace)",
            minWidth: 60,
            flexShrink: 0,
          }}
        >
          {entryKey}
        </span>
        <input
          type="text"
          value={String(value ?? "")}
          onChange={(e) => onChange(e.target.value || undefined)}
          style={{ ...inputStyle(compact), flex: 1, fontSize: 10 }}
        />
        <button onClick={onRemove} style={{ ...removeButtonStyle, opacity: 0.5 }}>
          <X size={10} />
        </button>
      </div>
    );
  }

  // Complex value — expandable
  return (
    <div style={{ borderBottom: `1px solid ${theme.border}`, padding: "6px 0" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: 6,
        }}
      >
        <button
          onClick={() => setExpanded(!expanded)}
          style={{
            display: "flex",
            alignItems: "center",
            gap: 4,
            flex: 1,
            background: "none",
            border: "none",
            cursor: "pointer",
            padding: 0,
            textAlign: "left",
          }}
        >
          {expanded ? (
            <ChevronDown size={10} style={{ color: theme.fgMuted }} />
          ) : (
            <ChevronRight size={10} style={{ color: theme.fgMuted }} />
          )}
          <span
            style={{
              fontSize: 11,
              fontWeight: 600,
              color: theme.accent,
              fontFamily: "var(--font-mono, ui-monospace, monospace)",
            }}
          >
            {entryKey}
          </span>
        </button>
        <button onClick={onRemove} style={{ ...removeButtonStyle, opacity: 0.5 }}>
          <X size={10} />
        </button>
      </div>

      {expanded && (
        <div
          style={{
            paddingLeft: 16,
            paddingTop: 6,
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 4,
          }}
        >
          {itemSchema?.properties ? (
            Object.entries(itemSchema.properties)
              .filter(([, s]) => !s.deprecated)
              .map(([key, fieldSchema]) => (
                <PropertyField
                  key={key}
                  name={key}
                  schema={fieldSchema}
                  value={(value as Record<string, unknown>)?.[key]}
                  onChange={(v) => onChange({ ...(value as Record<string, unknown>), [key]: v })}
                  compact={compact}
                  allValues={value as Record<string, unknown>}
                  depth={depth + 1}
                />
              ))
          ) : valueIsObject ? (
            <InferredObjectEditor
              value={value as Record<string, unknown>}
              onChange={onChange}
              compact={compact}
            />
          ) : (
            <JsonInlineEditor value={value} onChange={onChange} />
          )}
        </div>
      )}
    </div>
  );
}
