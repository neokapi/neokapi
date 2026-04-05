/**
 * LocaleSelect — single and multi-locale selectors built on the Combobox primitive.
 *
 * Pure components with no API dependency — locales are passed as props.
 * Uses a custom filter so typing matches against both display name and code.
 */

import { useMemo, useCallback } from "react";
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

interface LocaleOption {
  value: string;
  /** Display label: "French (fr)". */
  label: string;
  /** Lowercase search text. */
  searchText: string;
}

function buildOption(l: LocaleInfo): LocaleOption {
  return {
    value: l.code,
    label: `${l.displayName} (${l.code})`,
    searchText: `${l.displayName} ${l.code}`.toLowerCase(),
  };
}

function localeFilter(option: LocaleOption, query: string): boolean {
  if (!query) return true;
  return option.searchText.includes(query.toLowerCase());
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
  const optionMap = useMemo(() => {
    const map = new Map<string, LocaleOption>();
    for (const l of locales) map.set(l.code, buildOption(l));
    return map;
  }, [locales]);

  const selectedOption = value ? (optionMap.get(value) ?? null) : null;

  const handleChange = useCallback(
    (v: LocaleOption | null) => {
      if (v) onChange(v.value);
    },
    [onChange],
  );

  return (
    <div className={cn("w-full", className)}>
      <Combobox
        value={selectedOption}
        onValueChange={handleChange}
        disabled={disabled}
        filter={localeFilter}
      >
        <ComboboxInput placeholder={placeholder} />
        <ComboboxContent>
          <ComboboxList>
            {locales.map((l) => (
              <ComboboxItem key={l.code} value={optionMap.get(l.code)}>
                {l.displayName} ({l.code})
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
  const optionMap = useMemo(() => {
    const map = new Map<string, LocaleOption>();
    for (const l of locales) map.set(l.code, buildOption(l));
    return map;
  }, [locales]);

  const displayMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const l of locales) map.set(l.code, l.displayName);
    return map;
  }, [locales]);

  const selectedOptions = useMemo(
    () => value.map((code) => optionMap.get(code)).filter(Boolean) as LocaleOption[],
    [value, optionMap],
  );

  const available = useMemo(() => {
    const selected = new Set(value);
    return locales.filter((l) => !selected.has(l.code)).map((l) => optionMap.get(l.code)!);
  }, [locales, value, optionMap]);

  const handleChange = useCallback(
    (v: LocaleOption[] | null) => {
      onChange(v ? v.map((o) => o.value) : []);
    },
    [onChange],
  );

  return (
    <div className={cn("w-full", className)}>
      <Combobox
        value={selectedOptions}
        onValueChange={handleChange}
        multiple
        disabled={disabled}
        filter={localeFilter}
      >
        <ComboboxChips>
          {value.map((code) => (
            <ComboboxChip key={code} value={optionMap.get(code)}>
              {displayMap.get(code) ?? code} ({code})
            </ComboboxChip>
          ))}
          <ComboboxChipsInput placeholder={value.length === 0 ? placeholder : ""} />
        </ComboboxChips>
        <ComboboxContent>
          <ComboboxList>
            {available.map((opt) => (
              <ComboboxItem key={opt.value} value={opt}>
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
