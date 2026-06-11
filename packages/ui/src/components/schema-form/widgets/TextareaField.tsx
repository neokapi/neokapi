import { cn } from "../../../lib/utils";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

/** Multi-line text editor (the `textarea` widget). */
export function TextareaField({
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
  height,
  onChange,
}: FieldChromeProps & {
  value: string;
  placeholder?: string;
  height?: number;
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
      <textarea
        value={value}
        placeholder={placeholder || ""}
        disabled={disabled}
        onChange={(e) => onChange(e.target.value || undefined)}
        rows={height || 4}
        className={cn(
          "w-full rounded-md border border-input bg-transparent px-3 py-2 font-mono text-xs",
          "focus-visible:border-ring focus-visible:ring-ring/50 focus-visible:ring-[3px]",
          "resize-y",
          disabled && "opacity-50",
        )}
      />
    </FieldWrapper>
  );
}
