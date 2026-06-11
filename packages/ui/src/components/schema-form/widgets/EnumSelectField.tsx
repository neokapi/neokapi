import { cn } from "../../../lib/utils";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

/** Native dropdown select for enum-typed fields (type-based dispatch). */
export function EnumSelectField({
  label,
  description,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
  values,
  value,
  getLabel,
  enumDescriptions,
  onChange,
}: FieldChromeProps & {
  values: unknown[];
  value: unknown;
  getLabel: (val: unknown) => string;
  enumDescriptions?: Record<string, string>;
  onChange: (value: unknown) => void;
}) {
  const selectedDescription = value != null ? enumDescriptions?.[String(value)] : undefined;
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
      <select
        value={String(value ?? "")}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value)}
        className={cn(
          "h-8 w-full rounded-lg border border-input bg-transparent px-2 py-1 text-base md:text-sm",
          "transition-colors outline-none",
          "focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
          "dark:bg-input/30",
          disabled && "opacity-50 pointer-events-none",
        )}
      >
        <option value="">—</option>
        {values.map((v) => {
          const val = String(v);
          return (
            <option key={val} value={val}>
              {getLabel(val)}
            </option>
          );
        })}
      </select>
      {selectedDescription ? (
        <p className="text-xs text-muted-foreground italic mt-0.5">{selectedDescription}</p>
      ) : null}
    </FieldWrapper>
  );
}
