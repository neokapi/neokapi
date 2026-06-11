import { useState, useCallback } from "react";
import { cn } from "../../lib/utils";
import { PropertySchema } from "./types";

// UI components from the ui directory
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "../ui/collapsible";
import { ChevronRight } from "../icons";

import { BooleanField, TextField, JsonField } from "./fields";
import { CodeFinderRulesField } from "./CodeFinderRulesField";
import { resolveAdditionalProperties, hasAdditionalProperties } from "./schema-utils";

// ParameterField and the container fields below (NestedObjectField, MapField,
// ArrayField) are mutually recursive, so they live in the same module.

interface ParameterFieldProps {
  name: string;
  schema: PropertySchema;
  value: unknown;
  onChange: (value: unknown) => void;
  allValues?: Record<string, unknown>;
  depth?: number;
}

export function ParameterField({
  name,
  schema,
  value,
  onChange,
  allValues,
  depth = 0,
}: ParameterFieldProps) {
  const widget = schema["ui:widget"];

  // ui:visible conditional visibility
  const cond = schema["ui:visible"];
  if (cond && allValues && "field" in cond && "eq" in cond) {
    if (allValues[cond.field] !== cond.eq) return null;
  }

  // ── x-widget dispatch ──

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

  if (widget === "simplifierRulesEditor") {
    return (
      <div className="space-y-1">
        <Label htmlFor={name} className="text-sm">
          {name}
        </Label>
        <textarea
          id={name}
          className="w-full min-h-[60px] p-2 text-xs font-mono rounded border bg-background border-input focus:border-ring focus:outline-none resize-y"
          value={String(value ?? schema.default ?? "")}
          placeholder={schema["ui:placeholder"] || "One rule per line..."}
          onChange={(e) => onChange(e.target.value || undefined)}
        />
        {schema.description && (
          <p className="text-xs text-muted-foreground">{schema.description}</p>
        )}
      </div>
    );
  }

  if (widget === "elementRulesEditor" || widget === "attributeRulesEditor") {
    return (
      <MapField
        name={name}
        description={schema.description}
        value={value as Record<string, unknown> | undefined}
        itemSchema={resolveAdditionalProperties(schema)}
        onChange={onChange}
        depth={depth}
        keyPlaceholder={widget === "elementRulesEditor" ? "element name" : "attribute name"}
      />
    );
  }

  if (widget === "regexBuilder" || widget === "tagList") {
    return (
      <TextField
        name={name}
        description={schema.description}
        placeholder={
          schema["ui:placeholder"] || (widget === "tagList" ? "tag1, tag2, ..." : "pattern...")
        }
        value={value as string | undefined}
        defaultValue={schema.default as string | undefined}
        onChange={onChange}
        mono={widget === "regexBuilder"}
      />
    );
  }

  if (widget === "numberList") {
    return (
      <TextField
        name={name}
        description={schema.description}
        placeholder={schema["ui:placeholder"] || "1, 2, 3, ..."}
        value={value as string | undefined}
        defaultValue={schema.default as string | undefined}
        onChange={onChange}
      />
    );
  }

  // ── Type-based dispatch ──

  if (schema.type === "string" || schema.type === "integer" || schema.type === "number") {
    if (schema.enum && schema.enum.length > 0) {
      return (
        <div className="space-y-1">
          <Label htmlFor={name} className="text-sm">
            {name}
          </Label>
          <select
            id={name}
            className="w-full h-9 px-3 text-sm rounded border bg-background border-input focus:border-ring focus:outline-none"
            value={String(value ?? schema.default ?? "")}
            onChange={(e) => onChange(e.target.value)}
          >
            <option value="">—</option>
            {schema.enum.map((v) => (
              <option key={String(v)} value={String(v)}>
                {String(v)}
              </option>
            ))}
          </select>
          {schema.description && (
            <p className="text-xs text-muted-foreground">{schema.description}</p>
          )}
        </div>
      );
    }

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

  // ── Object: nested sub-form vs map vs JSON fallback ──

  if (schema.type === "object") {
    if (schema.properties && Object.keys(schema.properties).length > 0) {
      return (
        <NestedObjectField
          name={name}
          description={schema.description}
          schema={schema}
          value={value as Record<string, unknown> | undefined}
          onChange={onChange}
          depth={depth}
        />
      );
    }

    if (hasAdditionalProperties(schema)) {
      return (
        <MapField
          name={name}
          description={schema.description}
          value={value as Record<string, unknown> | undefined}
          itemSchema={resolveAdditionalProperties(schema)}
          onChange={onChange}
          depth={depth}
        />
      );
    }

    return (
      <JsonField
        name={name}
        description={schema.description}
        value={value as Record<string, unknown> | undefined}
        onChange={onChange}
      />
    );
  }

  // ── Array ──

  if (schema.type === "array") {
    if (schema.items) {
      return (
        <ArrayField
          name={name}
          description={schema.description}
          itemSchema={schema.items}
          value={value as unknown[] | undefined}
          onChange={onChange}
          depth={depth}
        />
      );
    }

    return (
      <JsonField
        name={name}
        description={schema.description}
        value={value as unknown[] | undefined}
        onChange={onChange}
      />
    );
  }

  return null;
}

// ─── Nested Object Field ────────────────────────────────────

function NestedObjectField({
  name,
  description,
  schema,
  value,
  onChange,
  depth,
}: {
  name: string;
  description?: string;
  schema: PropertySchema;
  value: Record<string, unknown> | undefined;
  onChange: (value: unknown) => void;
  depth: number;
}) {
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
    <Collapsible defaultOpen={depth === 0}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground text-muted-foreground w-full"
        >
          <ChevronRight className="h-4 w-4 transition-transform data-[state=open]:rotate-90" />
          {name}
          <span className="text-xs text-muted-foreground ml-auto">{keys.length}</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2 ml-4 pl-2 border-l border-border space-y-3">
        {description && <p className="text-xs text-muted-foreground">{description}</p>}
        {keys.map((key) => (
          <ParameterField
            key={key}
            name={key}
            schema={properties[key]}
            value={current[key]}
            onChange={(v) => handleFieldChange(key, v)}
            allValues={current}
            depth={depth + 1}
          />
        ))}
      </CollapsibleContent>
    </Collapsible>
  );
}

