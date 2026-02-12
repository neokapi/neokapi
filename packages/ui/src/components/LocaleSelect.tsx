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
    <div ref={wrapperRef} style={{ position: "relative", ...style }} data-testid={rest["data-testid"]}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        style={triggerStyle}
        data-testid={rest["data-testid"] ? `${rest["data-testid"]}-trigger` : undefined}
      >
        {loading ? "Loading..." : displayValue || "Select locale..."}
        <span style={{ marginLeft: "auto", opacity: 0.5 }}>{"\u25BE"}</span>
      </button>
      {open && (
        <div style={dropdownStyle} onMouseDown={(e) => e.stopPropagation()}>
          <input
            type="text"
            placeholder="Search locales..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            style={searchInputStyle}
            autoFocus
            data-testid={rest["data-testid"] ? `${rest["data-testid"]}-search` : undefined}
          />
          <div style={listStyle}>
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
                style={{
                  ...optionStyle,
                  backgroundColor: l.code === value ? "var(--accent)" : "transparent",
                  color: l.code === value ? "#fff" : "var(--text-primary)",
                }}
                data-testid={rest["data-testid"] ? `${rest["data-testid"]}-option-${l.code}` : undefined}
              >
                {l.display_name} <span style={{ opacity: 0.6, fontSize: 12 }}>({l.code})</span>
              </button>
            ))}
            {filtered.length === 0 && (
              <div style={{ padding: "8px 12px", fontSize: 12, color: "var(--text-secondary)" }}>
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
    <div ref={wrapperRef} style={{ position: "relative", ...style }} data-testid={rest["data-testid"]}>
      <div
        style={chipContainerStyle}
        onClick={() => setOpen(true)}
        data-testid={rest["data-testid"] ? `${rest["data-testid"]}-chips` : undefined}
      >
        {value.map((code) => (
          <span key={code} style={chipStyle}>
            {getDisplayName(code)} ({code})
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                removeLocale(code);
              }}
              onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); removeLocale(code); } }}
              style={chipRemoveStyle}
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
          style={chipInputStyle}
          data-testid={rest["data-testid"] ? `${rest["data-testid"]}-search` : undefined}
        />
      </div>
      {open && (
        <div style={dropdownStyle} onMouseDown={(e) => e.preventDefault()}>
          <div style={listStyle}>
            {available.map((l) => (
              <button
                key={l.code}
                type="button"
                onClick={(e) => { e.stopPropagation(); addLocale(l.code); }}
                style={optionStyle}
                data-testid={rest["data-testid"] ? `${rest["data-testid"]}-option-${l.code}` : undefined}
              >
                {l.display_name} <span style={{ opacity: 0.6, fontSize: 12 }}>({l.code})</span>
              </button>
            ))}
            {available.length === 0 && (
              <div style={{ padding: "8px 12px", fontSize: 12, color: "var(--text-secondary)" }}>
                {search ? "No matching locales" : "All locales selected"}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

const triggerStyle: React.CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 8,
  width: "100%",
  padding: "8px 12px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  color: "var(--text-primary)",
  fontSize: 14,
  cursor: "pointer",
  textAlign: "left",
};

const dropdownStyle: React.CSSProperties = {
  position: "absolute",
  top: "100%",
  left: 0,
  right: 0,
  marginTop: 4,
  backgroundColor: "var(--bg-secondary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  boxShadow: "0 4px 12px rgba(0,0,0,0.15)",
  zIndex: 100,
  overflow: "hidden",
};

const searchInputStyle: React.CSSProperties = {
  width: "100%",
  padding: "8px 12px",
  border: "none",
  borderBottom: "1px solid var(--border)",
  backgroundColor: "var(--bg-tertiary)",
  color: "var(--text-primary)",
  fontSize: 13,
  outline: "none",
  boxSizing: "border-box",
};

const listStyle: React.CSSProperties = {
  maxHeight: 240,
  overflowY: "auto",
};

const optionStyle: React.CSSProperties = {
  display: "block",
  width: "100%",
  padding: "6px 12px",
  border: "none",
  background: "transparent",
  color: "var(--text-primary)",
  fontSize: 13,
  cursor: "pointer",
  textAlign: "left",
};

const chipContainerStyle: React.CSSProperties = {
  display: "flex",
  flexWrap: "wrap",
  gap: 4,
  padding: "4px 8px",
  backgroundColor: "var(--bg-tertiary)",
  border: "1px solid var(--border)",
  borderRadius: 6,
  minHeight: 36,
  alignItems: "center",
  cursor: "text",
};

const chipStyle: React.CSSProperties = {
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "2px 8px",
  backgroundColor: "var(--accent)",
  color: "#fff",
  borderRadius: 4,
  fontSize: 12,
  fontWeight: 500,
};

const chipRemoveStyle: React.CSSProperties = {
  background: "none",
  border: "none",
  color: "#fff",
  cursor: "pointer",
  fontSize: 14,
  padding: 0,
  lineHeight: 1,
  opacity: 0.8,
};

const chipInputStyle: React.CSSProperties = {
  flex: 1,
  minWidth: 80,
  border: "none",
  background: "transparent",
  color: "var(--text-primary)",
  fontSize: 13,
  outline: "none",
  padding: "4px 0",
};
