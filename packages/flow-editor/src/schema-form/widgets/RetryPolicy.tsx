import { useState, useCallback } from "react";
import { ChevronDown, ChevronRight, RefreshCw } from "lucide-react";
import { theme, inputStyle } from "../utils";
import { FieldWrapper } from "../primitives/FieldWrapper";

export function RetryPolicySection({
  values,
  onChange,
  compact,
}: {
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact: boolean;
}) {
  const [collapsed, setCollapsed] = useState(true);
  const retry = (values.__retry as Record<string, unknown>) ?? {};

  const handleRetryChange = useCallback(
    (key: string, value: unknown) => {
      onChange({ ...values, __retry: { ...retry, [key]: value } });
    },
    [values, retry, onChange],
  );

  return (
    <div style={{ marginTop: 20 }}>
      {/* Section header — collapsible, flat style */}
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
        <RefreshCw size={10} style={{ color: theme.fgMuted }} />
        <span
          style={{
            fontSize: 11,
            fontWeight: 700,
            color: theme.fgMuted,
            textTransform: "uppercase",
            letterSpacing: "0.06em",
          }}
        >
          Retry Policy
        </span>
      </button>
      {!collapsed && (
        <div
          style={{
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 6,
          }}
        >
          <FieldWrapper label="Max Retries" compact={compact}>
            <input
              type="number"
              value={retry.maxRetries != null ? String(retry.maxRetries) : ""}
              placeholder="3"
              min={0}
              max={10}
              onChange={(e) =>
                handleRetryChange(
                  "maxRetries",
                  e.target.value === "" ? undefined : parseInt(e.target.value),
                )
              }
              style={{ ...inputStyle(compact), width: 50 }}
            />
          </FieldWrapper>
          <FieldWrapper label="Backoff" compact={compact}>
            <div style={{ display: "flex", alignItems: "center", gap: 4 }}>
              <input
                type="number"
                value={retry.backoffMs != null ? String(retry.backoffMs) : ""}
                placeholder="1000"
                min={0}
                onChange={(e) =>
                  handleRetryChange(
                    "backoffMs",
                    e.target.value === "" ? undefined : parseInt(e.target.value),
                  )
                }
                style={{ ...inputStyle(compact), width: 70 }}
              />
              <span style={{ fontSize: 10, color: theme.fgMuted }}>ms</span>
            </div>
          </FieldWrapper>
          <FieldWrapper label="Retry On" compact={compact}>
            <input
              type="text"
              value={String(retry.retryOn ?? "")}
              placeholder="error pattern..."
              onChange={(e) => handleRetryChange("retryOn", e.target.value || undefined)}
              style={inputStyle(compact)}
            />
          </FieldWrapper>
        </div>
      )}
    </div>
  );
}
