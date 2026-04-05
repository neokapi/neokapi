/**
 * FormatSelect — searchable format selector with grouping and rich display.
 *
 * Groups formats by source (built-in vs plugin), shows display names,
 * file extensions, and plugin source labels. Supports clear-to-auto-detect.
 */

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

/** Searchable format selector with built-in/plugin grouping and file extensions. */
export function FormatSelect({
  value,
  onChange,
  formats,
  placeholder = "auto-detect",
  className,
  disabled,
}: FormatSelectProps) {
  const builtIn = formats.filter((f) => !f.source || f.source === "built-in");
  const plugin = formats.filter((f) => f.source && f.source !== "built-in");

  return (
    <div className={cn("w-full", className)}>
      <Combobox
        value={value}
        onValueChange={(v: string | null) => onChange(v || undefined)}
        disabled={disabled}
      >
        <ComboboxInput placeholder={placeholder} showClear />
        <ComboboxContent>
          <ComboboxList>
            {builtIn.map((f) => (
              <ComboboxItem key={f.name} value={f.name}>
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
                  <ComboboxItem key={f.name} value={f.name}>
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
