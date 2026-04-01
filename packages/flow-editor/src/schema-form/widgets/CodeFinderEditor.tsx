import { useState, useCallback } from "react";
import { X } from "lucide-react";
import { theme, inputStyle, removeButtonStyle } from "../utils";

export function CodeFinderRulesEditor({
  label,
  description,
  value,
  presets,
  onChange,
  compact,
}: {
  label: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  presets?: Record<string, unknown>;
  onChange: (value: unknown) => void;
  compact: boolean;
}) {
  const [showPresets, setShowPresets] = useState(false);
  const rules = (value?.rules as Array<{ pattern: string }>) ?? [];
  const sample = (value?.sample as string) ?? "";

  const handleAddRule = useCallback(() => {
    onChange({ ...value, rules: [...rules, { pattern: "" }] });
  }, [value, rules, onChange]);

  const handleRemoveRule = useCallback(
    (index: number) => {
      const next = [...rules];
      next.splice(index, 1);
      onChange({ ...value, rules: next });
    },
    [value, rules, onChange],
  );

  const handleRuleChange = useCallback(
    (index: number, pattern: string) => {
      const next = [...rules];
      next[index] = { pattern };
      onChange({ ...value, rules: next });
    },
    [value, rules, onChange],
  );

  const handleApplyPreset = useCallback(
    (name: string) => {
      if (presets?.[name]) {
        onChange(presets[name]);
        setShowPresets(false);
      }
    },
    [presets, onChange],
  );

  return (
    <div>
      <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>{label}</div>
      {description && <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 6 }}>{description}</div>}

      {/* Preset selector */}
      {presets && Object.keys(presets).length > 0 && (
        <div style={{ position: "relative", marginBottom: 6 }}>
          <button
            onClick={() => setShowPresets(!showPresets)}
            style={{
              padding: "2px 8px",
              borderRadius: 4,
              border: `1px solid ${theme.border}`,
              background: theme.bgSecondary,
              color: theme.fgSecondary,
              cursor: "pointer",
              fontSize: 10,
              fontWeight: 500,
            }}
          >
            Presets
          </button>
          {showPresets && (
            <div
              style={{
                position: "absolute",
                top: "100%",
                left: 0,
                marginTop: 2,
                background: theme.bgCard,
                border: `1px solid ${theme.border}`,
                borderRadius: 4,
                boxShadow: "0 4px 12px oklch(0 0 0 / 0.2)",
                zIndex: 10,
                minWidth: 120,
                overflow: "hidden",
              }}
            >
              {Object.keys(presets).map((name) => (
                <button
                  key={name}
                  onClick={() => handleApplyPreset(name)}
                  style={{
                    display: "block",
                    width: "100%",
                    padding: "4px 10px",
                    textAlign: "left",
                    background: "transparent",
                    border: "none",
                    cursor: "pointer",
                    fontSize: 10,
                    color: theme.fg,
                  }}
                >
                  {name}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Rules list */}
      {rules.map((rule, index) => (
        <div
          key={index}
          style={{
            display: "flex",
            gap: 4,
            alignItems: "center",
            borderBottom: `1px solid ${theme.border}`,
            padding: "4px 0",
          }}
        >
          <span
            style={{ fontSize: 9, color: theme.fgMuted, minWidth: 12, textAlign: "center" }}
          >
            {index + 1}
          </span>
          <input
            type="text"
            value={rule.pattern}
            placeholder="Regex pattern..."
            onChange={(e) => handleRuleChange(index, e.target.value)}
            style={{
              ...inputStyle(compact),
              flex: 1,
              fontSize: 10,
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            }}
          />
          <button onClick={() => handleRemoveRule(index)} style={{ ...removeButtonStyle, opacity: 0.5 }}>
            <X size={10} />
          </button>
        </div>
      ))}

      <button
        onClick={handleAddRule}
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
        + Add rule
      </button>

      {/* Sample text */}
      <div style={{ marginTop: 4 }}>
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>Sample Text</div>
        <input
          type="text"
          value={sample}
          placeholder="Text to test patterns against..."
          onChange={(e) => onChange({ ...value, sample: e.target.value })}
          style={{ ...inputStyle(compact), fontSize: 10 }}
        />
      </div>
    </div>
  );
}
