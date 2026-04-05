/**
 * FormatSelect — searchable format selector with grouping and rich display.
 *
 * Groups formats by source (built-in vs plugin), shows display names,
 * file extensions, and plugin source labels. Supports clear-to-auto-detect.
 *
 * Uses object values ({ value, label }) so base-ui's built-in filter
 * matches against the label (display name + ID + extensions).
 */

import { useMemo, useCallback } from "react";
import {
  Combobox,
  ComboboxInput,
  ComboboxContent,
  ComboboxList,
  ComboboxItem,
  ComboboxEmpty,
  ComboboxGroup,
  ComboboxLabel,
  ComboboxSeparator,
} from "./combobox";
import { cn } from "../../lib/utils";

/** Format metadata for display in the selector. */
export interface FormatInfo {
  name: string;
  display_name?: string;
  extensions?: string[];
  source?: string;
}

export interface FormatSelectProps {
  value: string;
  onChange: (value: string | undefined) => void;
  formats: FormatInfo[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

interface FormatOption {
  value: string;
  label: string;
}

/** Searchable format selector with built-in/plugin grouping and file extensions. */
export function FormatSelect({
  value,
  onChange,
  formats,
  placeholder = "auto-detect",
  className,
  disabled,
}: FormatSelectProps) {
  // Build option objects with searchable labels.
  const optionMap = useMemo(() => {
    const map = new Map<string, FormatOption>();
    for (const f of formats) {
      const parts = [f.display_name || f.name, f.name];
      if (f.extensions?.length) parts.push(f.extensions.join(" "));
      if (f.source && f.source !== "built-in") parts.push(f.source);
      map.set(f.name, { value: f.name, label: parts.join(" ") });
    }
    return map;
  }, [formats]);

  const selectedOption = value ? (optionMap.get(value) ?? null) : null;

  const builtIn = formats.filter((f) => !f.source || f.source === "built-in");
  const plugin = formats.filter((f) => f.source && f.source !== "built-in");

  const handleChange = useCallback(
    (v: FormatOption | null) => onChange(v?.value || undefined),
    [onChange],
  );

  return (
    <div className={cn("w-full", className)}>
      <Combobox value={selectedOption} onValueChange={handleChange} disabled={disabled}>
        <ComboboxInput placeholder={placeholder} showClear />
        <ComboboxContent>
          <ComboboxList>
            {builtIn.map((f) => (
              <ComboboxItem key={f.name} value={optionMap.get(f.name)}>
                <div className="flex w-full items-center justify-between gap-2">
                  <span>{f.display_name || f.name}</span>
                  {f.extensions && f.extensions.length > 0 && (
                    <span className="text-[10px] text-muted-foreground">
                      {f.extensions.join(" ")}
                    </span>
                  )}
                </div>
              </ComboboxItem>
            ))}
            {plugin.length > 0 && builtIn.length > 0 && <ComboboxSeparator />}
            {plugin.length > 0 && (
              <ComboboxGroup>
                <ComboboxLabel>Plugins</ComboboxLabel>
                {plugin.map((f) => (
                  <ComboboxItem key={f.name} value={optionMap.get(f.name)}>
                    <div className="flex w-full items-center justify-between gap-2">
                      <span>{f.display_name || f.name}</span>
                      <span className="text-[10px] text-muted-foreground">
                        {f.source}
                        {f.extensions?.length ? ` · ${f.extensions.join(" ")}` : ""}
                      </span>
                    </div>
                  </ComboboxItem>
                ))}
              </ComboboxGroup>
            )}
            <ComboboxEmpty>No matching formats</ComboboxEmpty>
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
  );
}
