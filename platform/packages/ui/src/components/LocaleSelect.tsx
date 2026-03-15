import { useState, useRef, useEffect, useMemo, useCallback } from "react";
import { Search } from "lucide-react";
import { useLocales } from "../hooks/useLocales";
import { Combobox, ComboboxInput, ComboboxContent, ComboboxList, ComboboxItem, ComboboxEmpty } from "./ui/combobox";

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

/** Single-locale selector with search. Shows "French (fr)" in options, stores "fr". */
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

  return (
    <div style={style} className={className} data-testid={rest["data-testid"]}>
      <Combobox
        value={value}
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

/** Multi-locale chip input with search. Shows removable chips for each selected locale. */
export function MultiLocaleSelect({ value, onChange, codes, style, ...rest }: MultiLocaleSelectProps) {
  const { locales, getDisplayName, loading } = useLocales();
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const available = useMemo(() => {
    const selected = new Set(value);
    const codeSet = codes ? new Set(codes) : null;
    let list = locales.filter((l) => !selected.has(l.code) && (!codeSet || codeSet.has(l.code)));
    if (search) {
      const q = search.toLowerCase();
      list = list.filter(
        (l) => l.display_name.toLowerCase().includes(q) || l.code.toLowerCase().includes(q),
      );
    }
    return list;
  }, [locales, value, search]);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
        setSearch("");
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  // Focus the search input when the dropdown opens
  useEffect(() => {
    if (open) {
      // Small delay to let the dropdown render before focusing
      requestAnimationFrame(() => searchRef.current?.focus());
    }
  }, [open]);

  const removeLocale = (code: string) => {
    onChange(value.filter((v) => v !== code));
  };

  const addLocale = useCallback(
    (code: string) => {
      onChange([...value, code]);
      setSearch("");
      // Re-focus search after adding
      requestAnimationFrame(() => searchRef.current?.focus());
    },
    [value, onChange],
  );

  return (
    <div ref={wrapperRef} className="relative" style={style} data-testid={rest["data-testid"]}>
      <div
        className="flex flex-wrap gap-2 px-4 py-2.5 rounded-xl min-h-[44px] items-center cursor-pointer transition-all duration-300 backdrop-blur-sm"
        style={{
          background: "var(--input-bg)",
          border: "1px solid var(--input-border)",
          color: "var(--input-text)",
        }}
        onClick={() => setOpen(!open)}
        data-testid={rest["data-testid"] ? `${rest["data-testid"]}-chips` : undefined}
      >
        {value.length === 0 && (
          <span className="text-sm opacity-50">{loading ? "Loading..." : "Select locales..."}</span>
        )}
        {value.map((code) => (
          <span
            key={code}
            className="inline-flex items-center gap-1 px-2 py-0.5 bg-primary text-primary-foreground rounded-md text-xs font-medium"
          >
            {getDisplayName(code)} ({code})
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                removeLocale(code);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" || e.key === " ") {
                  e.preventDefault();
                  removeLocale(code);
                }
              }}
              className="cursor-pointer text-sm opacity-80 hover:opacity-100"
              data-testid={
                rest["data-testid"] ? `${rest["data-testid"]}-remove-${code}` : undefined
              }
            >
              {"\u00D7"}
            </span>
          </span>
        ))}
      </div>
      {open && (
        <div
          className="absolute top-full left-0 right-0 mt-1 rounded-xl shadow-md z-50 overflow-hidden backdrop-blur-md"
          style={{
            background: "var(--dropdown-bg)",
            border: "1px solid var(--dropdown-border)",
            boxShadow: "var(--dropdown-glow)",
          }}
          onMouseDown={(e) => e.preventDefault()}
        >
          <div
            className="flex items-center gap-2 px-3 py-2"
            style={{ borderBottom: "1px solid var(--dropdown-border)" }}
          >
            <Search
              className="h-4 w-4 shrink-0"
              style={{ color: "var(--text-muted)", opacity: 0.8 }}
            />
            <input
              ref={searchRef}
              type="text"
              placeholder="Search locales..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="flex-1 border-none bg-transparent text-sm outline-none font-medium"
              style={{ color: "var(--input-text)" }}
              data-testid={rest["data-testid"] ? `${rest["data-testid"]}-search` : undefined}
            />
          </div>
          <div className="max-h-60 overflow-y-auto p-1.5">
            {available.map((l) => (
              <button
                key={l.code}
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  addLocale(l.code);
                }}
                className="block w-full px-3 py-1.5 border-none bg-transparent text-sm cursor-pointer text-left rounded-lg"
                style={{ color: "var(--dropdown-item-text)" }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.background = "var(--dropdown-item-hover)";
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.background = "transparent";
                }}
                data-testid={
                  rest["data-testid"] ? `${rest["data-testid"]}-option-${l.code}` : undefined
                }
              >
                {l.display_name} <span className="opacity-60 text-xs">({l.code})</span>
              </button>
            ))}
            {available.length === 0 && (
              <div className="px-3 py-2 text-xs" style={{ color: "var(--text-muted)" }}>
                {search ? "No matching locales" : "All locales selected"}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
