import { cn } from "../../../lib/utils";
import { Input } from "../../ui/input";
import { useSchemaFormHost, type SchemaFormCredential } from "../host";
import type { PropertySchema } from "../types";

/**
 * Picker for a saved credential. Credentials can be supplied two ways:
 *
 *   - Inline `options` baked into the schema (how kapi-desktop's backend
 *     currently injects them via injectCredentialPicker).
 *   - A host-injected `credentials()` callback on the schema-form host —
 *     useful when the available credentials are known only to the host.
 *
 * Inline options take precedence. When neither source yields a list the widget
 * degrades to a plain text input so the field stays usable in hosts without a
 * credential store (the docs website, Storybook).
 */
export function CredentialPicker({
  schema,
  value,
  placeholder,
  disabled,
  onChange,
}: {
  schema: PropertySchema;
  value: string;
  placeholder?: string;
  disabled?: boolean;
  onChange: (value: string | undefined) => void;
}) {
  const host = useSchemaFormHost();

  // Inline schema options win; otherwise ask the host. resourceKind (when the
  // schema carries an x-path annotation) lets the host scope the list.
  const inlineOptions: SchemaFormCredential[] | undefined = schema.options?.map((o) => ({
    value: String(o.value),
    label: o.label,
  }));
  const credentials =
    inlineOptions ?? host.credentials?.(schema["x-path"]?.resourceKind) ?? undefined;

  if (!credentials || credentials.length === 0) {
    return (
      <Input
        value={value}
        placeholder={placeholder || "Credential name..."}
        disabled={disabled}
        className="text-xs h-8"
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value || undefined)}
      />
    );
  }

  return (
    <select
      value={value}
      disabled={disabled}
      onChange={(e) => onChange(e.target.value === "" ? undefined : e.target.value)}
      className={cn(
        "h-8 w-full rounded-lg border border-input bg-transparent px-2 py-1 text-base md:text-sm",
        "transition-colors outline-none",
        "focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50",
        "dark:bg-input/30",
        disabled && "opacity-50 pointer-events-none",
      )}
    >
      {credentials.map((c) => (
        <option key={c.value} value={c.value}>
          {c.label}
        </option>
      ))}
    </select>
  );
}
