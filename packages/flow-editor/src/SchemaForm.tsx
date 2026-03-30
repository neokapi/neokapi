import { useState, useCallback, useMemo } from "react";
import { ChevronDown, ChevronRight, RefreshCw } from "lucide-react";
import type { ComponentSchema, PropertySchema, ParameterGroup } from "./types";
import { theme } from "./theme";

interface SchemaFormProps {
  schema: ComponentSchema;
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact?: boolean;
}

/**
 * Schema-driven configuration form.
 * Auto-generates form fields from a ComponentSchema, respecting groups,
 * types, defaults, enums, and validation constraints.
 */
export function SchemaForm({
  schema,
  values,
  onChange,
  compact = false,
}: SchemaFormProps) {
  const { properties, groups, ungrouped } = useMemo(() => {
    const props = schema.properties || {};
    const grps = schema["x-groups"] || [];
    const grouped = new Set(grps.flatMap((g) => g.fields));
    const ungrp = Object.keys(props).filter(
      (k) => !grouped.has(k) && !props[k].deprecated,
    );
    return { properties: props, groups: grps, ungrouped: ungrp };
  }, [schema]);

  const handleChange = useCallback(
    (key: string, value: unknown) => {
      onChange({ ...values, [key]: value });
    },
    [values, onChange],
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: compact ? 4 : 8 }}>
      {/* Grouped fields */}
      {groups.map((group) => (
        <FieldGroup
          key={group.id}
          group={group}
          properties={properties}
          values={values}
          onChange={handleChange}
          compact={compact}
        />
      ))}

      {/* Ungrouped fields */}
      {ungrouped.length > 0 && (
        <div style={{ display: "flex", flexDirection: "column", gap: compact ? 2 : 6 }}>
          {groups.length > 0 && ungrouped.length > 0 && (
            <div
              style={{
                fontSize: 10,
                fontWeight: 600,
                color: theme.fgMuted,
                textTransform: "uppercase",
                letterSpacing: "0.05em",
                padding: "4px 0 2px",
              }}
            >
              Other
            </div>
          )}
          {ungrouped.map((key) => (
            <PropertyField
              key={key}
              name={key}
              schema={properties[key]}
              value={values[key]}
              onChange={(v) => handleChange(key, v)}
              compact={compact}
            />
          ))}
        </div>
      )}

      {/* Retry Policy section */}
      <RetryPolicySection
        values={values}
        onChange={onChange}
        compact={compact}
      />
    </div>
  );
}

function FieldGroup({
  group,
  properties,
  values,
  onChange,
  compact,
}: {
  group: ParameterGroup;
  properties: Record<string, PropertySchema>;
  values: Record<string, unknown>;
  onChange: (key: string, value: unknown) => void;
  compact: boolean;
}) {
  const [collapsed, setCollapsed] = useState(group.collapsed ?? false);
  const fields = group.fields.filter(
    (f) => properties[f] && !properties[f].deprecated,
  );
  if (fields.length === 0) return null;

  return (
    <div
      style={{
        borderRadius: 6,
        border: `1px solid ${theme.border}`,
        overflow: "hidden",
      }}
    >
      <button
        onClick={() => setCollapsed(!collapsed)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 6,
          width: "100%",
          padding: "6px 8px",
          background: theme.bgSecondary,
          border: "none",
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        {collapsed ? (
          <ChevronRight size={12} style={{ color: theme.fgMuted }} />
        ) : (
          <ChevronDown size={12} style={{ color: theme.fgMuted }} />
        )}
        <span
          style={{
            fontSize: 11,
            fontWeight: 600,
            color: theme.fg,
          }}
        >
          {group.label}
        </span>
        <span style={{ fontSize: 10, color: theme.fgMuted, marginLeft: "auto" }}>
          {fields.length}
        </span>
      </button>
      {!collapsed && (
        <div
          style={{
            padding: compact ? "4px 8px" : "6px 8px",
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
            />
          ))}
        </div>
      )}
    </div>
  );
}

