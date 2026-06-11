import { Input } from "../../ui/input";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

/** Plain single-line text input — the default renderer for string fields. */
export function TextField({
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
  onChange,
}: FieldChromeProps & {
  value: string;
  placeholder?: string;
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
        value={value}
        placeholder={placeholder}
        disabled={disabled}
        className="text-xs h-8"
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value || undefined)}
      />
    </FieldWrapper>
  );
}
