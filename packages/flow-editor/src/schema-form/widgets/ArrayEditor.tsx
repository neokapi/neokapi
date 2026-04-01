import { useCallback } from "react";
import { X } from "lucide-react";
import type { PropertySchema } from "../types";
import { theme, inputStyle, removeButtonStyle } from "../utils";
import { FieldWrapper } from "../primitives/FieldWrapper";
import { PropertyField } from "../PropertyField";
import { JsonInlineEditor } from "./JsonEditor";

export function ArrayEditor({
  label,
  description,
  itemSchema,
  value,
  onChange,
  compact,
  depth,
}: {
  label: string;
  description?: string;
  itemSchema: PropertySchema;
  value: unknown[] | undefined;
  onChange: (value: unknown) => void;
  compact: boolean;
  depth: number;
}) {
  const items = value ?? [];
  const isSimple =
    itemSchema.type === "string" || itemSchema.type === "number" || itemSchema.type === "integer";

  const handleAdd = useCallback(() => {
    const defaultVal =
      itemSchema.type === "object"
        ? {}
        : itemSchema.type === "array"
          ? []
          : itemSchema.type === "boolean"
            ? false
            : itemSchema.type === "number" || itemSchema.type === "integer"
              ? 0
              : "";
    onChange([...items, defaultVal]);
  }, [items, itemSchema, onChange]);

  const handleRemove = useCallback(
    (index: number) => {
      onChange(items.filter((_, i) => i !== index));
    },
    [items, onChange],
  );

  const handleItemChange = useCallback(
    (index: number, itemValue: unknown) => {
      const next = [...items];
      next[index] = itemValue;
      onChange(next);
    },
    [items, onChange],
  );

  const handleMove = useCallback(
    (index: number, direction: -1 | 1) => {
      const target = index + direction;
      if (target < 0 || target >= items.length) return;
      const next = [...items];
      [next[index], next[target]] = [next[target], next[index]];
      onChange(next);
    },
    [items, onChange],
  );

  // Simple arrays render as horizontal pill row
  if (isSimple) {
    return (
      <FieldWrapper label={label} description={description} compact={compact}>
        <div style={{ display: "flex", flexWrap: "wrap", gap: 4, alignItems: "center" }}>
          {items.map((item, i) => (
            <span key={i} style={{
              display: "inline-flex",
              alignItems: "center",
              gap: 3,
              padding: "2px 6px",
              borderRadius: 10,
              border: `1px solid ${theme.border}`,
              fontSize: 10,
              fontFamily: "var(--font-mono, ui-monospace, monospace)",
              color: theme.fg,
              background: theme.bgCard,
            }}>
              {String(item)}
              <button onClick={() => handleRemove(i)} style={{ background: "none", border: "none", cursor: "pointer", padding: 0, display: "flex" }}>
                <X size={8} style={{ color: theme.fgMuted }} />
              </button>
            </span>
          ))}
          <input
            placeholder="+ add"
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.target as HTMLInputElement).value.trim()) {
                const val = (e.target as HTMLInputElement).value.trim();
                onChange([...items, itemSchema.type === "number" || itemSchema.type === "integer" ? Number(val) : val]);
                (e.target as HTMLInputElement).value = "";
              }
            }}
            style={{
              border: "none",
              background: "transparent",
              fontSize: 10,
              width: 60,
              color: theme.fgMuted,
              outline: "none",
              fontFamily: "var(--font-mono, ui-monospace, monospace)",
            }}
          />
        </div>
      </FieldWrapper>
    );
  }

  // Complex arrays — flat section sub-label + list
  return (
    <div>
      <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>{label}</div>
      {description && (
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 6 }}>
          {description}
        </div>
      )}

      {items.map((item, index) => (
        <div
          key={index}
          style={{
            display: "flex",
            gap: 4,
            padding: "4px 0",
            alignItems: "stretch",
            borderBottom: `1px solid ${theme.border}`,
          }}
        >
          {/* Index */}
          <div
            style={{
              display: "flex",
              alignItems: "center",
              color: theme.fgMuted,
              flexShrink: 0,
            }}
          >
            <span style={{ fontSize: 9, fontWeight: 600, minWidth: 12, textAlign: "center" }}>
              {index + 1}
            </span>
          </div>

          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              gap: 2,
            }}
          >
            {itemSchema.properties ? (
              Object.entries(itemSchema.properties)
                .filter(([, s]) => !s.deprecated)
                .map(([key, fieldSchema]) => (
                  <PropertyField
                    key={key}
                    name={key}
                    schema={fieldSchema}
                    value={(item as Record<string, unknown>)?.[key]}
                    onChange={(v) =>
                      handleItemChange(index, {
                        ...(item as Record<string, unknown>),
                        [key]: v,
                      })
                    }
                    compact
                    allValues={item as Record<string, unknown>}
                    depth={depth + 1}
                  />
                ))
            ) : (
              <JsonInlineEditor
                value={item}
                onChange={(v) => handleItemChange(index, v)}
                compact={compact}
              />
            )}
          </div>

          <button
            onClick={() => handleRemove(index)}
            style={{
              ...removeButtonStyle,
              alignSelf: "flex-start",
              marginTop: 3,
              opacity: 0.5,
            }}
          >
            <X size={10} />
          </button>
        </div>
      ))}

      {/* Add item button */}
      <button
        onClick={handleAdd}
        style={{
          background: "none",
          border: "none",
          cursor: "pointer",
          padding: "6px 0",
          fontSize: 10,
          color: theme.accent,
          fontWeight: 600,
        }}
      >
        + Add item
      </button>
    </div>
  );
}
