/**
 * FormatSelect — searchable format selector using Popover + Command (cmdk).
 *
 * The cmdk Command component handles filtering automatically by matching
 * typed text against item content. Groups by source (built-in vs plugin).
 */

import { useState, useMemo } from "react";
import { ChevronsUpDown, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
} from "./command";
import { Button } from "./button";
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

function formatDisplayLabel(f: FormatInfo): string {
  const displayName = f.display_name || f.name;
  const isPlugin = f.source && f.source !== "built-in";
  if (displayName !== f.name || isPlugin) return `${displayName} (${f.name})`;
  return displayName;
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
  const [open, setOpen] = useState(false);

  const builtIn = useMemo(
    () => formats.filter((f) => !f.source || f.source === "built-in"),
    [formats],
  );
  const plugin = useMemo(
    () => formats.filter((f) => f.source && f.source !== "built-in"),
    [formats],
  );

  const selected = formats.find((f) => f.name === value);
  const triggerLabel = selected ? formatDisplayLabel(selected) : placeholder;

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            "h-8 w-full justify-between text-xs font-normal",
            !selected && "text-muted-foreground",
            className,
          )}
        >
          <span className="truncate">{triggerLabel}</span>
          <div className="flex shrink-0 items-center gap-0.5">
            {selected && (
              <span
                role="button"
                className="rounded-sm p-0.5 hover:bg-accent"
                onClick={(e) => {
                  e.stopPropagation();
                  onChange(undefined);
                }}
              >
                <X className="size-3 opacity-50 hover:opacity-100" />
              </span>
            )}
            <ChevronsUpDown className="size-3 opacity-50" />
          </div>
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-(--radix-popover-trigger-width) p-0" align="start">
        <Command>
          <CommandInput placeholder="Search formats..." />
          <CommandList>
            <CommandEmpty>No matching formats.</CommandEmpty>
            {builtIn.length > 0 && (
              <CommandGroup>
                {builtIn.map((f) => (
                  <CommandItem
                    key={f.name}
                    value={`${f.display_name || f.name} ${f.name} ${f.extensions?.join(" ") ?? ""}`}
                    onSelect={() => {
                      onChange(f.name === value ? undefined : f.name);
                      setOpen(false);
                    }}
                    data-checked={f.name === value}
                  >
                    <div className="flex w-full items-start gap-3">
                      <span className="min-w-0 flex-1 break-words">
                        {f.display_name || f.name}
                      </span>
                      {f.extensions && f.extensions.length > 0 && (
                        <span className="shrink-0 max-w-[55%] text-right text-[10px] leading-relaxed text-muted-foreground">
                          {f.extensions.join(" ")}
                        </span>
                      )}
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
            {plugin.length > 0 && builtIn.length > 0 && <CommandSeparator />}
            {plugin.length > 0 && (
              <CommandGroup heading="Plugins">
                {plugin.map((f) => (
                  <CommandItem
                    key={f.name}
                    value={`${f.display_name || f.name} ${f.name} ${f.source} ${f.extensions?.join(" ") ?? ""}`}
                    onSelect={() => {
                      onChange(f.name === value ? undefined : f.name);
                      setOpen(false);
                    }}
                    data-checked={f.name === value}
                  >
                    <div className="flex w-full items-start gap-3">
                      <span className="min-w-0 flex-1 break-words">
                        {f.display_name || f.name}
                      </span>
                      <span className="shrink-0 max-w-[55%] text-right text-[10px] leading-relaxed text-muted-foreground">
                        {f.source}
                        {f.extensions?.length ? ` · ${f.extensions.join(" ")}` : ""}
                      </span>
                    </div>
                  </CommandItem>
                ))}
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
