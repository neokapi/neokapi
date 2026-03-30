import { useState, useCallback, useMemo } from "react";
import {
  ChevronDown,
  ChevronRight,
  RefreshCw,
  Plus,
  X,
  GripVertical,
  Code2,
  Braces,
  List,
} from "lucide-react";
import type { ComponentSchema, PropertySchema, ParameterGroup } from "./types";
import { theme } from "./theme";

// ─── Public API ─────────────────────────────────────────────

interface SchemaFormProps {
  schema: ComponentSchema;
  values: Record<string, unknown>;
  onChange: (values: Record<string, unknown>) => void;
  compact?: boolean;
}

/**
 * Schema-driven configuration form.
 * Auto-generates form fields from a ComponentSchema, respecting groups,
 * types, defaults, enums, validation constraints, nested objects, arrays,
 * dynamic maps, and x-widget hints.
 */
export function SchemaForm({ schema, values, onChange, compact = false }: SchemaFormProps) {
  const { properties, groups, ungrouped } = useMemo(() => {
    const props = schema.properties || {};
    const grps = schema["x-groups"] || [];
    const grouped = new Set(grps.flatMap((g) => g.fields));
    const ungrp = Object.keys(props).filter((k) => !grouped.has(k) && !props[k].deprecated);
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
              allValues={values}
            />
          ))}
        </div>
      )}

      {/* Retry Policy section */}
      <RetryPolicySection values={values} onChange={onChange} compact={compact} />
    </div>
  );
}

// ─── Field Group ────────────────────────────────────────────

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
  const fields = group.fields.filter((f) => properties[f] && !properties[f].deprecated);
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
              allValues={values}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Property Field Dispatcher ──────────────────────────────

