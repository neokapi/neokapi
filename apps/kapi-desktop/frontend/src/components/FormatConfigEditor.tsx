import { SchemaForm } from "@neokapi/ui-primitives";
import type { ComponentSchema } from "../types/api";
import { useSchemaFormHost } from "../hooks/useSchemaFormHost";

interface FormatConfigEditorProps {
  /** Schema for the format's configuration parameters. */
  schema: ComponentSchema;
  /** Current configuration values. */
  values: Record<string, unknown>;
  /** Called when any value changes. */
  onChange: (values: Record<string, unknown>) => void;
  /** Optional title override (defaults to schema title). */
  title?: string;
  /** When provided, fields differing from preset show a colored indicator dot. */
  presetValues?: Record<string, unknown>;
}

/**
 * Format filter configuration editor.
 *
 * Schema-driven form for configuring format readers/writers.
 * Used in both the flow editor (reader/writer node config) and
 * the standalone format presets page.
 */
export function FormatConfigEditor({
  schema,
  values,
  onChange,
  title,
  presetValues,
}: FormatConfigEditorProps) {
  const filterMeta = schema.formatMeta;
  const extensions = filterMeta?.extensions || [];
  const mimeTypes = filterMeta?.mimeTypes || [];

  // Native file/folder dialogs + credential vault for schema-form path /
  // credential widgets (degrades to text inputs outside Wails, e.g. Storybook).
  const host = useSchemaFormHost();

  return (
    <div className="flex flex-col gap-3">
      {/* Header */}
      <div className="border-b border-border pb-3">
        <h3 className="text-sm font-semibold text-foreground">{title || schema.title}</h3>
        {schema.description && (
          <p className="mt-1 text-xs text-muted-foreground">{schema.description}</p>
        )}

        {/* Format metadata badges */}
        {(extensions.length > 0 || mimeTypes.length > 0) && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {extensions.map((ext) => (
              <span
                key={ext}
                className="rounded bg-secondary px-1.5 py-0.5 text-[10px] font-medium text-secondary-foreground"
              >
                {ext}
              </span>
            ))}
            {mimeTypes.slice(0, 2).map((mt) => (
              <span
                key={mt}
                className="rounded bg-accent px-1.5 py-0.5 text-[10px] text-muted-foreground"
              >
                {mt}
              </span>
            ))}
          </div>
        )}
      </div>

      {/* Schema form */}
      <SchemaForm
        schema={schema}
        values={values}
        onChange={onChange}
        presetValues={presetValues}
        host={host}
      />
    </div>
  );
}