// ─── Map Field (dynamic key-value) ──────────────────────────

function MapField({
  name,
  description,
  value,
  itemSchema,
  onChange,
  depth,
  keyPlaceholder = "key",
}: {
  name: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  itemSchema?: PropertySchema;
  onChange: (value: unknown) => void;
  depth: number;
  keyPlaceholder?: string;
}) {
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
    <Collapsible>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground text-muted-foreground w-full"
        >
          <ChevronRight className="h-4 w-4 transition-transform data-[state=open]:rotate-90" />
          {name}
          <span className="text-xs text-muted-foreground ml-auto">{entries.length}</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2 ml-4 pl-2 border-l border-border space-y-2">
        {description && <p className="text-xs text-muted-foreground">{description}</p>}

        {entries.map(([key, val]) => (
          <div key={key} className="space-y-1">
            <div className="flex items-center gap-2">
              <span className="text-xs font-mono font-semibold text-muted-foreground">{key}</span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="ml-auto h-6 w-6 p-0"
                onClick={() => handleRemove(key)}
              >
                ✕
              </Button>
            </div>
            {itemSchema?.properties ? (
              <div className="ml-2 pl-2 border-l border-border space-y-2">
                {Object.entries(itemSchema.properties)
                  .filter(([, s]) => !s.deprecated)
                  .map(([fKey, fSchema]) => (
                    <ParameterField
                      key={fKey}
                      name={fKey}
                      schema={fSchema}
                      value={(val as Record<string, unknown>)?.[fKey]}
                      onChange={(v) =>
                        handleEntryChange(key, { ...(val as Record<string, unknown>), [fKey]: v })
                      }
                      allValues={val as Record<string, unknown>}
                      depth={depth + 1}
                    />
                  ))}
              </div>
            ) : (
              <JsonField
                name={`${name}-${key}`}
                value={val as Record<string, unknown>}
                onChange={(v) => handleEntryChange(key, v)}
              />
            )}
          </div>
        ))}

        {/* Add entry */}
        <div className="flex items-center gap-2">
          <Input
            value={newKey}
            placeholder={keyPlaceholder}
            className="flex-1 text-xs h-7"
            onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewKey(e.target.value)}
            onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => {
              if (e.key === "Enter") handleAdd();
            }}
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-7"
            disabled={!newKey.trim() || newKey.trim() in current}
            onClick={handleAdd}
          >
            + Add
          </Button>
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

// ─── Array Field ────────────────────────────────────────────

function ArrayField({
  name,
  description,
  itemSchema,
  value,
  onChange,
  depth,
}: {
  name: string;
  description?: string;
  itemSchema: PropertySchema;
  value: unknown[] | undefined;
  onChange: (value: unknown) => void;
  depth: number;
}) {
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
    (index: number) => onChange(items.filter((_, i) => i !== index)),
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

  return (
    <Collapsible defaultOpen={depth === 0}>
      <CollapsibleTrigger asChild>
        <button
          type="button"
          className="flex items-center gap-2 text-sm font-medium hover:text-foreground text-muted-foreground w-full"
        >
          <ChevronRight className="h-4 w-4 transition-transform data-[state=open]:rotate-90" />
          {name}
          <span className="text-xs text-muted-foreground ml-auto">{items.length}</span>
        </button>
      </CollapsibleTrigger>
      <CollapsibleContent className="mt-2 ml-4 pl-2 border-l border-border space-y-2">
        {description && <p className="text-xs text-muted-foreground">{description}</p>}

        {items.map((item, index) => (
          <div key={index} className="flex items-start gap-2">
            <span className="text-[10px] text-muted-foreground font-semibold mt-2 min-w-[16px] text-center">
              {index + 1}
            </span>
            <div className="flex-1">
              {isSimple ? (
                <Input
                  type={itemSchema.type === "string" ? "text" : "number"}
                  value={String(item ?? "")}
                  placeholder={itemSchema["ui:placeholder"]}
                  className={cn("text-xs h-7", itemSchema.type === "string" && "font-mono")}
                  onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
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
                />
              ) : itemSchema.properties ? (
                <div className="pl-2 border-l border-border space-y-2">
                  {Object.entries(itemSchema.properties)
                    .filter(([, s]) => !s.deprecated)
                    .map(([key, fieldSchema]) => (
                      <ParameterField
                        key={key}
                        name={key}
                        schema={fieldSchema}
                        value={(item as Record<string, unknown>)?.[key]}
                        onChange={(v) =>
                          handleItemChange(index, {
                            ...(item as Record<string, unknown>),
                            [key]: v,
                          })
                        }
                        allValues={item as Record<string, unknown>}
                        depth={depth + 1}
                      />
                    ))}
                </div>
              ) : (
                <JsonField
                  name={`${name}-${index}`}
                  value={item as Record<string, unknown>}
                  onChange={(v) => handleItemChange(index, v)}
                />
              )}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 w-6 p-0 mt-0.5"
              onClick={() => handleRemove(index)}
            >
              ✕
            </Button>
          </div>
        ))}

        <Button type="button" variant="outline" size="sm" onClick={handleAdd} className="w-full">
          + Add item
        </Button>
      </CollapsibleContent>
    </Collapsible>
  );
}