function PropertyField({
  name,
  schema,
  value,
  onChange,
  compact,
}: {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
  compact: boolean;
}) {
  const label = schema.title || formatLabel(name);
  const resolved = value ?? schema.default;

  if (schema.type === "boolean") {
    return (
      <label
        style={{
          display: "flex",
          alignItems: "center",
          gap: 8,
          padding: "2px 0",
          cursor: "pointer",
        }}
      >
        <ToggleSwitch
          checked={resolved as boolean ?? false}
          onToggle={(v) => onChange(v)}
        />
        <div style={{ flex: 1 }}>
          <div
            style={{
              fontSize: compact ? 11 : 12,
              color: theme.fg,
              fontWeight: 500,
            }}
          >
            {label}
          </div>
          {!compact && schema.description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, marginTop: 1 }}>
              {schema.description}
            </div>
          )}
        </div>
      </label>
    );
  }

  if (schema.enum && schema.enum.length > 0) {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <select
          value={String(resolved ?? "")}
          onChange={(e) => onChange(e.target.value)}
          style={inputStyle(compact)}
        >
          <option value="">—</option>
          {schema.enum.map((v) => (
            <option key={String(v)} value={String(v)}>
              {String(v)}
            </option>
          ))}
        </select>
      </FieldWrapper>
    );
  }

  if (schema.type === "integer" || schema.type === "number") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <input
          type="number"
          value={resolved != null ? String(resolved) : ""}
          placeholder={schema.default != null ? String(schema.default) : undefined}
          min={schema.minimum}
          max={schema.maximum}
          step={schema.type === "integer" ? 1 : undefined}
          onChange={(e) => {
            const v = e.target.value;
            onChange(v === "" ? undefined : schema.type === "integer" ? parseInt(v) : parseFloat(v));
          }}
          style={inputStyle(compact)}
        />
      </FieldWrapper>
    );
  }

  // Default: string input
  return (
    <FieldWrapper label={label} description={schema.description} compact={compact}>
      <input
        type="text"
        value={String(resolved ?? "")}
        placeholder={
          schema["x-placeholder"] ||
          (schema.default != null ? String(schema.default) : undefined)
        }
        onChange={(e) => onChange(e.target.value || undefined)}
        style={inputStyle(compact)}
      />
    </FieldWrapper>
  );
}

function FieldWrapper({
  label,
  description,
  compact,
  children,
}: {
  label: string;
  description?: string;
  compact: boolean;
  children: React.ReactNode;
}) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <div
        style={{
          fontSize: compact ? 11 : 12,
          color: theme.fgSecondary,
          fontWeight: 500,
        }}
      >
        {label}
      </div>
      {!compact && description && (
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>
          {description}
        </div>
      )}
      {children}
    </div>
  );
}

function ToggleSwitch({
  checked,
  onToggle,
}: {
  checked: boolean;
  onToggle: (v: boolean) => void;
}) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onToggle(!checked)}
      style={{
        width: 28,
        height: 16,
        borderRadius: 8,
        border: "none",
        cursor: "pointer",
        position: "relative",
        flexShrink: 0,
        background: checked ? theme.accent : theme.bgMuted,
        transition: "background 150ms",
      }}
    >
      <div
        style={{
          width: 12,
          height: 12,
          borderRadius: 6,
          background: theme.primaryFg,
          position: "absolute",
          top: 2,
          left: checked ? 14 : 2,
          transition: "left 150ms",
        }}
      />
    </button>
  );
}

function RetryPolicySection({
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
    <div
      style={{
        borderRadius: 6,
        border: `1px solid ${theme.border}`,
        overflow: "hidden",
      }}
    >
      <button
        onClick={() => setCollapsed(!collapsed)}
        style={{
          display: "flex",
          alignItems: "center",
          gap: 6,
          width: "100%",
          padding: "6px 8px",
          background: "var(--muted)",
          border: "none",
          cursor: "pointer",
          textAlign: "left",
        }}
      >
        {collapsed ? (
          <ChevronRight size={12} style={{ color: theme.fgMuted }} />
        ) : (
          <ChevronDown size={12} style={{ color: theme.fgMuted }} />
        )}
        <RefreshCw size={11} style={{ color: theme.fgMuted }} />
        <span
          style={{
            fontSize: 11,
            fontWeight: 600,
            color: theme.fg,
          }}
        >
          Retry Policy
        </span>
      </button>
      {!collapsed && (
        <div
          style={{
            padding: compact ? "4px 8px" : "6px 8px",
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 6,
            background: "var(--muted)",
          }}
        >
          <FieldWrapper label="Max Retries" compact={compact}>
            <input
              type="number"
              value={retry.maxRetries != null ? String(retry.maxRetries) : ""}
              placeholder="3"
              min={0}
              max={10}
              onChange={(e) => handleRetryChange("maxRetries", e.target.value === "" ? undefined : parseInt(e.target.value))}
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
                onChange={(e) => handleRetryChange("backoffMs", e.target.value === "" ? undefined : parseInt(e.target.value))}
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

function inputStyle(compact: boolean): React.CSSProperties {
  return {
    width: "100%",
    padding: compact ? "3px 6px" : "5px 8px",
    fontSize: compact ? 11 : 12,
    borderRadius: 4,
    border: `1px solid ${theme.border}`,
    background: theme.bgCard,
    color: theme.fg,
    fontFamily: "inherit",
    outline: "none",
    boxSizing: "border-box",
  };
}

function formatLabel(name: string): string {
  return name
    .replace(/([A-Z])/g, " $1")
    .replace(/^./, (s) => s.toUpperCase())
    .trim();
}
