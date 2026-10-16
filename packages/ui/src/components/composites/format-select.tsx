/**
 * FormatSelect: searchable format selector using Popover + Command (cmdk).
 *
 * The cmdk Command component handles filtering automatically by matching
 * typed text against item content. Groups by source (built-in vs plugin). A
 * format's engine is shown beside its name when it is a plugin's or when
 * another engine also claims one of its extensions, and such an extension is
 * marked with "*": for a file with it, the project's engine order decides.
 */

import { useState, useMemo } from "react";
import { ChevronsUpDown, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
} from "../ui/command";
import { Button } from "../ui/button";
import { cn } from "../../lib/utils";

/** Format metadata for display in the selector. */
export interface FormatInfo {
  name: string;
  display_name?: string;
  extensions?: string[];
  source?: string;
  /** The engine the format belongs to: "native" or a plugin's name. */
  engine?: string;
  /** Extensions a format of another engine also claims. */
  shared_extensions?: string[];
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

/** The engine a format belongs to, as the listing names it. */
export function formatEngine(f: FormatInfo): string {
  if (f.engine) return f.engine;
  if (!f.source || f.source === "built-in") return "native";
  return f.source;
}

/** Whether the engine badge is shown: a plugin's format, or a contested extension. */
function showsEngine(f: FormatInfo): boolean {
  return formatEngine(f) !== "native" || (f.shared_extensions?.length ?? 0) > 0;
}

/** The format's extensions, with those another engine also claims marked. */
function extensionList(f: FormatInfo): string {
  const shared = new Set((f.shared_extensions ?? []).map((e) => e.toLowerCase()));
  return (f.extensions ?? []).map((e) => (shared.has(e.toLowerCase()) ? `${e}*` : e)).join(" ");
}

function EngineBadge({ format }: { format: FormatInfo }) {
  return (
    <span
      className="shrink-0 rounded bg-muted px-1 py-px text-[10px] leading-none text-muted-foreground"
      title={`Engine: ${formatEngine(format)}`}
    >
      {formatEngine(format)}
    </span>
  );
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
          <span className="flex min-w-0 items-center gap-1.5">
            <span className="truncate">{triggerLabel}</span>
            {selected && showsEngine(selected) && <EngineBadge format={selected} />}
          </span>
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
                      <span className="flex min-w-0 flex-1 items-center gap-1.5">
                        <span className="min-w-0 break-words">{f.display_name || f.name}</span>
                        {showsEngine(f) && <EngineBadge format={f} />}
                      </span>
                      {f.extensions && f.extensions.length > 0 && (
                        <span className="shrink-0 max-w-[55%] text-right text-[10px] leading-relaxed text-muted-foreground">
                          {extensionList(f)}
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
                      <span className="flex min-w-0 flex-1 items-center gap-1.5">
                        <span className="min-w-0 break-words">{f.display_name || f.name}</span>
                        <EngineBadge format={f} />
                      </span>
                      {f.extensions && f.extensions.length > 0 && (
                        <span className="shrink-0 max-w-[55%] text-right text-[10px] leading-relaxed text-muted-foreground">
                          {extensionList(f)}
                        </span>
                      )}
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
