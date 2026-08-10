import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
} from "@neokapi/ui-primitives";
import { useMemo, useState } from "react";
import { useLocales } from "../hooks/useLocales";

interface LocaleSelectProps {
  value: string;
  onChange: (value: string) => void;
  /** Restrict to these locale codes. If omitted, all known locales are shown. */
  codes?: string[];
  placeholder?: string;
  className?: string;
  style?: React.CSSProperties;
  "data-testid"?: string;
}

/** Single-locale selector with search. Shows "French (fr)" in the field and in options, stores "fr". */
export function LocaleSelect({
  value,
  onChange,
  codes,
  placeholder,
  className,
  style,
  ...rest
}: LocaleSelectProps) {
  const { locales, getDisplayName, loading } = useLocales();

  const options = useMemo(() => {
    if (codes) {
      return codes.map((code) => ({ value: code, label: `${getDisplayName(code)} (${code})` }));
    }
    return locales.map((l) => ({ value: l.code, label: `${l.display_name} (${l.code})` }));
  }, [codes, locales, getDisplayName]);

  // The field holds a code, but a reader should not have to decode one. Without
  // this the combobox renders its own value — "fr-FR" where the list beneath it
  // says "French (fr-FR)".
  const labelOf = useMemo(() => {
    const byCode = new Map(options.map((o) => [o.value, o.label]));
    return (code: string) => byCode.get(code) ?? `${getDisplayName(code)} (${code})`;
  }, [options, getDisplayName]);

  return (
    <div style={style} className={className} data-testid={rest["data-testid"]}>
      <Combobox
        value={value}
        itemToStringLabel={labelOf}
        onValueChange={(v: string | null) => {
          if (v != null) onChange(v);
        }}
      >
        <ComboboxInput placeholder={loading ? "Loading..." : placeholder || "Select locale..."} />
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

interface MultiLocaleSelectProps {
  value: string[];
  onChange: (value: string[]) => void;
  /** Restrict to these locale codes. If omitted, all known locales are shown. */
  codes?: string[];
  style?: React.CSSProperties;
  "data-testid"?: string;
}

/**
 * Multi-locale chip input with search. Shows removable chips for each selected
 * locale plus a shared Combobox "add" picker (searchable, keyboard-navigable) \u2014
 * no hand-rolled dropdown / click-outside / focus wiring.
 */
export function MultiLocaleSelect({
  value,
  onChange,
  codes,
  style,
  ...rest
}: MultiLocaleSelectProps) {
  const { locales, getDisplayName, loading } = useLocales();
  const testid = rest["data-testid"];
  const [search, setSearch] = useState("");

  // Options available to add: known locales, restricted to `codes`, minus already-selected.
  const selectable = useMemo(() => {
    const selected = new Set(value);
    const codeSet = codes ? new Set(codes) : null;
    return locales.filter((l) => !selected.has(l.code) && (!codeSet || codeSet.has(l.code)));
  }, [locales, value, codes]);

  // base-ui does not filter loose children on its own, so match on the typed text.
  const available = useMemo(() => {
    const q = search.trim().toLowerCase();
    if (!q) return selectable;
    return selectable.filter(
      (l) => l.display_name.toLowerCase().includes(q) || l.code.toLowerCase().includes(q),
    );
  }, [selectable, search]);

  const removeLocale = (code: string) => {
    onChange(value.filter((v) => v !== code));
  };

  const addLocale = (code: string) => {
    if (code && !value.includes(code)) onChange([...value, code]);
    setSearch("");
  };

  return (
    <div style={style} data-testid={testid} className="flex flex-col gap-2">
      {value.length > 0 && (
        <div className="flex flex-wrap gap-2" data-testid={testid ? `${testid}-chips` : undefined}>
          {value.map((code) => (
            <span
              key={code}
              className="inline-flex items-center gap-1 rounded-md bg-primary px-2 py-0.5 text-xs font-medium text-primary-foreground"
            >
              {getDisplayName(code)} ({code})
              <span
                role="button"
                tabIndex={0}
                onClick={() => removeLocale(code)}
                onKeyDown={(e) => {
                  if (e.key === "Enter" || e.key === " ") {
                    e.preventDefault();
                    removeLocale(code);
                  }
                }}
                className="cursor-pointer text-sm opacity-80 hover:opacity-100"
                data-testid={testid ? `${testid}-remove-${code}` : undefined}
              >
                {"\u00D7"}
              </span>
            </span>
          ))}
        </div>
      )}
      {selectable.length === 0 ? (
        <p
          className="px-1 py-2 text-xs text-muted-foreground"
          data-testid={testid ? `${testid}-empty` : undefined}
        >
          {value.length > 0
            ? "All locales selected"
            : loading
              ? "Loading..."
              : "No locales available"}
        </p>
      ) : (
        // Force-remount on each add so the picker resets to an empty search/value.
        <Combobox
          key={value.length}
          value={null}
          onValueChange={(v: string | null) => {
            if (v != null) addLocale(v);
          }}
          onInputValueChange={(text: string) => setSearch(text)}
        >
          <ComboboxInput
            placeholder={loading ? "Loading..." : "Add locale..."}
            data-testid={testid ? `${testid}-search` : undefined}
          />
          <ComboboxContent>
            <ComboboxList>
              <ComboboxEmpty>No matching locales</ComboboxEmpty>
              {available.map((l) => (
                <ComboboxItem
                  key={l.code}
                  value={l.code}
                  data-testid={testid ? `${testid}-option-${l.code}` : undefined}
                >
                  {l.display_name} <span className="text-xs opacity-60">({l.code})</span>
                </ComboboxItem>
              ))}
            </ComboboxList>
          </ComboboxContent>
        </Combobox>
      )}
    </div>
  );
}
