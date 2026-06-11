import { TagInput } from "../../ui/tag-input";
import { FieldWrapper } from "../primitives/FieldWrapper";
import { splitTagList } from "../field-helpers";
import type { FieldChromeProps } from "../field-chrome";

/** Tag editor over a comma-separated string value (the `tags` widget). */
export function TagsField({
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
      <TagInput
        value={splitTagList(value)}
        onChange={(newTags) => onChange(newTags.length > 0 ? newTags.join(", ") : undefined)}
        placeholder={placeholder || "Add tag..."}
        disabled={disabled}
      />
    </FieldWrapper>
  );
}
