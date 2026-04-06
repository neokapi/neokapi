import { ChevronRight } from "lucide-react";
import { cn } from "../../../lib/utils";
import { Label } from "../../ui/label";
import type { PropertySchema } from "../types";
import { PropertyField } from "../PropertyField";

export function NestedObjectEditor({
  label,
  description,
  schema,
  value,
  onChange,
  compact,
  depth,
  name,
  onDrillDown,
  disabled,
}: {
  label: string;
  description?: string;
  schema: PropertySchema;
  value: Record<string, unknown> | undefined;
  onChange: (value: unknown) => void;
  compact?: boolean;
  depth: number;
  name: string;
  onDrillDown?: (
    label: string,
    key: string,
    schema: PropertySchema,
    values: Record<string, unknown>,
  ) => void;
  disabled?: boolean;
}) {
  const props = schema.properties || {};
  const keys = Object.keys(props).filter((k) => !props[k].deprecated);
  const current = value ?? {};

  // At depth >= 2, render as drill-down button
  if (depth >= 2 && onDrillDown) {
    return (
      <button
        type="button"
        disabled={disabled}
        className={cn(
          "flex items-center justify-between w-full px-3 py-2 text-left text-xs",
          "rounded-md border hover:bg-accent/50 transition-colors",
          disabled && "opacity-50 cursor-not-allowed",
        )}
        onClick={() => onDrillDown(label, name, schema, current)}
      >
        <div>
          <span className="font-medium">{label}</span>
          <span className="ml-2 text-muted-foreground">{keys.length} fields</span>
        </div>
        <ChevronRight className="size-4 text-muted-foreground" />
      </button>
    );
  }

  // Render fields inline
  return (
    <div className={cn("space-y-1", disabled && "opacity-50")}>
      {label && depth > 0 && (
        <Label className="text-xs font-medium text-muted-foreground">{label}</Label>
      )}
      {!compact && description && <p className="text-xs text-muted-foreground">{description}</p>}
      <div className={cn("space-y-2", depth > 0 && "ml-3 border-l pl-3")}>
        {keys.map((key) => (
          <PropertyField
            key={key}
            name={key}
            schema={props[key]}
            value={current[key]}
            onChange={(v) => onChange({ ...current, [key]: v })}
            compact={compact}
            allValues={current}
            allProperties={props}
            depth={depth + 1}
            onDrillDown={onDrillDown}
          />
        ))}
      </div>
    </div>
  );
}