function PropertyField({
  name,
  schema,
  value,
  onChange,
  compact,
  allValues,
  depth = 0,
}: {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
  compact: boolean;
  allValues?: Record<string, unknown>;
  depth?: number;
}) {
  // x-showIf conditional visibility
  const showIf = schema["x-showIf"] as { field: string; value?: unknown; empty?: boolean } | undefined;
  if (showIf && allValues) {
    const otherVal = allValues[showIf.field];
    if (showIf.empty !== undefined) {
      // empty=true → show when the other field is empty/unset
      const isEmpty = otherVal === undefined || otherVal === null || otherVal === "";
      if (showIf.empty && !isEmpty) return null;
      if (!showIf.empty && isEmpty) return null;
    } else if (showIf.value !== undefined) {
      // String comparison for enum/mode-selector values
      if (String(otherVal ?? "") !== String(showIf.value)) return null;
    }
  }

  const label = schema.title || formatLabel(name);
  const resolved = value ?? schema.default;
  const widget = schema["x-widget"] as string | undefined;

  // ── x-widget dispatch ──

  if (widget === "code-editor") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["x-placeholder"] || "// Enter JavaScript code..."}
          onChange={(e) => onChange(e.target.value || undefined)}
          rows={compact ? 6 : 10}
          style={{
            ...inputStyle(compact),
            fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
            fontSize: compact ? 10 : 11,
            lineHeight: 1.5,
            resize: "vertical",
            minHeight: compact ? 80 : 120,
            whiteSpace: "pre",
            tabSize: 2,
          }}
          spellCheck={false}
        />
      </FieldWrapper>
    );
  }

  if (widget === "file-picker") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <div style={{ display: "flex", gap: 4 }}>
          <input
            type="text"
            value={String(resolved ?? "")}
            placeholder={schema["x-placeholder"] || "/path/to/file..."}
            onChange={(e) => onChange(e.target.value || undefined)}
            style={{
              ...inputStyle(compact),
              flex: 1,
              fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
              fontSize: compact ? 10 : 11,
            }}
          />
          <button
            type="button"
            onClick={() => {
              // Trigger native file dialog via the HTML file input fallback.
              const input = document.createElement("input");
              input.type = "file";
              input.accept = ".js,.mjs,.cjs";
              input.onchange = () => {
                const file = input.files?.[0];
                if (file) onChange(file.name);
              };
              input.click();
            }}
            style={{
              padding: compact ? "2px 6px" : "4px 8px",
              borderRadius: 4,
              border: `1px solid ${theme.border}`,
              background: theme.bgSecondary,
              color: theme.fgMuted,
              fontSize: compact ? 10 : 11,
              cursor: "pointer",
              whiteSpace: "nowrap",
              flexShrink: 0,
            }}
          >
            Browse
          </button>
        </div>
      </FieldWrapper>
    );
  }

  if (widget === "codeFinderRules") {
    return (
      <CodeFinderRulesEditor
        label={label}
        description={schema.description}
        value={resolved as Record<string, unknown> | undefined}
        presets={schema["x-presets"]}
        onChange={onChange}
        compact={compact}
      />
    );
  }

  if (widget === "simplifierRulesEditor") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["x-placeholder"] || "One rule per line..."}
          onChange={(e) => onChange(e.target.value || undefined)}
          style={{
            ...inputStyle(compact),
            minHeight: 60,
            resize: "vertical",
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            fontSize: compact ? 10 : 11,
            lineHeight: 1.5,
          }}
        />
      </FieldWrapper>
    );
  }

  if (widget === "elementRulesEditor" || widget === "attributeRulesEditor") {
    return (
      <MapEditor
        label={label}
        description={schema.description}
        value={resolved as Record<string, unknown> | undefined}
        itemSchema={resolveRef(schema)}
        onChange={onChange}
        compact={compact}
        depth={depth}
        keyPlaceholder={widget === "elementRulesEditor" ? "element name" : "attribute name"}
      />
    );
  }

  if (widget === "regexBuilder" || widget === "tagList") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <input
          type="text"
          value={String(resolved ?? "")}
          placeholder={
            schema["x-placeholder"] || (widget === "tagList" ? "tag1, tag2, ..." : "pattern...")
          }
          onChange={(e) => onChange(e.target.value || undefined)}
          style={{
            ...inputStyle(compact),
            fontFamily:
              widget === "regexBuilder"
                ? "ui-monospace, SFMono-Regular, Menlo, monospace"
                : "inherit",
          }}
        />
      </FieldWrapper>
    );
  }

  if (widget === "numberList") {
    return (
      <FieldWrapper label={label} description={schema.description} compact={compact}>
        <input
          type="text"
          value={String(resolved ?? "")}
          placeholder={schema["x-placeholder"] || "1, 2, 3, ..."}
          onChange={(e) => onChange(e.target.value || undefined)}
          style={inputStyle(compact)}
        />
      </FieldWrapper>
    );
  }

  // ── Type-based dispatch ──

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
        <ToggleSwitch checked={(resolved as boolean) ?? false} onToggle={(v) => onChange(v)} />
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
            onChange(
              v === "" ? undefined : schema.type === "integer" ? parseInt(v) : parseFloat(v),
            );
          }}
          style={inputStyle(compact)}
        />
      </FieldWrapper>
    );
  }

  // ── Object: nested sub-form vs map vs JSON fallback ──

  if (schema.type === "object") {
    // Object with defined properties → collapsible nested sub-form
    if (schema.properties && Object.keys(schema.properties).length > 0) {
      return (
        <NestedObjectEditor
          label={label}
          description={schema.description}
          schema={schema}
          value={resolved as Record<string, unknown> | undefined}
          onChange={onChange}
          compact={compact}
          depth={depth}
        />
      );
    }

    // Object with additionalProperties → dynamic map editor
    if (hasAdditionalProperties(schema)) {
      return (
        <MapEditor
          label={label}
          description={schema.description}
          value={resolved as Record<string, unknown> | undefined}
          itemSchema={resolveRef(schema)}
          onChange={onChange}
          compact={compact}
          depth={depth}
        />
      );
    }

    // Bare object → JSON textarea
    return (
      <JsonEditor
        label={label}
        description={schema.description}
        value={resolved}
        onChange={onChange}
      />
    );
  }

  // ── Array: structured list vs JSON fallback ──

  if (schema.type === "array") {
    if (schema.items) {
      return (
        <ArrayEditor
          label={label}
          description={schema.description}
          itemSchema={schema.items}
          value={resolved as unknown[] | undefined}
          onChange={onChange}
          compact={compact}
          depth={depth}
        />
      );
    }

    // Array without items schema → JSON fallback
    return (
      <JsonEditor
        label={label}
        description={schema.description}
        value={resolved}
        onChange={onChange}
      />
    );
  }

  // Default: string input
  return (
    <FieldWrapper label={label} description={schema.description} compact={compact}>
      <input
        type="text"
        value={String(resolved ?? "")}
        placeholder={
          schema["x-placeholder"] || (schema.default != null ? String(schema.default) : undefined)
        }
        onChange={(e) => onChange(e.target.value || undefined)}
        style={inputStyle(compact)}
      />
    </FieldWrapper>
  );
}

