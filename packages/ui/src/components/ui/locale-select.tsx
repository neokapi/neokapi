/**
 * LocaleSelect — single and multi-locale selectors using Popover + Command (cmdk).
 *
 * The cmdk Command component handles filtering automatically by matching
 * typed text against item content (display name and code).
 */

import { useState } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { ChevronsUpDown, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
} from "./command";
import { Button } from "./button";
import { cn } from "../../lib/utils";
import { LocalePill } from "../resource-browser/LocalePill";

/** Locale info for display in selectors. */
export interface LocaleInfo {
  code: string;
  displayName: string;
}

/** Resolve a locale code to a display name via the browser's Intl API. */
let intlNames: Intl.DisplayNames | null = null;
export function resolveLocaleName(code: string): string {
  try {
    if (!intlNames) intlNames = new Intl.DisplayNames("en", { type: "language" });
    return intlNames.of(code) ?? code;
  } catch {
    return code;
  }
}

// --- Single locale selector ---

export interface LocaleSelectProps {
  value: string;
  onChange: (value: string) => void;
  locales: LocaleInfo[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
  /** Compact mode — trigger shows only the colored LocalePill, no display name. */
  compact?: boolean;
}

/** Single-locale selector with search. */
export function LocaleSelect({
  value,
  onChange,
  locales,
  placeholder = t("Select locale..."),
  className,
  disabled,
  compact = false,
}: LocaleSelectProps) {
  const [open, setOpen] = useState(false);

  const selected = locales.find((l) => l.code === value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            "h-8 justify-between text-xs font-normal",
            compact ? "w-auto" : "w-full",
            !selected && "text-muted-foreground",
            className,
          )}
        >
          {selected ? (
            compact ? (
              <LocalePill locale={selected.code} />
            ) : (
              <span className="flex items-center gap-1.5 truncate">
                <LocalePill locale={selected.code} />
                <span>{selected.displayName}</span>
              </span>
            )
          ) : (
            <span className="truncate">{placeholder}</span>
          )}
          <ChevronsUpDown className="size-3 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="min-w-[240px] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search locales..." />
          <CommandList>
            <CommandEmpty>No matching locales.</CommandEmpty>
            <CommandGroup>
              {locales.map((l) => (
                <CommandItem
                  key={l.code}
                  value={`${l.displayName} ${l.code}`}
                  onSelect={() => {
                    onChange(l.code);
                    setOpen(false);
                  }}
                  data-checked={l.code === value}
                >
                  <span className="flex items-center gap-2">
                    <LocalePill locale={l.code} />
                    <span className="whitespace-nowrap">{l.displayName}</span>
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

// --- Multi-locale selector ---

export interface MultiLocaleSelectProps {
  value: string[];
  onChange: (value: string[]) => void;
  locales: LocaleInfo[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

/** Multi-locale selector with chip display and searchable dropdown. */
export function MultiLocaleSelect({
  value,
  onChange,
  locales,
  placeholder = t("Add locale..."),
  className,
  disabled,
}: MultiLocaleSelectProps) {
  const [open, setOpen] = useState(false);
  const selectedSet = new Set(value);

  const displayMap = new Map(locales.map((l) => [l.code, l.displayName]));

  const toggle = (code: string) => {
    if (selectedSet.has(code)) {
      onChange(value.filter((v) => v !== code));
    } else {
      onChange([...value, code]);
    }
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn(
            "h-auto min-h-8 w-full justify-between text-xs font-normal",
            value.length === 0 && "text-muted-foreground",
            className,
          )}
        >
          {value.length === 0 ? (
            <span>{placeholder}</span>
          ) : (
            <div className="flex flex-wrap gap-1">
              {value.map((code) => (
                <span
                  key={code}
                  className="inline-flex items-center gap-1 rounded bg-muted px-1 py-0.5 text-[10px] font-medium"
                >
                  <LocalePill locale={code} />
                  <span>{displayMap.get(code) ?? code}</span>
                  <span
                    role="button"
                    className="rounded-sm hover:bg-accent"
                    onClick={(e) => {
                      e.stopPropagation();
                      onChange(value.filter((v) => v !== code));
                    }}
                  >
                    <X className="size-2.5 opacity-50 hover:opacity-100" />
                  </span>
                </span>
              ))}
            </div>
          )}
          <ChevronsUpDown className="size-3 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="min-w-[240px] p-0" align="start">
        <Command>
          <CommandInput placeholder="Search locales..." />
          <CommandList>
            <CommandEmpty>No matching locales.</CommandEmpty>
            <CommandGroup>
              {locales.map((l) => (
                <CommandItem
                  key={l.code}
                  value={`${l.displayName} ${l.code}`}
                  onSelect={() => toggle(l.code)}
                  data-checked={selectedSet.has(l.code)}
                >
                  <span className="flex items-center gap-2">
                    <LocalePill locale={l.code} />
                    <span className="whitespace-nowrap">{l.displayName}</span>
                  </span>
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}
