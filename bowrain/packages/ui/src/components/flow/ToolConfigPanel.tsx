import {
  Input,
  Label,
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
  Switch,
  cn,
} from "@neokapi/ui-primitives";
import * as React from "react";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";

import { ResourcePicker, type ResourceOption } from "../ResourcePicker";

// These are intentionally minimal, self-contained schema types — NOT the shared
// @neokapi/contract-types ones (issue #817). `ConditionExpr` here is a flat
// optional interface that `evalCondition` below reads by direct field access;
// the shared `ConditionExpr` is a discriminated union, which `evalCondition`
// could not consume without narrowing. Keep these local until this panel is
// reworked to evaluate the shared union form.

/** Option item with typed value and label. */
export interface OptionItem {
  value: unknown;
  label: string;
}

/** Condition expression for conditional visibility. */
export interface ConditionExpr {
  field?: string;
  eq?: unknown;
  empty?: boolean;
  not?: ConditionExpr;
  all?: ConditionExpr[];
  any?: ConditionExpr[];
}

/** Minimal schema types matching neokapi's ComponentSchema shape. */
export interface PropertySchema {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  options?: OptionItem[];
  properties?: Record<string, PropertySchema>;
  items?: PropertySchema;
  "x-path"?: {
    type?: "file" | "directory";
    role?: "input" | "output";
    resourceKind?: "tm" | "termbase" | "srx";
    accepts?: string[];
    browseTitle?: string;
    forSaveAs?: boolean;
    filters?: { name: string; extensions: string }[];
  };
  "ui:visible"?: ConditionExpr;
  "ui:enabled"?: ConditionExpr;
  "ui:widget"?: string;
}

export interface ComponentSchema {
  $id?: string;
  title: string;
  description?: string;
  properties: Record<string, PropertySchema>;
  "ui:groups"?: { id: string; label: string; fields?: string[] }[];
}

export interface ToolConfigPanelProps {
  /** The step's JSON Schema with properties and x-path annotations. */
  schema: ComponentSchema;
  /** Current config values. */
  config: Record<string, unknown>;
  /** Called when any config value changes. */
  onChange: (config: Record<string, unknown>) => void;
  /** Available named resources for ResourcePicker dropdowns. */
  resources?: Record<string, ResourceOption[]>;
  /** Resource context for resolving/previewing paths. */
  resourceContext?: {
    projectDir: string;
    outputDir: string;
  };
  /** Read-only mode. */
  readOnly?: boolean;
  /** Additional class name. */
  className?: string;
}

/** Evaluate a ConditionExpr against current config values. */
function evalCondition(cond: ConditionExpr | undefined, config: Record<string, unknown>): boolean {
  if (!cond) return true;
  if (cond.not) return !evalCondition(cond.not, config);
  if (cond.all) return cond.all.every((c) => evalCondition(c, config));
  if (cond.any) return cond.any.some((c) => evalCondition(c, config));
  if (cond.field) {
    const val = config[cond.field];
    if (cond.empty !== undefined) return cond.empty ? !val : !!val;
    return val === cond.eq;
  }
  return true;
}