// ─── Nested Object Editor ───────────────────────────────────

function NestedObjectEditor({
  label,
  description,
  schema,
  value,
  onChange,
  compact,
  depth,
}: {
  label: string;
  description?: string;
  schema: PropertySchema;
  value: Record<string, unknown> | undefined;
  onChange: (value: unknown) => void;
  compact: boolean;
  depth: number;
}) {
  const [collapsed, setCollapsed] = useState(depth > 0);
  const current = value ?? {};
  const properties = schema.properties || {};
  const keys = Object.keys(properties).filter((k) => !properties[k].deprecated);

  const handleFieldChange = useCallback(
    (key: string, fieldValue: unknown) => {
      onChange({ ...current, [key]: fieldValue });
    },
    [current, onChange],
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
          padding: "5px 8px",
          background: depth > 0 ? "transparent" : theme.bgSecondary,
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
        <Braces size={11} style={{ color: theme.accent, flexShrink: 0 }} />
        <span
          style={{
            fontSize: 11,
            fontWeight: 600,
            color: theme.fg,
            flex: 1,
          }}
        >
          {label}
        </span>
        <span style={{ fontSize: 10, color: theme.fgMuted }}>{keys.length}</span>
      </button>

      {!collapsed && (
        <div
          style={{
            padding: compact ? "4px 8px" : "6px 8px",
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 6,
            borderTop: `1px solid ${theme.border}`,
          }}
        >
          {description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>{description}</div>
          )}
          {keys.map((key) => (
            <PropertyField
              key={key}
              name={key}
              schema={properties[key]}
              value={current[key]}
              onChange={(v) => handleFieldChange(key, v)}
              compact={compact}
              allValues={current}
              depth={depth + 1}
            />
          ))}
        </div>
      )}
    </div>
  );
}

// ─── Map Editor (dynamic key-value) ─────────────────────────

function MapEditor({
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
  const [collapsed, setCollapsed] = useState(true);
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
          padding: "5px 8px",
          background: depth > 0 ? "transparent" : theme.bgSecondary,
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
        <List size={11} style={{ color: theme.accent, flexShrink: 0 }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg, flex: 1 }}>{label}</span>
        <span style={{ fontSize: 10, color: theme.fgMuted }}>{entries.length}</span>
      </button>

      {!collapsed && (
        <div
          style={{
            borderTop: `1px solid ${theme.border}`,
            display: "flex",
            flexDirection: "column",
          }}
        >
          {description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, padding: "4px 8px 0" }}>
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
              padding: "4px 8px 6px",
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
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                width: 22,
                height: 22,
                borderRadius: 4,
                border: `1px solid ${theme.border}`,
                background: theme.bgSecondary,
                cursor: newKey.trim() && !(newKey.trim() in current) ? "pointer" : "not-allowed",
                opacity: newKey.trim() && !(newKey.trim() in current) ? 1 : 0.4,
                color: theme.fg,
              }}
            >
              <Plus size={11} />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

