import {
  FormItem,
  FormLabel,
  FormDescription,
  FormMessage,
  FormControl,
  FormHelpText,
} from "../../ui/form";
import type { ToolDocParam } from "../types";

export function FieldWrapper({
  label,
  description,
  children,
  compact,
  isModified,
  docParam,
  vertical,
  disabled,
  error,
}: {
  label: string;
  description?: string;
  children: React.ReactNode;
  compact?: boolean;
  isModified?: boolean;
  docParam?: ToolDocParam;
  vertical?: boolean;
  disabled?: boolean;
  error?: string;
}) {
  const docDescription = docParam?.description || docParam?.help;

  return (
    <FormItem disabled={disabled} modified={isModified}>
      {label && <FormLabel disabled={disabled}>{label}</FormLabel>}

      {description && (
        <FormDescription className={compact ? "text-[11px] leading-snug" : undefined}>
          {description}
        </FormDescription>
      )}

      <FormControl vertical={vertical}>{children}</FormControl>

      <FormMessage>{error}</FormMessage>

      {docDescription && !compact && (
        <FormHelpText
          description={docDescription}
          notes={docParam?.notes}
          dependencies={docParam?.dependsOn}
        />
      )}
    </FormItem>
  );
}
