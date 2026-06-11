import { Input } from "../../ui/input";
import { FieldWrapper } from "../primitives/FieldWrapper";
import { coerceNumericInput } from "../field-helpers";
import type { FieldChromeProps } from "../field-chrome";
import type { PropertySchema } from "../types";

/** Numeric input for integer- and number-typed fields (type-based dispatch). */
export function NumberField({
  label,
  description,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
  value,
  placeholder,
  min,
  max,
  type,
  onChange,
}: FieldChromeProps & {
  value: string;
  placeholder?: string;
  min?: number;
  max?: number;
  type: PropertySchema["type"];
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
      <Input
        type="number"
        value={value}
        placeholder={placeholder}
        min={min}
        max={max}
        step={type === "integer" ? 1 : undefined}
        disabled={disabled}
        className="text-xs h-8"
        onChange={(e: React.ChangeEvent<HTMLInputElement>) =>
          onChange(coerceNumericInput(e.target.value, type))
        }
      />
    </FieldWrapper>
  );
}