function renderProperty(
  key: string,
  prop: PropertySchema,
  value: unknown,
  updateField: (key: string, value: unknown) => void,
  config: Record<string, unknown>,
  resources: Record<string, ResourceOption[]>,
  readOnly: boolean,
) {
  const label = prop.title || key;

  // Conditional visibility
  if (prop["ui:visible"] && !evalCondition(prop["ui:visible"], config)) {
    return null;
  }

  // Conditional enablement
  const enabled = prop["ui:enabled"] ? evalCondition(prop["ui:enabled"], config) : true;
  const disabled = readOnly || !enabled;

  // x-path annotation → ResourcePicker.
  if (prop["x-path"]) {
    const xPath = prop["x-path"];
    const kind = xPath.resourceKind;
    return (
      <ResourcePicker
        key={key}
        value={(value as string) || ""}
        onChange={(v) => updateField(key, v)}
        resourceKind={kind}
        pathType={xPath.type}
        role={xPath.role}
        resources={kind ? resources[kind] : undefined}
        label={label}
        disabled={disabled}
      />
    );
  }

  // Boolean → Switch.
  if (prop.type === "boolean") {
    return (
      <div
        key={key}
        className={cn("flex items-center justify-between gap-3", disabled && "opacity-50")}
      >
        <div className="flex flex-col">
          <Label className="text-sm">{label}</Label>
          {prop.description && (
            <span className="text-xs text-muted-foreground">{prop.description}</span>
          )}
        </div>
        <Switch
          checked={!!value}
          onCheckedChange={(v: boolean) => updateField(key, v)}
          disabled={disabled}
        />
      </div>
    );
  }

  // Options (labeled enum) → Select.
  if (prop.options && prop.options.length > 0) {
    return (
      <div key={key} className="flex flex-col gap-1.5">
        <Label>{label}</Label>
        <Select
          value={String(value ?? "")}
          onValueChange={(v: string) => updateField(key, v)}
          disabled={disabled}
        >
          <SelectTrigger>
            <SelectValue placeholder="Select..." />
          </SelectTrigger>
          <SelectContent>
            {prop.options.map((opt) => (
              <SelectItem key={String(opt.value)} value={String(opt.value)}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {prop.description && (
          <span className="text-xs text-muted-foreground">{prop.description}</span>
        )}
      </div>
    );
  }

  // String → Input.
  if (prop.type === "string") {
    return (
      <div key={key} className="flex flex-col gap-1.5">
        <Label>{label}</Label>
        <Input
          value={(value as string) || ""}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => updateField(key, e.target.value)}
          placeholder={prop.description}
          disabled={disabled}
        />
      </div>
    );
  }

  // Integer → Input[type=number].
  if (prop.type === "integer" || prop.type === "number") {
    return (
      <div key={key} className="flex flex-col gap-1.5">
        <Label>{label}</Label>
        <Input
          type="number"
          value={value !== undefined ? String(value) : ""}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
            updateField(key, parseInt(e.target.value, 10))
          }
          placeholder={prop.description}
          disabled={disabled}
        />
      </div>
    );
  }

  return null;
}

export function ToolConfigPanel({
  schema,
  config,
  onChange,
  resources = {},
  readOnly = false,
  className,
}: ToolConfigPanelProps) {
  const [collapsedSections, setCollapsedSections] = React.useState<Set<string>>(new Set());

  const updateField = (key: string, value: unknown) => {
    onChange({ ...config, [key]: value });
  };

  const toggleSection = (key: string) => {
    setCollapsedSections((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const entries = Object.entries(schema.properties);

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {entries.map(([key, prop]) => {
        // Nested object → collapsible section
        if (prop.type === "object" && prop.properties && Object.keys(prop.properties).length > 0) {
          const isCollapsed = collapsedSections.has(key);
          return (
            <div key={key} className="rounded-md border">
              <button
                type="button"
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm font-medium hover:bg-accent/30"
                onClick={() => toggleSection(key)}
              >
                {isCollapsed ? (
                  <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" />
                ) : (
                  <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" />
                )}
                <span>{prop.title || key}</span>
                {prop.description && (
                  <span className="ml-auto text-xs font-normal text-muted-foreground">
                    {prop.description}
                  </span>
                )}
              </button>
              {!isCollapsed && (
                <div className="flex flex-col gap-3 border-t px-3 py-3">
                  {Object.entries(prop.properties).map(([childKey, childProp]) =>
                    renderProperty(
                      childKey,
                      childProp,
                      (config[key] as Record<string, unknown>)?.[childKey] ?? childProp.default,
                      (childKey2, childValue) => {
                        const parent = (config[key] as Record<string, unknown>) || {};
                        onChange({
                          ...config,
                          [key]: { ...parent, [childKey2]: childValue },
                        });
                      },
                      (config[key] as Record<string, unknown>) || {},
                      resources,
                      readOnly,
                    ),
                  )}
                </div>
              )}
            </div>
          );
        }

        // Leaf property
        const value = config[key] ?? prop.default;
        return renderProperty(key, prop, value, updateField, config, resources, readOnly);
      })}
    </div>
  );
}
