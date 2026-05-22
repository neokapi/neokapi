import { useState } from "react";
import { Button } from "../../ui/button";
import { Input } from "../../ui/input";
import { FormInputAction } from "../../ui/form";
import { useSchemaFormHost, type SchemaFormBrowseRequest } from "../host";
import type { PropertySchema } from "../types";

/** Normalised path metadata gathered from x-path and ui:widget-options. */
interface PathMeta {
  title?: string;
  forSaveAs?: boolean;
  filters?: Array<{ name: string; extensions: string }>;
  accepts?: string[];
}

// Path metadata reaches the form from two sources:
//   - native tools annotate via `x-path` (PathAnnotation: accepts/browseTitle/forSaveAs/filters)
//   - the Okapi bridge flattens the same keys directly onto `ui:widget-options`
// Read both, preferring x-path when present.
function resolvePathMeta(schema: PropertySchema): PathMeta {
  const xPath = schema["x-path"];
  const opts = schema["ui:widget-options"] as
    | { browseTitle?: string; forSaveAs?: boolean; filters?: PathMeta["filters"] }
    | undefined;
  return {
    title: xPath?.browseTitle ?? opts?.browseTitle,
    forSaveAs: xPath?.forSaveAs ?? opts?.forSaveAs,
    filters: xPath?.filters ?? opts?.filters,
    accepts: xPath?.accepts,
  };
}

/**
 * File / folder path widget. Renders a text input plus a Browse button that
 * delegates to a host-injected `onBrowse` handler (Wails dialogs in
 * kapi-desktop, etc.). With no handler — e.g. on the docs website where there
 * is no filesystem — the Browse button is omitted and the widget degrades to a
 * plain labelled text input.
 */
export function PathPicker({
  kind,
  name,
  schema,
  value,
  placeholder,
  disabled,
  onChange,
}: {
  kind: "file" | "directory";
  name: string;
  schema: PropertySchema;
  value: string;
  placeholder?: string;
  disabled?: boolean;
  onChange: (value: string | undefined) => void;
}) {
  const host = useSchemaFormHost();
  const meta = resolvePathMeta(schema);
  const [browsing, setBrowsing] = useState(false);

  const fallbackPlaceholder =
    placeholder || meta.title || (kind === "directory" ? "/path/to/folder..." : "/path/to/file...");

  const input = (
    <Input
      value={value}
      placeholder={fallbackPlaceholder}
      disabled={disabled}
      className="flex-1 font-mono text-xs h-8"
      onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value || undefined)}
    />
  );

  const handleBrowse = async () => {
    if (!host.onBrowse || browsing) return;
    const request: SchemaFormBrowseRequest = {
      kind,
      field: name,
      currentValue: value || undefined,
      title: meta.title,
      forSaveAs: meta.forSaveAs,
      filters: meta.filters,
      accepts: meta.accepts,
    };
    setBrowsing(true);
    try {
      const picked = await host.onBrowse(request);
      if (picked) onChange(picked);
    } finally {
      setBrowsing(false);
    }
  };

  return (
    <>
      {host.onBrowse ? (
        <FormInputAction>
          {input}
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={disabled || browsing}
            className="h-8 text-xs shrink-0"
            onClick={handleBrowse}
          >
            Browse
          </Button>
        </FormInputAction>
      ) : (
        input
      )}
      {meta.filters && meta.filters.length > 0 && (
        <p className="text-xs text-muted-foreground mt-0.5">
          {meta.filters.map((f) => f.name).join(", ")}
        </p>
      )}
    </>
  );
}
