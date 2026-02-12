import { useState, useRef, useEffect, useMemo } from "react";
import { useLocales } from "../hooks/useLocales";

interface LocaleSelectProps {
  value: string;
  onChange: (value: string) => void;
  style?: React.CSSProperties;
  "data-testid"?: string;
}

/** Single-locale selector with search. Shows "French (fr)" in options, stores "fr". */
export function LocaleSelect({ value, onChange, style, ...rest }: LocaleSelectProps) {
  const { locales, loading } = useLocales();
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);

  const filtered = useMemo(() => {
    if (!search) return locales;
    const q = search.toLowerCase();
    return locales.filter(
      (l) =>
        l.display_name.toLowerCase().includes(q) ||
        l.code.toLowerCase().includes(q),
    );
  }, [locales, search]);

  const displayValue = useMemo(() => {
    const found = locales.find((l) => l.code === value);
    return found ? `${found.display_name} (${found.code})` : value;
  }, [locales, value]);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  return (
    <div ref={wrapperRef} className="relative" style={style} data-testid={rest["data-testid"]}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 w-full px-3 py-2 bg-muted border border-input rounded-md text-foreground text-sm cursor-pointer text-left"
        data-testid={rest["data-testid"] ? `${rest["data-testid"]}-trigger` : undefined}
      >
        {loading ? "Loading..." : displayValue || "Select locale..."}
        <span className="ml-auto opacity-50">{"\u25BE"}</span>
      </button>
      {open && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-popover border border-border rounded-md shadow-md z-50 overflow-hidden" onMouseDown={(e) => e.stopPropagation()}>
          <input
            type="text"
            placeholder="Search locales..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-full px-3 py-2 border-none border-b border-border bg-muted text-foreground text-[13px] outline-none box-border"
            autoFocus
            data-testid={rest["data-testid"] ? `${rest["data-testid"]}-search` : undefined}
          />
          <div className="max-h-60 overflow-y-auto">
            {filtered.map((l) => (
              <button
                key={l.code}
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onChange(l.code);
                  setOpen(false);
                  setSearch("");
                }}
                className={`block w-full px-3 py-1.5 border-none text-[13px] cursor-pointer text-left ${
                  l.code === value
                    ? "bg-primary text-primary-foreground"
                    : "bg-transparent text-foreground hover:bg-accent"
                }`}
                data-testid={rest["data-testid"] ? `${rest["data-testid"]}-option-${l.code}` : undefined}
              >
                {l.display_name} <span className="opacity-60 text-xs">({l.code})</span>
              </button>
            ))}
            {filtered.length === 0 && (
              <div className="px-3 py-2 text-xs text-muted-foreground">
                No matching locales
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

interface MultiLocaleSelectProps {
  value: string[];
  onChange: (value: string[]) => void;
  style?: React.CSSProperties;
  "data-testid"?: string;
}

/** Multi-locale chip input with search. Shows removable chips for each selected locale. */
export function MultiLocaleSelect({ value, onChange, style, ...rest }: MultiLocaleSelectProps) {
  const { locales, getDisplayName, loading } = useLocales();
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);
  const wrapperRef = useRef<HTMLDivElement>(null);

  const available = useMemo(() => {
    const selected = new Set(value);
    let list = locales.filter((l) => !selected.has(l.code));
    if (search) {
      const q = search.toLowerCase();
      list = list.filter(
        (l) =>
          l.display_name.toLowerCase().includes(q) ||
          l.code.toLowerCase().includes(q),
      );
    }
    return list;
  }, [locales, value, search]);

  useEffect(() => {
    const handleClick = (e: MouseEvent) => {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, []);

  const removeLocale = (code: string) => {
    onChange(value.filter((v) => v !== code));
  };

  const addLocale = (code: string) => {
    onChange([...value, code]);
    setSearch("");
  };

  return (
    <div ref={wrapperRef} className="relative" style={style} data-testid={rest["data-testid"]}>
      <div
        className="flex flex-wrap gap-1 px-2 py-1 bg-muted border border-input rounded-md min-h-9 items-center cursor-text"
        onClick={() => setOpen(true)}
        data-testid={rest["data-testid"] ? `${rest["data-testid"]}-chips` : undefined}
      >
        {value.map((code) => (
          <span key={code} className="inline-flex items-center gap-1 px-2 py-0.5 bg-primary text-primary-foreground rounded text-xs font-medium">
            {getDisplayName(code)} ({code})
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                removeLocale(code);
              }}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); removeLocale(code); } }}
              className="cursor-pointer text-sm opacity-80 hover:opacity-100"
              data-testid={rest["data-testid"] ? `${rest["data-testid"]}-remove-${code}` : undefined}
            >
              {"\u00D7"}
            </span>
          </span>
        ))}
        <input
          type="text"
          placeholder={value.length === 0 ? (loading ? "Loading..." : "Add locales...") : ""}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onFocus={() => setOpen(true)}
          className="flex-1 min-w-[80px] border-none bg-transparent text-foreground text-[13px] outline-none py-1"
          data-testid={rest["data-testid"] ? `${rest["data-testid"]}-search` : undefined}
        />
      </div>
      {open && (
        <div className="absolute top-full left-0 right-0 mt-1 bg-popover border border-border rounded-md shadow-md z-50 overflow-hidden" onMouseDown={(e) => e.preventDefault()}>
          <div className="max-h-60 overflow-y-auto">
            {available.map((l) => (
              <button
                key={l.code}
                type="button"
                onClick={(e) => { e.stopPropagation(); addLocale(l.code); }}
                className="block w-full px-3 py-1.5 border-none bg-transparent text-foreground text-[13px] cursor-pointer text-left hover:bg-accent"
                data-testid={rest["data-testid"] ? `${rest["data-testid"]}-option-${l.code}` : undefined}
              >
                {l.display_name} <span className="opacity-60 text-xs">({l.code})</span>
              </button>
            ))}
            {available.length === 0 && (
              <div className="px-3 py-2 text-xs text-muted-foreground">
                {search ? "No matching locales" : "All locales selected"}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
