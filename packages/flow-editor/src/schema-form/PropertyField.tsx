import type { PropertySchema, ToolDocParam } from "./types";
import { theme, inputStyle, formatLabel, resolveRef, hasAdditionalProperties } from "./utils";
import { resolveSchemaRef } from "./hooks/useSchemaResolution";
import { evaluateCondition } from "./hooks/useConditionalVisibility";
import { useFieldEnabled } from "./hooks/useFieldEnabled";
import { usePresetComparison } from "./hooks/usePresetComparison";
import { resolveWidgetName } from "./registry";
import { Md } from "./primitives/Markdown";
import { ToggleSwitch } from "./primitives/ToggleSwitch";
import { PresetDot } from "./primitives/PresetDot";
import { FieldWrapper } from "./primitives/FieldWrapper";
import { CodeFinderRulesEditor } from "./widgets/CodeFinderEditor";
import { MapEditor } from "./widgets/MapEditor";
import { ArrayEditor } from "./widgets/ArrayEditor";
import { NestedObjectEditor } from "./widgets/NestedObjectEditor";
import { JsonEditor } from "./widgets/JsonEditor";

export function PropertyField({
  name,
  schema: rawSchema,
  value,
  onChange,
  compact,
  allValues,
  allProperties,
  depth = 0,
  onDrillDown,
  presetValues,
  docParam,
  defs,
  error,
}: {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
  compact: boolean;
  allValues?: Record<string, unknown>;
  allProperties?: Record<string, PropertySchema>;
  depth?: number;
  onDrillDown?: (label: string, key: string, schema: PropertySchema, values: Record<string, unknown>) => void;
  presetValues?: Record<string, unknown>;
  docParam?: ToolDocParam;
  defs?: Record<string, PropertySchema>;
  error?: string;
}) {
  // Resolve $ref if present
  const schema = rawSchema.$ref ? resolveSchemaRef(rawSchema, defs) : rawSchema;

  // ui:enabled — disable field based on condition expression
  const enabled = useFieldEnabled(schema["ui:enabled"], allValues, allProperties);
  const disabled = !enabled;

  // ui:visible — conditional visibility
  const visible = evaluateCondition(schema["ui:visible"], allValues, allProperties);
  if (!visible) return null;

  const label = schema.title || formatLabel(name);
  const resolved = value ?? schema.default;
  const widget = resolveWidgetName(schema["ui:widget"]);
  const enumLabels = schema["ui:enum-labels"];

  // Suppress description when it's redundant with the label (same text, case-insensitive).
  const description = schema.description && label.toLowerCase() !== schema.description.toLowerCase()
    ? schema.description
    : undefined;

  // Layout hints from ui:layout
  const layoutHints = schema["ui:layout"];
  const showLabel = !layoutHints?.hideLabel;
  const verticalLayout = layoutHints?.vertical ?? false;

  // Widget options (path/folder/text metadata)
  const editor = schema["ui:widget-options"] as
    | { path?: { browseTitle?: string; forSaveAs?: boolean; allowEmpty?: boolean; filters?: Array<{ name: string; extensions: string }> }; folder?: { browseTitle?: string }; text?: { password?: boolean; allowEmpty?: boolean; height?: number } }
    | undefined;

  // Preset-modified indicator: compare current value with preset value.
  const isModifiedFromPreset = usePresetComparison(name, value, schema.default, presetValues);

  // ── x-widget dispatch ──

  if (widget === "segmented" && schema.enum && schema.enum.length >= 2) {
    const current = String(resolved ?? schema.enum[0]);
    return (
      <div style={{ display: "flex", marginBottom: compact ? 4 : 6 }}>
        {schema.enum.map((opt, i) => {
          const val = String(opt);
          const isActive = current === val;
          const isFirst = i === 0;
          const isLast = i === schema.enum!.length - 1;
          return (
            <button
              key={val}
              onClick={() => onChange(val)}
              style={{
                flex: 1,
                padding: "4px 0",
                fontSize: 10,
                fontWeight: 600,
                letterSpacing: "0.03em",
                border: `1px solid ${theme.border}`,
                borderRight: isLast ? `1px solid ${theme.border}` : "none",
                borderRadius: isFirst
                  ? "4px 0 0 4px"
                  : isLast
                    ? "0 4px 4px 0"
                    : "0",
                background: isActive ? theme.accent : "transparent",
                color: isActive ? theme.accentFg : theme.fgMuted,
                cursor: "pointer",
                transition: "background 120ms, color 120ms",
                textTransform: "capitalize",
              }}
            >
              {val === "inline" ? "Inline Code" : val.charAt(0).toUpperCase() + val.slice(1)}
            </button>
          );
        })}
      </div>
    );
  }

  if (widget === "code-editor") {
    return (
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} error={error}>
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"] || "// Enter JavaScript code..."}
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
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} error={error}>
        <div style={{ display: "flex", gap: 4 }}>
          <input
            type="text"
            value={String(resolved ?? "")}
            placeholder={schema["ui:placeholder"] || "/path/to/file..."}
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

  if (widget === "code-finder") {
    return (
      <CodeFinderRulesEditor
        label={label}
        description={description}
        value={resolved as Record<string, unknown> | undefined}
        presets={schema["ui:presets"]}
        onChange={onChange}
        compact={compact}
      />
    );
  }

  if (widget === "simplifier-rules") {
    return (
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} error={error}>
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"] || "One rule per line..."}
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

  if (widget === "element-rules" || widget === "attribute-rules") {
    return (
      <MapEditor
        label={label}
        description={description}
        value={resolved as Record<string, unknown> | undefined}
        itemSchema={resolveRef(schema)}
        onChange={onChange}
        compact={compact}
        depth={depth}
        keyPlaceholder={widget === "element-rules" ? "element name" : "attribute name"}
      />
    );
  }

  if (widget === "regex" || widget === "tags") {
    return (
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} error={error}>
        <input
          type="text"
          value={String(resolved ?? "")}
          placeholder={
            schema["ui:placeholder"] || (widget === "tags" ? "tag1, tag2, ..." : "pattern...")
          }
          onChange={(e) => onChange(e.target.value || undefined)}
          style={{
            ...inputStyle(compact),
            fontFamily:
              widget === "regex"
                ? "ui-monospace, SFMono-Regular, Menlo, monospace"
                : "inherit",
          }}
        />
      </FieldWrapper>
    );
  }

  if (widget === "number-list") {
    return (
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} error={error}>
        <input
          type="text"
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"] || "1, 2, 3, ..."}
          onChange={(e) => onChange(e.target.value || undefined)}
          style={inputStyle(compact)}
        />
      </FieldWrapper>
    );
  }

  // ── x-editor widget dispatch (new structured metadata) ──

  if (widget === "path") {
    const pathMeta = editor?.path;
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <div style={{ display: "flex", gap: 4 }}>
          <input
            type="text"
            value={String(resolved ?? "")}
            placeholder={schema["ui:placeholder"] || pathMeta?.browseTitle || "/path/to/file..."}
            onChange={(e) => onChange(e.target.value || undefined)}
            disabled={disabled}
            style={{
              ...inputStyle(compact),
              flex: 1,
              fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
              fontSize: compact ? 10 : 11,
              opacity: disabled ? 0.5 : 1,
            }}
          />
          <button
            type="button"
            disabled={disabled}
            onClick={() => {
              const input = document.createElement("input");
              input.type = "file";
              if (pathMeta?.filters?.length) {
                // Build accept string from filter extensions
                const exts = pathMeta.filters
                  .map((f: { name: string; extensions: string }) => f.extensions.replace(/\*\./g, ".").replace(/\*/g, ""))
                  .filter((e: string) => e && e !== ".")
                  .join(",");
                if (exts) input.accept = exts;
              }
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
              cursor: disabled ? "not-allowed" : "pointer",
              whiteSpace: "nowrap",
              flexShrink: 0,
              opacity: disabled ? 0.5 : 1,
            }}
          >
            Browse
          </button>
        </div>
        {pathMeta?.filters && pathMeta.filters.length > 0 && (
          <div style={{ fontSize: 9, color: theme.fgMuted, marginTop: 2 }}>
            {pathMeta.filters.map((f: { name: string; extensions: string }) => f.name).join(", ")}
          </div>
        )}
      </FieldWrapper>
    );
  }

  if (widget === "folder") {
    const folderMeta = editor?.folder;
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <div style={{ display: "flex", gap: 4 }}>
          <input
            type="text"
            value={String(resolved ?? "")}
            placeholder={schema["ui:placeholder"] || folderMeta?.browseTitle || "/path/to/folder..."}
            onChange={(e) => onChange(e.target.value || undefined)}
            disabled={disabled}
            style={{
              ...inputStyle(compact),
              flex: 1,
              fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
              fontSize: compact ? 10 : 11,
              opacity: disabled ? 0.5 : 1,
            }}
          />
          <button
            type="button"
            disabled={disabled}
            style={{
              padding: compact ? "2px 6px" : "4px 8px",
              borderRadius: 4,
              border: `1px solid ${theme.border}`,
              background: theme.bgSecondary,
              color: theme.fgMuted,
              fontSize: compact ? 10 : 11,
              cursor: disabled ? "not-allowed" : "pointer",
              whiteSpace: "nowrap",
              flexShrink: 0,
              opacity: disabled ? 0.5 : 1,
            }}
          >
            Browse
          </button>
        </div>
      </FieldWrapper>
    );
  }

  if (widget === "textarea") {
    const textMeta = editor?.text;
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"] || ""}
          onChange={(e) => onChange(e.target.value || undefined)}
          disabled={disabled}
          rows={textMeta?.height || 4}
          style={{
            ...inputStyle(compact),
            minHeight: (textMeta?.height || 4) * 20,
            resize: "vertical",
            fontFamily: "var(--font-mono, ui-monospace, SFMono-Regular, Menlo, monospace)",
            fontSize: compact ? 10 : 11,
            lineHeight: 1.5,
            opacity: disabled ? 0.5 : 1,
          }}
        />
      </FieldWrapper>
    );
  }

  if (widget === "password") {
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <input
          type="password"
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"]}
          onChange={(e) => onChange(e.target.value || undefined)}
          disabled={disabled}
          style={{ ...inputStyle(compact), opacity: disabled ? 0.5 : 1 }}
        />
      </FieldWrapper>
    );
  }

  const widgetOpts = schema["ui:widget-options"] as Record<string, unknown> | undefined;

  if (widget === "checklist" && widgetOpts?.entries) {
    const entries = widgetOpts.entries as Array<{ name: string; title: string; description?: string }>;
    const current = (resolved as Record<string, boolean>) ?? {};
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          {entries.map((entry) => (
            <label
              key={entry.name}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 8,
                cursor: disabled ? "not-allowed" : "pointer",
                opacity: disabled ? 0.5 : 1,
              }}
            >
              <ToggleSwitch
                checked={current[entry.name] ?? false}
                onToggle={(v) => onChange({ ...current, [entry.name]: v })}
              />
              <div style={{ flex: 1 }}>
                <div style={{ fontSize: compact ? 11 : 12, color: theme.fg, fontWeight: 500 }}>
                  {entry.title}
                </div>
                {entry.description && !compact && (
                  <div style={{ fontSize: 10, color: theme.fgMuted, marginTop: 1 }}>
                    {entry.description}
                  </div>
                )}
              </div>
            </label>
          ))}
        </div>
      </FieldWrapper>
    );
  }

  if (widget === "select" && schema.enum && schema.enum.length > 0) {
    // x-editor "select" = scrollable list (vs "dropdown" which uses standard <select>)
    const current = String(resolved ?? "");
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} vertical={verticalLayout} disabled={disabled} error={error}>
        <div style={{
          border: `1px solid ${theme.border}`,
          borderRadius: 4,
          maxHeight: 120,
          overflowY: "auto",
          background: theme.bgCard,
          opacity: disabled ? 0.5 : 1,
        }}>
          {schema.enum.map((v) => {
            const val = String(v);
            const isActive = current === val;
            const displayLabel = enumLabels?.[val] ?? val;
            return (
              <button
                key={val}
                onClick={() => !disabled && onChange(val)}
                disabled={disabled}
                style={{
                  display: "block",
                  width: "100%",
                  padding: "4px 8px",
                  textAlign: "left",
                  background: isActive ? theme.accent : "transparent",
                  color: isActive ? theme.accentFg : theme.fg,
                  border: "none",
                  borderBottom: `1px solid ${theme.border}`,
                  cursor: disabled ? "not-allowed" : "pointer",
                  fontSize: compact ? 10 : 11,
                  fontWeight: isActive ? 600 : 400,
                }}
              >
                {displayLabel}
              </button>
            );
          })}
        </div>
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
          cursor: disabled ? "not-allowed" : "pointer",
          opacity: disabled ? 0.5 : 1,
        }}
      >
        <ToggleSwitch checked={(resolved as boolean) ?? false} onToggle={(v) => !disabled && onChange(v)} />
        <div style={{ flex: 1 }}>
          <div
            style={{
              fontSize: compact ? 11 : 12,
              color: theme.fg,
              fontWeight: 500,
            }}
          >
            <PresetDot visible={isModifiedFromPreset} />{label}
          </div>
          {!compact && description && (
            <div style={{ fontSize: 10, color: theme.fgMuted, marginTop: 1 }}>
              <Md text={description} style={{ fontSize: 10, color: theme.fgMuted }} />
            </div>
          )}
        </div>
      </label>
    );
  }

  if (schema.enum && schema.enum.length > 0) {
    return (
      <FieldWrapper label={showLabel ? label : ""} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} disabled={disabled} error={error}>
        <select
          value={String(resolved ?? "")}
          onChange={(e) => onChange(e.target.value)}
          disabled={disabled}
          style={{ ...inputStyle(compact), opacity: disabled ? 0.5 : 1 }}
        >
          <option value="">—</option>
          {schema.enum.map((v) => {
            const val = String(v);
            return (
              <option key={val} value={val}>
                {enumLabels?.[val] ?? val}
              </option>
            );
          })}
        </select>
        {(() => {
          const desc = resolved != null ? schema["ui:enum-descriptions"]?.[String(resolved)] : undefined;
          return desc ? (
            <div style={{ fontSize: 9, color: theme.fgMuted, marginTop: 2, fontStyle: "italic" }}>
              {desc}
            </div>
          ) : null;
        })()}
      </FieldWrapper>
    );
  }

  if (schema.type === "integer" || schema.type === "number") {
    return (
      <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} disabled={disabled} error={error}>
        <input
          type="number"
          value={resolved != null ? String(resolved) : ""}
          placeholder={schema.default != null ? String(schema.default) : undefined}
          min={schema.minimum}
          max={schema.maximum}
          step={schema.type === "integer" ? 1 : undefined}
          disabled={disabled}
          onChange={(e) => {
            const v = e.target.value;
            onChange(
              v === "" ? undefined : schema.type === "integer" ? parseInt(v) : parseFloat(v),
            );
          }}
          style={{ ...inputStyle(compact), opacity: disabled ? 0.5 : 1 }}
        />
      </FieldWrapper>
    );
  }

  // ── Object: nested sub-form vs map vs JSON fallback ──

  if (schema.type === "object") {
    // Object with defined properties → nested sub-form (flat at depth 0-1)
    if (schema.properties && Object.keys(schema.properties).length > 0) {
      return (
        <NestedObjectEditor
          label={label}
          description={description}
          schema={schema}
          value={resolved as Record<string, unknown> | undefined}
          onChange={onChange}
          compact={compact}
          depth={depth}
          name={name}
          onDrillDown={onDrillDown}
          defs={defs}
        />
      );
    }

    // Object with additionalProperties → dynamic map editor
    if (hasAdditionalProperties(schema)) {
      return (
        <MapEditor
          label={label}
          description={description}
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
        description={description}
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
          description={description}
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
        description={description}
        value={resolved}
        onChange={onChange}
      />
    );
  }

  // Default: string input
  return (
    <FieldWrapper label={label} description={description} compact={compact} isModified={isModifiedFromPreset} docParam={docParam} disabled={disabled} error={error}>
      <input
        type="text"
        value={String(resolved ?? "")}
        placeholder={
          schema["ui:placeholder"] || (schema.default != null ? String(schema.default) : undefined)
        }
        onChange={(e) => onChange(e.target.value || undefined)}
        disabled={disabled}
        style={{ ...inputStyle(compact), opacity: disabled ? 0.5 : 1 }}
      />
    </FieldWrapper>
  );
}