function MapEntry({
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

  // Simple value — inline edit
  if (!isComplex) {
    return (
      <div
        style={{
          display: "flex",
          gap: 4,
          padding: "3px 8px",
          alignItems: "center",
          borderBottom: `1px solid ${theme.border}`,
        }}
      >
        <span
          style={{
            fontSize: 10,
            fontWeight: 600,
            color: theme.fgSecondary,
            minWidth: 60,
            fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
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
        <button onClick={onRemove} style={removeButtonStyle}>
          <X size={10} />
        </button>
      </div>
    );
  }

  // Complex value — expandable sub-form
  return (
    <div style={{ borderBottom: `1px solid ${theme.border}` }}>
      <div
        style={{
          display: "flex",
          gap: 4,
          padding: "3px 8px",
          alignItems: "center",
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
              fontSize: 10,
              fontWeight: 600,
              color: theme.fgSecondary,
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
            }}
          >
            {entryKey}
          </span>
        </button>
        <button onClick={onRemove} style={removeButtonStyle}>
          <X size={10} />
        </button>
      </div>

      {expanded && (
        <div
          style={{
            padding: "4px 8px 6px 16px",
            display: "flex",
            flexDirection: "column",
            gap: compact ? 2 : 4,
          }}
        >
          {itemSchema?.properties ? (
            // Render sub-properties
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
          ) : (
            // Fallback: JSON editor for unstructured objects
            <JsonInlineEditor value={value} onChange={onChange} />
          )}
        </div>
      )}
    </div>
  );
}

// ─── Array Editor ───────────────────────────────────────────

function ArrayEditor({
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
  const [collapsed, setCollapsed] = useState(depth > 0);
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
          padding: "5px 8px",
          background: depth > 0 ? "transparent" : theme.bgSecondary,
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
        <List size={11} style={{ color: theme.accent, flexShrink: 0 }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg, flex: 1 }}>{label}</span>
        <span style={{ fontSize: 10, color: theme.fgMuted }}>{items.length}</span>
      </button>

      {!collapsed && (
        <div
          style={{
            borderTop: `1px solid ${theme.border}`,
            display: "flex",
            flexDirection: "column",
          }}
        >
          {description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, padding: "4px 8px 0" }}>
              {description}
            </div>
          )}

          {items.map((item, index) => (
            <div
              key={index}
              style={{
                display: "flex",
                gap: 4,
                padding: isSimple ? "3px 8px" : "0",
                alignItems: isSimple ? "center" : "stretch",
                borderBottom: `1px solid ${theme.border}`,
              }}
            >
              {/* Drag handle / index */}
              <div
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 2,
                  padding: isSimple ? 0 : "3px 0 3px 8px",
                  color: theme.fgMuted,
                  flexShrink: 0,
                }}
              >
                <GripVertical
                  size={10}
                  style={{ cursor: "grab", opacity: 0.5 }}
                  onClick={() => handleMove(index, -1)}
                />
                <span style={{ fontSize: 9, fontWeight: 600, minWidth: 12, textAlign: "center" }}>
                  {index + 1}
                </span>
              </div>

              {isSimple ? (
                <input
                  type={itemSchema.type === "string" ? "text" : "number"}
                  value={String(item ?? "")}
                  placeholder={itemSchema["x-placeholder"]}
                  onChange={(e) => {
                    const v = e.target.value;
                    handleItemChange(
                      index,
                      itemSchema.type === "integer"
                        ? parseInt(v) || 0
                        : itemSchema.type === "number"
                          ? parseFloat(v) || 0
                          : v,
                    );
                  }}
                  style={{
                    ...inputStyle(compact),
                    flex: 1,
                    fontSize: 10,
                    fontFamily:
                      itemSchema.type === "string"
                        ? "ui-monospace, SFMono-Regular, Menlo, monospace"
                        : "inherit",
                  }}
                />
              ) : (
                <div
                  style={{
                    flex: 1,
                    padding: "4px 0",
                    display: "flex",
                    flexDirection: "column",
                    gap: 2,
                  }}
                >
                  {itemSchema.properties ? (
                    Object.entries(itemSchema.properties)
                      .filter(([, s]) => !s.deprecated)
                      .map(([key, fieldSchema]) => (
                        <div key={key} style={{ padding: "0 4px" }}>
                          <PropertyField
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
                        </div>
                      ))
                  ) : (
                    <div style={{ padding: "0 4px" }}>
                      <JsonInlineEditor
                        value={item}
                        onChange={(v) => handleItemChange(index, v)}
                        compact={compact}
                      />
                    </div>
                  )}
                </div>
              )}

              <button
                onClick={() => handleRemove(index)}
                style={{
                  ...removeButtonStyle,
                  alignSelf: isSimple ? "center" : "flex-start",
                  marginTop: isSimple ? 0 : 3,
                  marginRight: isSimple ? 0 : 8,
                }}
              >
                <X size={10} />
              </button>
            </div>
          ))}

          {/* Add item button */}
          <div style={{ padding: "4px 8px 6px" }}>
            <button
              onClick={handleAdd}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 4,
                padding: "3px 8px",
                borderRadius: 4,
                border: `1px dashed ${theme.border}`,
                background: "transparent",
                color: theme.fgMuted,
                cursor: "pointer",
                fontSize: 10,
                width: "100%",
                justifyContent: "center",
              }}
            >
              <Plus size={10} />
              Add item
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Code Finder Rules Editor ───────────────────────────────

