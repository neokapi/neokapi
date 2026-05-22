import { cn } from "../../lib/utils";
import { Input } from "../ui/input";
import { Switch } from "../ui/switch";
import { FormToggle } from "../ui/form";
import type { PropertySchema, ToolDocParam } from "./types";
import { evaluateCondition } from "./hooks/useConditionalVisibility";
import { useFieldEnabled } from "./hooks/useFieldEnabled";
import { usePresetComparison } from "./hooks/usePresetComparison";
import { resolveWidgetName } from "./registry";
import { resolveRef, hasAdditionalProperties } from "./utils";
import { FieldWrapper } from "./primitives/FieldWrapper";
import { CodeInput } from "../ui/code-input";
import { TagInput } from "../ui/tag-input";
import { CodeFinderEditor } from "../ui/code-finder-editor";
import { MapEditor } from "./widgets/MapEditor";
import { ArrayEditor } from "./widgets/ArrayEditor";
import { NestedObjectEditor } from "./widgets/NestedObjectEditor";
import { JsonEditor } from "./widgets/JsonEditor";
import { NumberListEditor } from "./widgets/NumberListEditor";
import { PathPicker } from "./widgets/PathPicker";
import { CredentialPicker } from "./widgets/CredentialPicker";

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
  error,
}: {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
  compact?: boolean;
  allValues?: Record<string, unknown>;
  allProperties?: Record<string, PropertySchema>;
  depth?: number;
  onDrillDown?: (
    label: string,
    key: string,
    schema: PropertySchema,
    values: Record<string, unknown>,
  ) => void;
  presetValues?: Record<string, unknown>;
  docParam?: ToolDocParam;
  error?: string;
}) {
  const schema = rawSchema;

  const enabled = useFieldEnabled(schema["ui:enabled"], allValues, allProperties);
  const disabled = !enabled;
  const isModifiedFromPreset = usePresetComparison(name, value, schema.default, presetValues);

  const visible = evaluateCondition(schema["ui:visible"], allValues, allProperties);
  if (!visible) return null;

  const label = schema.title || name;
  const resolved = value ?? schema.default;
  const widget = resolveWidgetName(schema["ui:widget"]);
  const options = schema.options;
  const enumLabels = schema["ui:enum-labels"];
  const effectiveEnum = options ? options.map((o) => o.value) : schema.enum;
  const getLabel = (val: unknown): string => {
    if (options) {
      const opt = options.find((o) => String(o.value) === String(val));
      return opt?.label ?? String(val);
    }
    return enumLabels?.[String(val)] ?? String(val);
  };

  const description =
    schema.description && label.toLowerCase() !== schema.description.toLowerCase()
      ? schema.description
      : undefined;

  const layoutHints = schema["ui:layout"];
  const showLabel = !layoutHints?.hideLabel;
  const verticalLayout = layoutHints?.vertical ?? false;

  const editor = schema["ui:widget-options"] as
    | {
        text?: { height?: number };
      }
    | undefined;

  // -- Widget dispatch --

  if (widget === "segmented" && effectiveEnum && effectiveEnum.length >= 2) {
    const current = String(resolved ?? effectiveEnum[0]);
    return (
      <div className="flex mb-1.5">
        {effectiveEnum.map((opt, i) => {
          const val = String(opt);
          const isActive = current === val;
          const isFirst = i === 0;
          const isLast = i === effectiveEnum!.length - 1;
          return (
            <button
              key={val}
              type="button"
              disabled={disabled}
              onClick={() => onChange(val)}
              className={cn(
                "flex-1 py-1 text-xs font-semibold tracking-wide border transition-colors capitalize",
                isFirst && "rounded-l-md",
                isLast && "rounded-r-md",
                !isLast && "border-r-0",
                isActive
                  ? "bg-primary text-primary-foreground"
                  : "bg-transparent text-muted-foreground hover:bg-accent/50",
                disabled && "opacity-50 cursor-not-allowed",
              )}
            >
              {getLabel(val)}
            </button>
          );
        })}
      </div>
    );
  }

  if (widget === "code-editor") {
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <CodeInput
          value={String(resolved ?? "")}
          onChange={(v) => onChange(v || undefined)}
          language="javascript"
          placeholder={schema["ui:placeholder"] || "// Enter code..."}
          disabled={disabled}
          minHeight={compact ? 120 : 200}
        />
      </FieldWrapper>
    );
  }

  if (widget === "code-finder") {
    return (
      <CodeFinderEditor
        label={label}
        description={description}
        value={
          resolved as
            | {
                rules: Array<{ pattern: string }>;
                sample?: string;
                useAllRulesWhenTesting?: boolean;
              }
            | undefined
        }
        presets={schema["ui:presets"]}
        onChange={onChange}
        disabled={disabled}
      />
    );
  }

  if (widget === "simplifier-rules") {
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <CodeInput
          value={String(resolved ?? "")}
          onChange={(v) => onChange(v || undefined)}
          language="simplifier-rules"
          placeholder={'if TYPE = "b";\nif TAG_TYPE = STANDALONE;'}
          disabled={disabled}
          minHeight={60}
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

  if (widget === "regex") {
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <CodeInput
          value={String(resolved ?? "")}
          onChange={(v) => onChange(v || undefined)}
          language="regex"
          placeholder={schema["ui:placeholder"] || "pattern..."}
          disabled={disabled}
          singleLine
          className="text-xs"
        />
      </FieldWrapper>
    );
  }

  if (widget === "tags") {
    const tagString = String(resolved ?? "");
    const tags = tagString
      ? tagString
          .split(",")
          .map((t) => t.trim())
          .filter(Boolean)
      : [];
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <TagInput
          value={tags}
          onChange={(newTags) => onChange(newTags.length > 0 ? newTags.join(", ") : undefined)}
          placeholder={schema["ui:placeholder"] || "Add tag..."}
          disabled={disabled}
        />
      </FieldWrapper>
    );
  }

  if (widget === "number-list") {
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <NumberListEditor
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"]}
          disabled={disabled}
          onChange={onChange}
        />
      </FieldWrapper>
    );
  }

  if (widget === "credential-picker") {
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <CredentialPicker
          schema={schema}
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"]}
          disabled={disabled}
          onChange={onChange}
        />
      </FieldWrapper>
    );
  }

  if (
    widget === "path" ||
    widget === "file-picker" ||
    widget === "folder" ||
    widget === "folder-picker"
  ) {
    const kind = widget === "folder" || widget === "folder-picker" ? "directory" : "file";
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <PathPicker
          kind={kind}
          name={name}
          schema={schema}
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"]}
          disabled={disabled}
          onChange={onChange}
        />
      </FieldWrapper>
    );
  }

  if (widget === "textarea") {
    const textMeta = editor?.text;
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <textarea
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"] || ""}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value || undefined)}
          rows={textMeta?.height || 4}
          className={cn(
            "w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs",
            "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
            "resize-y",
            disabled && "opacity-50",
          )}
        />
      </FieldWrapper>
    );
  }

  if (widget === "password") {
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <Input
          type="password"
          value={String(resolved ?? "")}
          placeholder={schema["ui:placeholder"]}
          disabled={disabled}
          className="text-xs h-8"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            onChange(e.target.value || undefined)
          }
        />
      </FieldWrapper>
    );
  }

  const widgetOpts = schema["ui:widget-options"] as Record<string, unknown> | undefined;

  if (widget === "checklist" && widgetOpts?.entries) {
    const entries = widgetOpts.entries as Array<{
      name: string;
      title: string;
      description?: string;
    }>;
    const current = (resolved as Record<string, boolean>) ?? {};
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <div className="space-y-1">
          {entries.map((entry) => (
            <label
              key={entry.name}
              className={cn(
                "flex items-center gap-2 cursor-pointer",
                disabled && "opacity-50 cursor-not-allowed",
              )}
            >
              <Switch
                size="sm"
                checked={current[entry.name] ?? false}
                disabled={disabled}
                onCheckedChange={(v: boolean) => onChange({ ...current, [entry.name]: v })}
              />
              <div className="flex-1">
                <span className="text-xs font-medium">{entry.title}</span>
                {entry.description && !compact && (
                  <p className="text-xs text-muted-foreground">{entry.description}</p>
                )}
              </div>
            </label>
          ))}
        </div>
      </FieldWrapper>
    );
  }

  if (widget === "select" && effectiveEnum && effectiveEnum.length > 0) {
    const current = String(resolved ?? "");
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
      >
        <div
          className={cn(
            "border border-input rounded-md max-h-[120px] overflow-y-auto",
            disabled && "opacity-50",
          )}
        >
          {effectiveEnum.map((v) => {
            const val = String(v);
            const isActive = current === val;
            return (
              <button
                key={val}
                type="button"
                disabled={disabled}
                onClick={() => onChange(val)}
                className={cn(
                  "block w-full px-2 py-1 text-left text-xs border-b last:border-b-0 transition-colors",
                  isActive
                    ? "bg-primary text-primary-foreground font-medium"
                    : "hover:bg-accent/50",
                )}
              >
                {getLabel(val)}
              </button>
            );
          })}
        </div>
      </FieldWrapper>
    );
  }

  // -- Type-based dispatch --

  if (schema.type === "boolean") {
    return (
      <FormToggle
        checked={(resolved as boolean) ?? false}
        onCheckedChange={(v) => onChange(v)}
        label={label}
        description={description}
        modified={isModifiedFromPreset}
        disabled={disabled}
        compact={compact}
      />
    );
  }

  if (effectiveEnum && effectiveEnum.length > 0) {
    return (
      <FieldWrapper
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <select
          value={String(resolved ?? "")}
          disabled={disabled}
          onChange={(e) => onChange(e.target.value)}
          className={cn(
            "h-8 w-full rounded-lg border border-input bg-transparent px-2 py-1 text-base md:text-sm",
            "transition-colors outline-none",
            "focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
            "dark:bg-input/30",
            disabled && "opacity-50 pointer-events-none",
          )}
        >
          <option value="">—</option>
          {effectiveEnum.map((v) => {
            const val = String(v);
            return (
              <option key={val} value={val}>
                {getLabel(val)}
              </option>
            );
          })}
        </select>
        {(() => {
          const desc =
            resolved != null ? schema["ui:enum-descriptions"]?.[String(resolved)] : undefined;
          return desc ? (
            <p className="text-xs text-muted-foreground italic mt-0.5">{desc}</p>
          ) : null;
        })()}
      </FieldWrapper>
    );
  }

  if (schema.type === "integer" || schema.type === "number") {
    return (
      <FieldWrapper
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
      >
        <Input
          type="number"
          value={resolved != null ? String(resolved) : ""}
          placeholder={schema.default != null ? String(schema.default) : undefined}
          min={schema.minimum}
          max={schema.maximum}
          step={schema.type === "integer" ? 1 : undefined}
          disabled={disabled}
          className="text-xs h-8"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
            const v = e.target.value;
            onChange(
              v === "" ? undefined : schema.type === "integer" ? parseInt(v) : parseFloat(v),
            );
          }}
        />
      </FieldWrapper>
    );
  }

  if (schema.type === "object") {
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
          disabled={disabled}
        />
      );
    }

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

    return (
      <JsonEditor label={label} description={description} value={resolved} onChange={onChange} />
    );
  }

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
    return (
      <JsonEditor label={label} description={description} value={resolved} onChange={onChange} />
    );
  }

  // Default: string input
  return (
    <FieldWrapper
      label={label}
      description={description}
      compact={compact}
      isModified={isModifiedFromPreset}
      docParam={docParam}
      disabled={disabled}
      error={error}
    >
      <Input
        value={String(resolved ?? "")}
        placeholder={
          schema["ui:placeholder"] || (schema.default != null ? String(schema.default) : undefined)
        }
        disabled={disabled}
        className="text-xs h-8"
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value || undefined)}
      />
    </FieldWrapper>
  );
}
