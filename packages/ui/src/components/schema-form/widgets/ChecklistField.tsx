import { cn } from "../../../lib/utils";
import { Switch } from "../../ui/switch";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

export interface ChecklistEntry {
  name: string;
  title: string;
  description?: string;
}

/** Named-toggle list over a boolean map value (the `checklist` widget). */
export function ChecklistField({
  label,
  description,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
  entries,
  value,
  onChange,
}: FieldChromeProps & {
  entries: ChecklistEntry[];
  value: Record<string, boolean> | undefined;
  onChange: (value: unknown) => void;
}) {
  const current = value ?? {};
  return (
    <FieldWrapper
      label={label}
      description={description}
      compact={compact}
      isModified={isModified}
      docParam={docParam}
      vertical={vertical}
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
