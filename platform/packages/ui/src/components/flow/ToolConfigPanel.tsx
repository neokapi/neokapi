import * as React from "react";

import { cn } from "../../lib/utils";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Switch } from "../ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { ResourcePicker, type ResourceOption } from "../ResourcePicker";

/** Minimal schema types matching neokapi's ComponentSchema shape. */
export interface PropertySchema {
  type: string;
  title?: string;
  description?: string;
  default?: unknown;
  enum?: string[];
  "x-path"?: {
    type?: "file" | "directory";
    role?: "input" | "output";
    resourceKind?: "tm" | "termbase" | "srx";
    accepts?: string[];
  };
  "ui:visible"?: { field: string; eq: unknown };
}

export interface ComponentSchema {
  $id?: string;
  title: string;
  description?: string;
  properties: Record<string, PropertySchema>;
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

export function ToolConfigPanel({
  schema,
  config,
  onChange,
  resources = {},
  resourceContext,
  readOnly = false,
  className,
}: ToolConfigPanelProps) {
  const updateField = (key: string, value: unknown) => {
    onChange({ ...config, [key]: value });
  };

  const entries = Object.entries(schema.properties);

  return (
    <div className={cn("flex flex-col gap-4", className)}>
      {entries.map(([key, prop]) => {
        // Conditional visibility.
        if (prop["ui:visible"]) {
          const { field, eq } = prop["ui:visible"];
          if (config[field] !== eq) return null;
        }

        const value = config[key] ?? prop.default;
        const label = prop.title || key;

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
              disabled={readOnly}
            />
          );
        }

        // Boolean → Switch.
        if (prop.type === "boolean") {
          return (
            <div key={key} className="flex items-center justify-between gap-3">
              <div className="flex flex-col">
                <Label className="text-sm">{label}</Label>
                {prop.description && (
                  <span className="text-xs text-muted-foreground">{prop.description}</span>
                )}
              </div>
              <Switch
                checked={!!value}
                onCheckedChange={(v) => updateField(key, v)}
                disabled={readOnly}
              />
            </div>
          );
        }

        // String with enum → Select.
        if (prop.type === "string" && prop.enum) {
          return (
            <div key={key} className="flex flex-col gap-1.5">
              <Label>{label}</Label>
              <Select
                value={(value as string) || ""}
                onValueChange={(v) => updateField(key, v)}
                disabled={readOnly}
              >
                <SelectTrigger>
                  <SelectValue placeholder="Select..." />
                </SelectTrigger>
                <SelectContent>
                  {prop.enum.map((opt) => (
                    <SelectItem key={opt} value={opt}>
                      {opt}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
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
                onChange={(e) => updateField(key, e.target.value)}
                placeholder={prop.description}
                disabled={readOnly}
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
                onChange={(e) => updateField(key, parseInt(e.target.value, 10))}
                placeholder={prop.description}
                disabled={readOnly}
              />
            </div>
          );
        }

        return null;
      })}
    </div>
  );
}
