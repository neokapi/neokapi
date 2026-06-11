import { cn } from "../../../lib/utils";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

/** Scrollable button-list single select (the `select` widget). */
export function SelectListField({
  label,
  description,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
  values,
  current,
  getLabel,
  onChange,
}: FieldChromeProps & {
  values: unknown[];
  current: string;
  getLabel: (val: unknown) => string;
  onChange: (value: unknown) => void;
}) {
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
      <div
        className={cn(
          "border border-input rounded-md max-h-[120px] overflow-y-auto",
          disabled && "opacity-50",
        )}
      >
        {values.map((v) => {
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
                isActive ? "bg-primary text-primary-foreground font-medium" : "hover:bg-accent/50",
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
