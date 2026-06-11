import { FormToggle } from "../ui/form";
import type { PropertySchema, ToolDocParam } from "./types";
import { evaluateCondition } from "./hooks/useConditionalVisibility";
import { useFieldEnabled } from "./hooks/useFieldEnabled";
import { usePresetComparison } from "./hooks/usePresetComparison";
import { resolveWidgetName } from "./registry";
import { resolveRef, hasAdditionalProperties, humanizeKey } from "./utils";
import { enumOptionLabel } from "./field-helpers";
import { FieldWrapper } from "./primitives/FieldWrapper";
import { CodeFinderEditor } from "../ui/code-finder-editor";
import { MapEditor } from "./widgets/MapEditor";
import { ArrayEditor } from "./widgets/ArrayEditor";
import { NestedObjectEditor } from "./widgets/NestedObjectEditor";
import { JsonEditor } from "./widgets/JsonEditor";
import { NumberListEditor } from "./widgets/NumberListEditor";
import { PathPicker } from "./widgets/PathPicker";
import { CredentialPicker } from "./widgets/CredentialPicker";
import { SegmentedField } from "./widgets/SegmentedField";
import { CodeInputField } from "./widgets/CodeInputField";
import { TagsField } from "./widgets/TagsField";
import { TextareaField } from "./widgets/TextareaField";
import { PasswordField } from "./widgets/PasswordField";
import { ChecklistField, type ChecklistEntry } from "./widgets/ChecklistField";
import { SelectListField } from "./widgets/SelectListField";
import { EnumSelectField } from "./widgets/EnumSelectField";
import { NumberField } from "./widgets/NumberField";
import { TextField } from "./widgets/TextField";

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

  const label = schema.title || humanizeKey(name);
  const resolved = value ?? schema.default;
  const widget = resolveWidgetName(schema["ui:widget"]);
  const options = schema.options;
  const enumLabels = schema["ui:enum-labels"];
  const effectiveEnum = options ? options.map((o) => o.value) : schema.enum;
  const getLabel = (val: unknown): string => enumOptionLabel(val, options, enumLabels);

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
    return (
      <SegmentedField
        values={effectiveEnum}
        current={String(resolved ?? effectiveEnum[0])}
        disabled={disabled}
        getLabel={getLabel}
        onChange={onChange}
      />
    );
  }

  if (widget === "code-editor") {
    return (
      <CodeInputField
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        language="javascript"
        placeholder={schema["ui:placeholder"] || "// Enter code..."}
        minHeight={compact ? 120 : 200}
        onChange={onChange}
      />
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
      <CodeInputField
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        language="simplifier-rules"
        placeholder={'if TYPE = "b";\nif TAG_TYPE = STANDALONE;'}
        minHeight={60}
        onChange={onChange}
      />
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
      <CodeInputField
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        language="regex"
        placeholder={schema["ui:placeholder"] || "pattern..."}
        singleLine
        inputClassName="text-xs"
        onChange={onChange}
      />
    );
  }

  if (widget === "tags") {
    return (
      <TagsField
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        placeholder={schema["ui:placeholder"]}
        onChange={onChange}
      />
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
    return (
      <TextareaField
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        placeholder={schema["ui:placeholder"]}
        height={editor?.text?.height}
        onChange={onChange}
      />
    );
  }

  if (widget === "password") {
    return (
      <PasswordField
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
        value={String(resolved ?? "")}
        placeholder={schema["ui:placeholder"]}
        onChange={onChange}
      />
    );
  }

  const widgetOpts = schema["ui:widget-options"] as Record<string, unknown> | undefined;

  if (widget === "checklist" && widgetOpts?.entries) {
    return (
      <ChecklistField
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
        entries={widgetOpts.entries as ChecklistEntry[]}
        value={resolved as Record<string, boolean> | undefined}
        onChange={onChange}
      />
    );
  }

  if (widget === "select" && effectiveEnum && effectiveEnum.length > 0) {
    return (
      <SelectListField
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        vertical={verticalLayout}
        disabled={disabled}
        error={error}
        values={effectiveEnum}
        current={String(resolved ?? "")}
        getLabel={getLabel}
        onChange={onChange}
      />
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
      <EnumSelectField
        label={showLabel ? label : ""}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        values={effectiveEnum}
        value={resolved}
        getLabel={getLabel}
        enumDescriptions={schema["ui:enum-descriptions"]}
        onChange={onChange}
      />
    );
  }

  if (schema.type === "integer" || schema.type === "number") {
    return (
      <NumberField
        label={label}
        description={description}
        compact={compact}
        isModified={isModifiedFromPreset}
        docParam={docParam}
        disabled={disabled}
        error={error}
        value={resolved != null ? String(resolved) : ""}
        placeholder={schema.default != null ? String(schema.default) : undefined}
        min={schema.minimum}
        max={schema.maximum}
        type={schema.type}
        onChange={onChange}
      />
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
    <TextField
      label={label}
      description={description}
      compact={compact}
      isModified={isModifiedFromPreset}
      docParam={docParam}
      disabled={disabled}
      error={error}
      value={String(resolved ?? "")}
      placeholder={
        schema["ui:placeholder"] || (schema.default != null ? String(schema.default) : undefined)
      }
      onChange={onChange}
    />
  );
}