function CodeFinderRulesEditor({
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
  const [collapsed, setCollapsed] = useState(false);
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
          padding: "5px 8px",
          background: theme.bgSecondary,
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
        <Code2 size={11} style={{ color: theme.accent, flexShrink: 0 }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg, flex: 1 }}>{label}</span>
        <span style={{ fontSize: 10, color: theme.fgMuted }}>{rules.length}</span>
      </button>

      {!collapsed && (
        <div
          style={{
            borderTop: `1px solid ${theme.border}`,
            display: "flex",
            flexDirection: "column",
            gap: 4,
            padding: "6px 8px",
          }}
        >
          {description && <div style={{ fontSize: 10, color: theme.fgMuted }}>{description}</div>}

          {/* Preset selector */}
          {presets && Object.keys(presets).length > 0 && (
            <div style={{ position: "relative" }}>
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
              <button onClick={() => handleRemoveRule(index)} style={removeButtonStyle}>
                <X size={10} />
              </button>
            </div>
          ))}

          <button
            onClick={handleAddRule}
            style={{
              display: "flex",
              alignItems: "center",
              gap: 4,
              padding: "3px 8px",
              borderRadius: 4,
              border: `1px dashed ${theme.border}`,
              background: "transparent",
              color: theme.fgMuted,
              cursor: "pointer",
              fontSize: 10,
              justifyContent: "center",
            }}
          >
            <Plus size={10} />
            Add rule
          </button>

          {/* Sample text */}
          <div style={{ marginTop: 2 }}>
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
      )}
    </div>
  );
}

// ─── JSON Editor (fallback for unstructured types) ──────────

function JsonEditor({
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
    <div
      style={{
        borderRadius: 6,
        border: `1px solid ${error ? theme.destructive : theme.border}`,
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
          padding: "5px 8px",
          background: theme.bgSecondary,
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
        <Braces size={11} style={{ color: theme.fgMuted, flexShrink: 0 }} />
        <span style={{ fontSize: 11, fontWeight: 600, color: theme.fg, flex: 1 }}>{label}</span>
        {error && <span style={{ fontSize: 9, color: theme.destructive }}>{error}</span>}
      </button>

      {!collapsed && (
        <div
          style={{
            borderTop: `1px solid ${theme.border}`,
            padding: "6px 8px",
          }}
        >
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

/** Compact inline JSON editor (no collapsible header). */
function JsonInlineEditor({
  value,
  onChange,
}: {
  value: unknown;
  onChange: (value: unknown) => void;
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

// ─── Retry Policy Section ───────────────────────────────────

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

// ─── Shared Primitives ──────────────────────────────────────

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
        <div style={{ fontSize: 10, color: theme.fgMuted, marginBottom: 2 }}>{description}</div>
      )}
      {children}
    </div>
  );
}

function ToggleSwitch({ checked, onToggle }: { checked: boolean; onToggle: (v: boolean) => void }) {
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

// ─── Utilities ──────────────────────────────────────────────

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

const removeButtonStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: 18,
  height: 18,
  borderRadius: 4,
  border: "none",
  background: "transparent",
  cursor: "pointer",
  color: theme.fgMuted,
  flexShrink: 0,
};

function formatLabel(name: string): string {
  return name
    .replace(/([A-Z])/g, " $1")
    .replace(/^./, (s) => s.toUpperCase())
    .trim();
}

/**
 * Resolve $ref-style additionalProperties. In our schemas, refs point
 * to $defs which aren't resolved at runtime, so we return the raw
 * additionalProperties schema if it's an object, otherwise undefined.
 */
function resolveRef(schema: PropertySchema): PropertySchema | undefined {
  const ap = schema.additionalProperties;
  if (ap && typeof ap === "object") {
    return ap;
  }
  return undefined;
}

function hasAdditionalProperties(schema: PropertySchema): boolean {
  return schema.additionalProperties != null && schema.additionalProperties !== false;
}
