import { useState, useCallback, useMemo } from "react";
import { cn } from "@neokapi/ui-primitives";
import {
  ComponentSchema,
  FilterSchema,
  ParameterGroup,
  PropertySchema,
  FilterParamsValue,
} from "./types";

// UI components from the ui directory
import { Button } from "@neokapi/ui-primitives/components/ui/button";
import { Input } from "@neokapi/ui-primitives/components/ui/input";
import { Label } from "@neokapi/ui-primitives/components/ui/label";
import { Switch } from "@neokapi/ui-primitives/components/ui/switch";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@neokapi/ui-primitives/components/ui/collapsible";
import { ChevronDown, ChevronRight } from "../icons";

interface FilterConfigEditorProps {
  /** The filter or tool schema */
  schema: FilterSchema | ComponentSchema;
  /** Current parameter values */
  value: FilterParamsValue;
  /** Called when any parameter changes */
  onChange: (params: FilterParamsValue) => void;
  /** Optional CSS class */
  className?: string;
}

/**
 * FilterConfigEditor renders a dynamic form for filter or tool parameters
 * based on the JSON Schema with x-groups and x-widget extensions.
 * Also exported as SchemaConfigEditor for tool use cases.
 */
export function FilterConfigEditor({
  schema,
  value,
  onChange,
  className,
}: FilterConfigEditorProps) {
  const groups = schema["ui:groups"] ?? [];
  const properties = schema.properties ?? {};

  // Track which groups are collapsed
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => {
    const initial = new Set<string>();
    groups.forEach((g) => {
      if (g.collapsed) initial.add(g.id);
    });
    return initial;
  });

  const toggleGroup = useCallback((groupId: string) => {
    setCollapsedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(groupId)) {
        next.delete(groupId);
      } else {
        next.add(groupId);
      }
      return next;
    });
  }, []);

  // Identify ungrouped fields
  const groupedFields = useMemo(() => {
    const set = new Set<string>();
    groups.forEach((g) => g.fields.forEach((f) => set.add(f)));
    return set;
  }, [groups]);

  const ungroupedFields = useMemo(() => {
    return Object.keys(properties).filter((f) => !groupedFields.has(f));
  }, [properties, groupedFields]);

  const handleChange = useCallback(
    (fieldName: string, fieldValue: unknown) => {
      onChange({ ...value, [fieldName]: fieldValue });
    },
    [value, onChange],
  );

  return (
    <div className={cn("space-y-4", className)}>
      {/* Grouped parameters */}
      {groups.map((group) => (
        <ParameterGroupSection
          key={group.id}
          group={group}
          properties={properties}
          values={value}
          collapsed={collapsedGroups.has(group.id)}
          onToggle={() => toggleGroup(group.id)}
          onChange={handleChange}
        />
      ))}

      {/* Ungrouped parameters */}
      {ungroupedFields.length > 0 && (
        <div className="space-y-3">
          <h3 className="text-sm font-medium text-muted-foreground">Other Parameters</h3>
          {ungroupedFields.map((fieldName) => (
            <ParameterField
              key={fieldName}
              name={fieldName}
              schema={properties[fieldName]}
              value={value[fieldName]}
              onChange={(v) => handleChange(fieldName, v)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

interface ParameterGroupSectionProps {
  group: ParameterGroup;
  properties: Record<string, PropertySchema>;
  values: FilterParamsValue;
  collapsed: boolean;
  onToggle: () => void;
  onChange: (field: string, value: unknown) => void;
}

function ParameterGroupSection({
  group,
  properties,
  values,
  collapsed,
  onToggle,
  onChange,
}: ParameterGroupSectionProps) {
  return (
    <Collapsible open={!collapsed} onOpenChange={() => onToggle()}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground text-muted-foreground w-full"
        >
          {collapsed ? <ChevronRight className="h-4 w-4" /> : <ChevronDown className="h-4 w-4" />}
          {group.label}
        </button>
      </CollapsibleTrigger>

      <CollapsibleContent className="mt-2 ml-6 space-y-3">
        {group.description && (
          <p className="text-xs text-muted-foreground mb-2">{group.description}</p>
        )}
        {group.fields.map((fieldName) => {
          const schema = properties[fieldName];
          if (!schema) return null;
          return (
            <ParameterField
              key={fieldName}
              name={fieldName}
              schema={schema}
              value={values[fieldName]}
              onChange={(v) => onChange(fieldName, v)}
            />
          );
        })}
      </CollapsibleContent>
    </Collapsible>
  );
}

interface ParameterFieldProps {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
}

function ParameterField({ name, schema, value, onChange }: ParameterFieldProps) {
  const widget = schema["ui:widget"];

  // Handle different types and widgets
  if (schema.type === "boolean") {
    return (
      <BooleanField
        name={name}
        description={schema.description}
        value={value as boolean | undefined}
        defaultValue={schema.default as boolean | undefined}
        onChange={onChange}
      />
    );
  }

  if (widget === "regexBuilder" || widget === "tagList") {
    return (
      <TextField
        name={name}
        description={schema.description}
        placeholder={schema["ui:placeholder"]}
        value={value as string | undefined}
        defaultValue={schema.default as string | undefined}
        onChange={onChange}
      />
    );
  }

  if (widget === "codeFinderRules") {
    return (
      <CodeFinderRulesField
        name={name}
        description={schema.description}
        value={value as Record<string, unknown> | undefined}
        presets={schema["ui:presets"]}
        onChange={onChange}
      />
    );
  }

  // Default string/number field
  if (schema.type === "string" || schema.type === "integer" || schema.type === "number") {
    return (
      <TextField
        name={name}
        description={schema.description}
        placeholder={schema["ui:placeholder"]}
        value={value as string | number | undefined}
        defaultValue={schema.default as string | number | undefined}
        onChange={onChange}
        type={schema.type === "integer" || schema.type === "number" ? "number" : "text"}
      />
    );
  }

  // Object type - show as JSON for now
  if (schema.type === "object") {
    return (
      <JsonField
        name={name}
        description={schema.description}
        value={value as Record<string, unknown> | undefined}
        onChange={onChange}
      />
    );
  }

  return null;
}

// Individual field components

interface BooleanFieldProps {
  name: string;
  description?: string;
  value: boolean | undefined;
  defaultValue?: boolean;
  onChange: (value: boolean) => void;
}

function BooleanField({ name, description, value, defaultValue, onChange }: BooleanFieldProps) {
  const checked = value ?? defaultValue ?? false;
  return (
    <div className="flex items-center justify-between">
      <Label htmlFor={name} className="text-sm">
        {description || name}
      </Label>
      <Switch id={name} checked={checked} onCheckedChange={onChange} />
    </div>
  );
}

interface TextFieldProps {
  name: string;
  description?: string;
  placeholder?: string;
  value: string | number | undefined;
  defaultValue?: string | number;
  type?: "text" | "number";
  onChange: (value: string | number) => void;
}

function TextField({
  name,
  description,
  placeholder,
  value,
  defaultValue,
  type = "text",
  onChange,
}: TextFieldProps) {
  const displayValue = value ?? defaultValue ?? "";
  return (
    <div className="space-y-1">
      <Label htmlFor={name} className="text-sm">
        {description || name}
      </Label>
      <Input
        id={name}
        type={type}
        value={displayValue}
        placeholder={placeholder}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
          const v = type === "number" ? Number(e.target.value) : e.target.value;
          onChange(v);
        }}
      />
    </div>
  );
}

interface JsonFieldProps {
  name: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  onChange: (value: Record<string, unknown>) => void;
}

function JsonField({ name, description, value, onChange }: JsonFieldProps) {
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
    <div className="space-y-1">
      <Label htmlFor={name} className="text-sm">
        {description || name}
      </Label>
      <textarea
        id={name}
        className={cn(
          "w-full min-h-[80px] p-2 text-xs font-mono rounded border",
          "bg-background border-input focus:border-ring focus:outline-none",
          error && "border-destructive",
        )}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onBlur={handleBlur}
      />
      {error && <p className="text-xs text-destructive">{error}</p>}
    </div>
  );
}

interface CodeFinderRulesFieldProps {
  name: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  presets?: Record<string, unknown>;
  onChange: (value: Record<string, unknown>) => void;
}

function CodeFinderRulesField({
  name,
  description,
  value,
  presets,
  onChange,
}: CodeFinderRulesFieldProps) {
  const [showPresets, setShowPresets] = useState(false);

  const rules = (value?.rules as Array<{ pattern: string }>) ?? [];
  const sample = (value?.sample as string) ?? "";

  const handleAddRule = useCallback(() => {
    onChange({
      ...value,
      rules: [...rules, { pattern: "" }],
    });
  }, [value, rules, onChange]);

  const handleRemoveRule = useCallback(
    (index: number) => {
      const newRules = [...rules];
      newRules.splice(index, 1);
      onChange({ ...value, rules: newRules });
    },
    [value, rules, onChange],
  );

  const handleRuleChange = useCallback(
    (index: number, pattern: string) => {
      const newRules = [...rules];
      newRules[index] = { pattern };
      onChange({ ...value, rules: newRules });
    },
    [value, rules, onChange],
  );

  const handleSampleChange = useCallback(
    (newSample: string) => {
      onChange({ ...value, sample: newSample });
    },
    [value, onChange],
  );

  const handleApplyPreset = useCallback(
    (presetName: string) => {
      const preset = presets?.[presetName] as Record<string, unknown>;
      if (preset) {
        onChange(preset);
      }
      setShowPresets(false);
    },
    [presets, onChange],
  );

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{description || name}</Label>
        {presets && Object.keys(presets).length > 0 && (
          <div className="relative">
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setShowPresets(!showPresets)}
            >
              Presets
            </Button>
            {showPresets && (
              <div className="absolute right-0 mt-1 bg-popover border border-border rounded shadow-lg z-10">
                {Object.keys(presets).map((presetName) => (
                  <button
                    key={presetName}
                    type="button"
                    className="block w-full px-3 py-1.5 text-left text-sm hover:bg-accent"
                    onClick={() => handleApplyPreset(presetName)}
                  >
                    {presetName}
                  </button>
                ))}
              </div>
            )}
          </div>
        )}
      </div>

      <div className="space-y-2 ml-2">
        {rules.map((rule, index) => (
          <div key={index} className="flex items-center gap-2">
            <Input
              value={rule.pattern}
              placeholder="Regex pattern"
              className="flex-1 font-mono text-xs"
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => handleRuleChange(index, e.target.value)}
            />
            <Button type="button" variant="ghost" size="sm" onClick={() => handleRemoveRule(index)}>
              ✕
            </Button>
          </div>
        ))}
        <Button type="button" variant="outline" size="sm" onClick={handleAddRule}>
          + Add Rule
        </Button>
      </div>

      <div className="mt-2">
        <Label className="text-xs text-muted-foreground">Sample Text</Label>
        <Input
          value={sample}
          placeholder="Sample text to test patterns"
          className="mt-1"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => handleSampleChange(e.target.value)}
        />
      </div>
    </div>
  );
}

/** Alias for tool/step config editing use cases. */
export const SchemaConfigEditor = FilterConfigEditor;

export default FilterConfigEditor;
