/**
 * LocaleSelect — single and multi-locale selectors built on the Combobox primitive.
 *
 * Pure components with no API dependency — locales are passed as props.
 * Used by Kapi Desktop and Bowrain for language selection with autocomplete.
 */

import { useMemo } from "react";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxChips,
  ComboboxChip,
  ComboboxChipsInput,
} from "./combobox";
import { cn } from "../../lib/utils";

/** Locale info for display in selectors. */
export interface LocaleInfo {
  code: string;
  displayName: string;
}

// --- Single locale selector ---

export interface LocaleSelectProps {
  value: string;
  onChange: (value: string) => void;
  locales: LocaleInfo[];
  placeholder?: string;
  className?: string;
  disabled?: boolean;
}

/** Single-locale selector with search. Shows "French (fr)" in options, stores "fr". */
export function LocaleSelect({
  value,
  onChange,
  locales,
  placeholder = "Select locale...",
  className,
  disabled,
}: LocaleSelectProps) {
  const options = useMemo(
    () => locales.map((l) => ({ value: l.code, label: `${l.displayName} (${l.code})` })),
    [locales],
  );

  return (
    <div className={cn("w-full", className)}>
      <Combobox
        value={value}
        onValueChange={(v: string | null) => {
          if (v != null) onChange(v);
        }}
        disabled={disabled}
      >
        <ComboboxInput placeholder={placeholder} />
        <ComboboxContent>
          <ComboboxList>
            {options.map((opt) => (
              <ComboboxItem key={opt.value} value={opt.value}>
                {opt.label}
              </ComboboxItem>
            ))}
            <ComboboxEmpty>No matching locales</ComboboxEmpty>
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
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

/** Multi-locale chip input with search. Shows removable chips for each selected locale. */
export function MultiLocaleSelect({
  value,
  onChange,
  locales,
  placeholder = "Add locale...",
  className,
  disabled,
}: MultiLocaleSelectProps) {
  const displayMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const l of locales) map.set(l.code, l.displayName);
    return map;
  }, [locales]);

  const available = useMemo(() => {
    const selected = new Set(value);
    return locales
      .filter((l) => !selected.has(l.code))
      .map((l) => ({ value: l.code, label: `${l.displayName} (${l.code})` }));
  }, [locales, value]);

  return (
    <div className={cn("w-full", className)}>
      <Combobox
        value={value}
        onValueChange={(v) => {
          if (v != null) onChange(v as unknown as string[]);
        }}
        multiple
        disabled={disabled}
      >
        <ComboboxChips>
          {value.map((code) => (
            <ComboboxChip key={code} value={code}>
              {displayMap.get(code) ?? code} ({code})
            </ComboboxChip>
          ))}
          <ComboboxChipsInput placeholder={value.length === 0 ? placeholder : ""} />
        </ComboboxChips>
        <ComboboxContent>
          <ComboboxList>
            {available.map((opt) => (
              <ComboboxItem key={opt.value} value={opt.value}>
                {opt.label}
              </ComboboxItem>
            ))}
            <ComboboxEmpty>
              {value.length === locales.length ? "All locales selected" : "No matching locales"}
            </ComboboxEmpty>
          </ComboboxList>
        </ComboboxContent>
      </Combobox>
    </div>
  );
}
