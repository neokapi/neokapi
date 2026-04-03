import { useState, useCallback } from "react";
import { X, ChevronDown, ChevronRight, Plus } from "lucide-react";
import { cn } from "../../../lib/utils";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { Label } from "../../ui/label";
import type { PropertySchema } from "../types";
import { PropertyField } from "../PropertyField";

function MapEntry({
  entryKey,
  value,
  itemSchema,
  onValueChange,
  onRemove,
  compact,
  depth,
  keyPlaceholder,
}: {
  entryKey: string;
  value: unknown;
  itemSchema?: PropertySchema;
  onValueChange: (value: unknown) => void;
  onRemove: () => void;
  compact?: boolean;
  depth: number;
  keyPlaceholder?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const isComplex =
    itemSchema?.type === "object" || itemSchema?.type === "array" || typeof value === "object";

  if (!isComplex) {
    return (
      <div className="flex items-center gap-0 rounded-md border border-input overflow-hidden">
        <span className="shrink-0 px-2.5 py-1.5 text-xs font-mono text-muted-foreground bg-muted/40 border-r border-input">
          {entryKey}
        </span>
        <input
          value={String(value ?? "")}
          className="flex-1 bg-transparent px-2.5 py-1.5 text-xs outline-none"
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onValueChange(e.target.value || undefined)}
        />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="size-7 p-0 shrink-0 text-muted-foreground hover:text-destructive rounded-none"
          onClick={onRemove}
        >
          <X className="size-3" />
        </Button>
      </div>
    );
  }

  return (
    <div className="border border-input rounded-md">
      <button
        type="button"
        className="flex items-center gap-2 w-full px-2 py-1.5 text-left text-xs"
        onClick={() => setExpanded(!expanded)}
      >
        {expanded ? (
          <ChevronDown className="size-3 text-muted-foreground" />
        ) : (
          <ChevronRight className="size-3 text-muted-foreground" />
        )}
        <span className="font-mono font-medium">{entryKey}</span>
        <span className="flex-1" />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="size-5 p-0 text-muted-foreground hover:text-destructive"
          onClick={(e: React.MouseEvent) => {
            e.stopPropagation();
            onRemove();
          }}
        >
          <X className="size-3" />
        </Button>
      </button>
      {expanded && itemSchema && (
        <div className="px-3 pb-2">
          <PropertyField
            name={entryKey}
            schema={itemSchema}
            value={value}
            onChange={onValueChange}
            compact={compact}
            depth={depth + 1}
          />
        </div>
      )}
    </div>
  );
}

export function MapEditor({
  label,
  description,
  value,
  itemSchema,
  onChange,
  compact,
  depth = 0,
  keyPlaceholder,
}: {
  label: string;
  description?: string;
  value: Record<string, unknown> | undefined;
  itemSchema?: PropertySchema;
  onChange: (value: unknown) => void;
  compact?: boolean;
  depth?: number;
  keyPlaceholder?: string;
}) {
  const entries = value ?? {};
  const [newKey, setNewKey] = useState("");

  const handleAdd = useCallback(() => {
    if (!newKey.trim()) return;
    onChange({ ...entries, [newKey.trim()]: "" });
    setNewKey("");
  }, [entries, newKey, onChange]);

  const handleRemove = useCallback(
    (key: string) => {
      const next = { ...entries };
      delete next[key];
      onChange(next);
    },
    [entries, onChange],
  );

  const handleValueChange = useCallback(
    (key: string, val: unknown) => {
      onChange({ ...entries, [key]: val });
    },
    [entries, onChange],
  );

  return (
    <div className="space-y-2">
      {label && <Label className="text-xs font-medium">{label}</Label>}
      {!compact && description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      <div className="space-y-1.5">
        {Object.entries(entries).map(([key, val]) => (
          <MapEntry
            key={key}
            entryKey={key}
            value={val}
            itemSchema={itemSchema}
            onValueChange={(v) => handleValueChange(key, v)}
            onRemove={() => handleRemove(key)}
            compact={compact}
            depth={depth}
            keyPlaceholder={keyPlaceholder}
          />
        ))}
      </div>
      <div className="flex items-center gap-2">
        <Input
          value={newKey}
          placeholder={keyPlaceholder || "New key..."}
          className="h-7 text-xs flex-1"
          onKeyDown={(e: React.KeyboardEvent<HTMLInputElement>) => e.key === "Enter" && handleAdd()}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => setNewKey(e.target.value)}
        />
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="h-7 text-xs gap-1"
          onClick={handleAdd}
        >
          <Plus className="size-3" />
          Add
        </Button>
      </div>
    </div>
  );
}
