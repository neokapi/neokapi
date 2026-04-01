import { useState, useCallback } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { theme, formatLabel } from "../utils";

export function JsonEditor({
  label,
  description,
  value,
  onChange,
}: {
  label: string;
  description?: string;
  value: unknown;
  onChange: (value: unknown) => void;
}) {
  const [collapsed, setCollapsed] = useState(true);
  const [text, setText] = useState(() =>
    JSON.stringify(value ?? (Array.isArray(value) ? [] : {}), null, 2),
  );
  const [error, setError] = useState<string | null>(null);

  const handleBlur = useCallback(() => {
    try {
      const parsed = JSON.parse(text);
      setError(null);
      onChange(parsed);
    } catch {
      setError("Invalid JSON");
    }
  }, [text, onChange]);

  return (
    <div>
      <button
        onClick={() => setCollapsed(!collapsed)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 4,
          width: "100%",
          padding: 0,
          paddingBottom: 4,
          background: "none",
          border: "none",
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        {collapsed ? (
          <ChevronRight size={11} style={{ color: theme.fgMuted }} />
        ) : (
          <ChevronDown size={11} style={{ color: theme.fgMuted }} />
        )}
        <span style={{ fontSize: 11, fontWeight: 500, color: theme.fgSecondary, flex: 1 }}>{label}</span>
        {error && <span style={{ fontSize: 9, color: theme.destructive }}>{error}</span>}
      </button>

      {!collapsed && (
        <div>
          {description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 4 }}>{description}</div>
          )}
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            onBlur={handleBlur}
            style={{
              width: "100%",
              minHeight: 80,
              maxHeight: 200,
              padding: "6px 8px",
              fontSize: 10,
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
              lineHeight: 1.5,
              borderRadius: 4,
              border: `1px solid ${error ? theme.destructive : theme.border}`,
              background: theme.bgCard,
              color: theme.fg,
              resize: "vertical",
              outline: "none",
              boxSizing: "border-box",
            }}
          />
        </div>
      )}
    </div>
  );
}

/**
 * Inferred object editor — renders object values without a schema by
 * introspecting the actual value types. Used when additionalProperties
 * is empty but the runtime value is a rich object.
 */
export function InferredObjectEditor({
  value,
  onChange,
  compact,
}: {
  value: Record<string, unknown>;
  onChange: (value: unknown) => void;
  compact: boolean;
}) {
  const entries = Object.entries(value);
  const handleFieldChange = useCallback(
    (key: string, fieldValue: unknown) => {
      onChange({ ...value, [key]: fieldValue });
    },
    [value, onChange],
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: compact ? 2 : 4 }}>
      {entries.map(([key, val]) => {
        // Array of strings → pill list
        if (Array.isArray(val) && val.every((v) => typeof v === "string")) {
          return (
            <div key={key}>
              <div style={{ fontSize: 10, color: theme.fgMuted, fontWeight: 500, marginBottom: 2 }}>
                {formatLabel(key)}
              </div>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 3 }}>
                {val.map((item, i) => (
                  <span
                    key={i}
                    style={{
                      fontSize: 10,
                      padding: "1px 6px",
                      borderRadius: 10,
                      border: `1px solid ${theme.border}`,
                      fontFamily: "var(--font-mono, ui-monospace, monospace)",
                      color: theme.fg,
                      background: theme.bgCard,
                    }}
                  >
                    {String(item)}
                  </span>
                ))}
              </div>
            </div>
          );
        }

        // Boolean → inline label
        if (typeof val === "boolean") {
          return (
            <div key={key} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 10 }}>
              <span style={{ color: theme.fgMuted, fontWeight: 500 }}>{formatLabel(key)}:</span>
              <span style={{ color: val ? "oklch(0.65 0.15 145)" : theme.fgMuted }}>{val ? "Yes" : "No"}</span>
            </div>
          );
        }

        // String → inline text
        if (typeof val === "string") {
          return (
            <div key={key} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 10 }}>
              <span style={{ color: theme.fgMuted, fontWeight: 500 }}>{formatLabel(key)}:</span>
              <span style={{ color: theme.fg, fontFamily: "var(--font-mono, ui-monospace, monospace)" }}>{val}</span>
            </div>
          );
        }

        // Number → inline
        if (typeof val === "number") {
          return (
            <div key={key} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 10 }}>
              <span style={{ color: theme.fgMuted, fontWeight: 500 }}>{formatLabel(key)}:</span>
              <span style={{ color: theme.fg }}>{val}</span>
            </div>
          );
        }

        // Fallback: JSON string
        return (
          <div key={key} style={{ fontSize: 10 }}>
            <span style={{ color: theme.fgMuted, fontWeight: 500 }}>{formatLabel(key)}: </span>
            <span style={{ color: theme.fgMuted, fontFamily: "var(--font-mono, ui-monospace, monospace)" }}>
              {JSON.stringify(val)}
            </span>
          </div>
        );
      })}
    </div>
  );
}

/** Compact inline JSON editor (no collapsible header). */
export function JsonInlineEditor({
  value,
  onChange,
  compact: _compact,
}: {
  value: unknown;
  onChange: (value: unknown) => void;
  compact?: boolean;
}) {
  const [text, setText] = useState(() => JSON.stringify(value ?? {}, null, 2));
  const [error, setError] = useState<string | null>(null);

  const handleBlur = useCallback(() => {
    try {
      const parsed = JSON.parse(text);
      setError(null);
      onChange(parsed);
    } catch {
      setError("Invalid JSON");
    }
  }, [text, onChange]);

  return (
    <div>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={handleBlur}
        style={{
          width: "100%",
          minHeight: 50,
          padding: "4px 6px",
          fontSize: 10,
          fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
          lineHeight: 1.5,
          borderRadius: 4,
          border: `1px solid ${error ? theme.destructive : theme.border}`,
          background: theme.bgCard,
          color: theme.fg,
          resize: "vertical",
          outline: "none",
          boxSizing: "border-box",
        }}
      />
      {error && <div style={{ fontSize: 9, color: theme.destructive, marginTop: 2 }}>{error}</div>}
    </div>
  );
}
