import { CodeInput, type CodeLanguage } from "../../ui/code-input";
import { FieldWrapper } from "../primitives/FieldWrapper";
import type { FieldChromeProps } from "../field-chrome";

/**
 * FieldWrapper-framed CodeInput, shared by the `code-editor`,
 * `simplifier-rules`, and `regex` widgets.
 */
export function CodeInputField({
  label,
  description,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
  value,
  language,
  placeholder,
  minHeight,
  singleLine,
  inputClassName,
  onChange,
}: FieldChromeProps & {
  value: string;
  language: CodeLanguage;
  placeholder?: string;
  minHeight?: number;
  singleLine?: boolean;
  inputClassName?: string;
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
      <CodeInput
        value={value}
        onChange={(v) => onChange(v || undefined)}
        language={language}
        placeholder={placeholder}
        disabled={disabled}
        minHeight={minHeight}
        singleLine={singleLine}
        className={inputClassName}
      />
    </FieldWrapper>
  );
}
