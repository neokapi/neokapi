/**
 * LocaleSelect — single and multi-locale selectors.
 *
 * The single selector keeps its rich pill trigger on Popover + Command (cmdk),
 * which filters by matching typed text against item content (display name + code).
 * The multi selector renders removable pill chips plus the shared Combobox
 * primitive as a searchable "add locale" picker — no hand-rolled dropdown.
 */

import { useMemo, useState } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import { ChevronsUpDown, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "../ui/popover";
import {
  Command,
  CommandInput,
  CommandList,
  CommandEmpty,
  CommandGroup,
  CommandItem,
} from "../ui/command";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "../ui/combobox";
import { Button } from "../ui/button";
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

/** Multi-locale selector: removable pill chips plus a searchable add-picker. */
export function MultiLocaleSelect({
  value,
  onChange,
  locales,
  placeholder = t("Add locale..."),
  className,
  disabled,
}: MultiLocaleSelectProps) {
  const [search, setSearch] = useState("");

  const displayMap = useMemo(() => new Map(locales.map((l) => [l.code, l.displayName])), [locales]);

  // Locales still available to add (not already selected).
  const selectable = useMemo(() => {
    const selected = new Set(value);
    return locales.filter((l) => !selected.has(l.code));
  }, [locales, value]);

  // base-ui does not filter loose children on its own; match on the typed text.
  const available = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return selectable;
    return selectable.filter(
      (l) => l.displayName.toLowerCase().includes(q) || l.code.toLowerCase().includes(q),
    );
  }, [selectable, search]);

  const addLocale = (code: string) => {
    if (code && !value.includes(code)) onChange([...value, code]);
    setSearch("");
  };

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      {value.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {value.map((code) => (
            <span
              key={code}
              className="inline-flex items-center gap-1 rounded bg-muted px-1 py-0.5 text-[10px] font-medium"
            >
              <LocalePill locale={code} />
              <span>{displayMap.get(code) ?? code}</span>
              {!disabled && (
                <span
                  role="button"
                  tabIndex={0}
                  className="rounded-sm hover:bg-accent"
                  onClick={() => onChange(value.filter((v) => v !== code))}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      onChange(value.filter((v) => v !== code));
                    }
                  }}
                >
                  <X className="size-2.5 opacity-50 hover:opacity-100" />
                </span>
              )}
            </span>
          ))}
        </div>
      )}
      {selectable.length === 0 ? (
        <p className="px-1 py-1.5 text-xs text-muted-foreground">
          {value.length > 0 ? t("All locales selected") : t("No locales available")}
        </p>
      ) : (
        <Combobox
          key={value.length}
          value={null}
          disabled={disabled}
          onValueChange={(v: string | null) => {
            if (v != null) addLocale(v);
          }}
          onInputValueChange={(text: string) => setSearch(text)}
        >
          <ComboboxInput placeholder={placeholder} disabled={disabled} className="w-full" />
          <ComboboxContent>
            <ComboboxList>
              <ComboboxEmpty>No matching locales.</ComboboxEmpty>
              {available.map((l) => (
                <ComboboxItem key={l.code} value={l.code}>
                  <span className="flex items-center gap-2">
                    <LocalePill locale={l.code} />
                    <span className="whitespace-nowrap">{l.displayName}</span>
                  </span>
                </ComboboxItem>
              ))}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      )}
    </div>
  );
}
