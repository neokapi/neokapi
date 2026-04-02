import { useState, useCallback } from "react";
import { X, ChevronUp, ChevronDown, Plus } from "lucide-react";
import { cn } from "../../../lib/utils";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";
import type { PropertySchema } from "../types";
import { PropertyField } from "../PropertyField";
import { JsonInlineEditor } from "./JsonEditor";

export function ArrayEditor({
  label,
  description,
  itemSchema,
  value,
  onChange,
  compact,
  depth = 0,
}: {
  label: string;
  description?: string;
  itemSchema: PropertySchema;
  value: unknown[] | undefined;
  onChange: (value: unknown) => void;
  compact?: boolean;
  depth?: number;
}) {
  const items = value ?? [];
  const isSimple =
    itemSchema.type === "string" ||
    itemSchema.type === "number" ||
    itemSchema.type === "integer";

  const [newItem, setNewItem] = useState("");

  const handleAdd = useCallback(
    (val: unknown) => {
      onChange([...items, val]);
    },
    [items, onChange],
  );

  const handleRemove = useCallback(
    (index: number) => {
      const next = [...items];
      next.splice(index, 1);
      onChange(next);
    },
    [items, onChange],
  );

  const handleChange = useCallback(
    (index: number, val: unknown) => {
      const next = [...items];
      next[index] = val;
      onChange(next);
    },
    [items, onChange],
  );

  const handleMove = useCallback(
    (index: number, dir: -1 | 1) => {
      const next = [...items];
      const target = index + dir;
      if (target < 0 || target >= next.length) return;
      [next[index], next[target]] = [next[target], next[index]];
      onChange(next);
    },
    [items, onChange],
  );

  if (isSimple) {
    return (
      <div className="space-y-1.5">
        {label && <Label className="text-xs font-medium">{label}</Label>}
        {!compact && description && (
          <p className="text-xs text-muted-foreground">{description}</p>
        )}
        <div className="flex flex-wrap gap-1.5">
          {items.map((item, i) => (
            <span
              key={i}
              className="inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded-full border border-input bg-muted/50"
            >
              {String(item)}
              <button
                type="button"
                className="text-muted-foreground hover:text-destructive"
                onClick={() => handleRemove(i)}
              >
                <X className="size-3" />
              </button>
            </span>
          ))}
          <Input
            value={newItem}
            placeholder="Add..."
            className="h-6 w-24 text-xs"
            onKeyDown={(e) => {
              if (e.key === "Enter" && newItem.trim()) {
                handleAdd(
                  itemSchema.type === "number"
                    ? parseFloat(newItem)
                    : itemSchema.type === "integer"
                      ? parseInt(newItem)
                      : newItem,
                );
                setNewItem("");
              }
            }}
            onChange={(e) => setNewItem(e.target.value)}
          />
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {label && <Label className="text-xs font-medium">{label}</Label>}
      {!compact && description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      <div className="space-y-2">
        {items.map((item, i) => (
          <div key={i} className="flex items-start gap-2 border border-input rounded-md p-2">
            <span className="text-xs text-muted-foreground w-4 text-right tabular-nums shrink-0 pt-1">
              {i + 1}
            </span>
            <div className="flex-1">
              {itemSchema.properties ? (
                <PropertyField
                  name={`${i}`}
                  schema={itemSchema}
                  value={item}
                  onChange={(v) => handleChange(i, v)}
                  compact={compact}
                  depth={depth + 1}
                />
              ) : (
                <JsonInlineEditor
                  value={item}
                  onChange={(v) => handleChange(i, v)}
                />
              )}
            </div>
            <div className="flex flex-col gap-0.5 shrink-0">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="size-6 p-0"
                disabled={i === 0}
                onClick={() => handleMove(i, -1)}
              >
                <ChevronUp className="size-3" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="size-6 p-0"
                disabled={i === items.length - 1}
                onClick={() => handleMove(i, 1)}
              >
                <ChevronDown className="size-3" />
              </Button>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="size-6 p-0 text-muted-foreground hover:text-destructive"
                onClick={() => handleRemove(i)}
              >
                <X className="size-3" />
              </Button>
            </div>
          </div>
        ))}
      </div>
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="h-7 text-xs gap-1"
        onClick={() => handleAdd(itemSchema.properties ? {} : "")}
      >
        <Plus className="size-3" />
        Add item
      </Button>
    </div>
  );
}
