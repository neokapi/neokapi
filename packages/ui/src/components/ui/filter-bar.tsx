import { useState, useCallback, useRef, useMemo, useEffect } from "react";
import { t } from "@neokapi/kapi-react/runtime";
import { createPortal } from "react-dom";
import { X, Search, Check, ChevronDown } from "lucide-react";
import { Badge } from "./badge";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** A parsed filter token like { key: "project", value: "my-app" } */
export interface FilterToken {
  key: string;
  value: string;
}

/** Definition of a filterable field — used for autocomplete suggestions */
export interface FilterField {
  /** The key used in the filter syntax, e.g. "project" */
  key: string;
  /** Human-readable label, e.g. "Project" */
  label: string;
  /** Hint text shown in the available filters list, e.g. "filter by project name" */
  hint?: string;
  /** Suggested values for autocomplete */
  values?: { value: string; label: string }[];
}

/** A quick-access preset filter (e.g. "Yesterday's activity") */
export interface FilterPreset {
  label: string;
  filters: FilterToken[];
  search?: string;
}

export interface FilterBarProps {
  /** Active filter tokens */
  filters: FilterToken[];
  /** Called when filters change (add/remove token, or free-text changes) */
  onFiltersChange: (filters: FilterToken[]) => void;
  /** Free-text search value */
  search: string;
  /** Called when free-text search changes */
  onSearchChange: (search: string) => void;
  /** Available filter fields for autocomplete */
  fields: FilterField[];
  /** Quick-access preset filters */
  presets?: FilterPreset[];
  /** Placeholder text */
  placeholder?: string;
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

function parseInput(
  raw: string,
  knownKeys: Set<string>,
): { tokens: FilterToken[]; freeText: string } {
  const tokens: FilterToken[] = [];
  const freeTextParts: string[] = [];
  const parts = raw.split(/\s+/);

  for (const part of parts) {
    const colonIdx = part.indexOf(":");
    if (colonIdx > 0) {
      const key = part.slice(0, colonIdx).toLowerCase();
      const value = part.slice(colonIdx + 1).replace(/^"|"$/g, "");
      if (knownKeys.has(key) && value) {
        tokens.push({ key, value });
        continue;
      }
    }
    if (part.trim()) freeTextParts.push(part);
  }

  return { tokens, freeText: freeTextParts.join(" ") };
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function FilterBar({
  filters,
  onFiltersChange,
  search,
  onSearchChange,
  fields,
  presets,
  placeholder = t("Search..."),
}: FilterBarProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [inputValue, setInputValue] = useState(search);
  const containerRef = useRef<HTMLDivElement>(null);

  // Inline autocomplete for key:value typing and filter key hints
  const autocompleteRef = useRef<HTMLDivElement>(null);
  const [showAutocomplete, setShowAutocomplete] = useState(false);
  const [selectedAC, setSelectedAC] = useState(0);
  const [inputFocused, setInputFocused] = useState(false);

  // Filters dropdown (separate button)
  const filtersBtnRef = useRef<HTMLButtonElement>(null);
  const filtersDropdownRef = useRef<HTMLDivElement>(null);
  const [showFiltersMenu, setShowFiltersMenu] = useState(false);
  const [filtersMenuField, setFiltersMenuField] = useState<FilterField | null>(null);

  const knownKeys = useMemo(() => new Set(fields.map((f) => f.key)), [fields]);

  // Check if a preset is active
  const isPresetActive = useCallback(
    (preset: FilterPreset) => {
      if (preset.search && preset.search !== search) return false;
      for (const pf of preset.filters) {
        if (!filters.some((f) => f.key === pf.key && f.value === pf.value)) return false;
      }
      return true;
    },
    [filters, search],
  );

  // Autocomplete suggestions for inline key:value typing
  type ACItem =
    | { type: "heading"; label: string }
    | { type: "field"; display: string; hint: string; action: () => void }
    | { type: "value"; display: string; detail: string; active: boolean; action: () => void };

  const acItems = useMemo<ACItem[]>(() => {
    const input = inputValue.trim().toLowerCase();

    // State: user typed "key:" — show values
    if (input.includes(":")) {
      const colonIdx = input.indexOf(":");
      const key = input.slice(0, colonIdx);
      const valuePart = input.slice(colonIdx + 1).toLowerCase();
      const field = fields.find((f) => f.key === key);
      if (!field?.values) return [];

      return field.values
        .filter(
          (v) =>
            v.value.toLowerCase().includes(valuePart) || v.label.toLowerCase().includes(valuePart),
        )
        .slice(0, 8)
        .map((v) => ({
          type: "value" as const,
          display: v.label,
          detail: key + ":" + v.value,
          active: filters.some((f) => f.key === key && f.value === v.value),
          action: () => {
            const isActive = filters.some((f) => f.key === key && f.value === v.value);
            if (isActive) {
              onFiltersChange(filters.filter((f) => !(f.key === key && f.value === v.value)));
            } else {
              onFiltersChange([...filters, { key, value: v.value }]);
            }
            setInputValue("");
            setShowAutocomplete(false);
          },
        }));
    }

    // State: empty or typing a prefix — show available filter keys
    const matchedFields = input
      ? fields.filter((f) => f.key.startsWith(input) || f.label.toLowerCase().startsWith(input))
      : fields;

    if (matchedFields.length === 0) return [];

    const items: ACItem[] = [{ type: "heading", label: t("Available filters") }];
    for (const f of matchedFields) {
      items.push({
        type: "field",
        display: f.key + ":",
        hint: f.hint ?? t("filter by {label}", { label: f.label.toLowerCase() }),
        action: () => {
          setInputValue(f.key + ":");
          inputRef.current?.focus();
        },
      });
    }
    return items;
  }, [inputValue, fields, filters, onFiltersChange]);

  const acActionItems = useMemo(
    () =>
      acItems.filter((item): item is ACItem & { action: () => void } => item.type !== "heading"),
    [acItems],
  );

  useEffect(() => {
    setSelectedAC(0);
  }, [acActionItems.length]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        if (showAutocomplete && acActionItems[selectedAC]) {
          acActionItems[selectedAC].action();
          return;
        }
        const { tokens, freeText } = parseInput(inputValue, knownKeys);
        if (tokens.length > 0) {
          onFiltersChange([...filters, ...tokens]);
          // Keep only the free text part in the input after extracting tokens.
          setInputValue(freeText);
        }
        onSearchChange(freeText);
        setShowAutocomplete(false);
      } else if (e.key === "Backspace" && inputValue === "" && filters.length > 0) {
        onFiltersChange(filters.slice(0, -1));
      } else if (e.key === "ArrowDown" && showAutocomplete) {
        e.preventDefault();
        setSelectedAC((s) => Math.min(s + 1, acActionItems.length - 1));
      } else if (e.key === "ArrowUp" && showAutocomplete) {
        e.preventDefault();
        setSelectedAC((s) => Math.max(s - 1, 0));
      } else if (e.key === "Escape") {
        setShowAutocomplete(false);
      }
    },
    [
      inputValue,
      filters,
      onFiltersChange,
      onSearchChange,
      knownKeys,
      showAutocomplete,
      acActionItems,
      selectedAC,
    ],
  );

  const handleInputChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(e.target.value);
    setShowAutocomplete(true);
  }, []);

  const removeFilter = useCallback(
    (index: number) => onFiltersChange(filters.filter((_, i) => i !== index)),
    [filters, onFiltersChange],
  );

  // Close dropdowns on outside click
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (
        autocompleteRef.current &&
        !autocompleteRef.current.contains(target) &&
        containerRef.current &&
        !containerRef.current.contains(target)
      ) {
        setShowAutocomplete(false);
      }
      if (
        filtersDropdownRef.current &&
        !filtersDropdownRef.current.contains(target) &&
        filtersBtnRef.current &&
        !filtersBtnRef.current.contains(target)
      ) {
        setShowFiltersMenu(false);
        setFiltersMenuField(null);
      }
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, []);

  const togglePreset = useCallback(
    (preset: FilterPreset) => {
      if (isPresetActive(preset)) {
        const presetKeys = new Set(preset.filters.map((f) => f.key + ":" + f.value));
        onFiltersChange(filters.filter((f) => !presetKeys.has(f.key + ":" + f.value)));
        if (preset.search) onSearchChange("");
      } else {
        onFiltersChange([...filters, ...preset.filters]);
        if (preset.search) onSearchChange(preset.search);
      }
    },
    [filters, search, onFiltersChange, onSearchChange, isPresetActive],
  );

  const toggleFieldValue = useCallback(
    (key: string, value: string) => {
      const isActive = filters.some((f) => f.key === key && f.value === value);
      if (isActive) {
        onFiltersChange(filters.filter((f) => !(f.key === key && f.value === value)));
      } else {
        onFiltersChange([...filters, { key, value }]);
      }
    },
    [filters, onFiltersChange],
  );

  return (
    <div ref={containerRef} className="flex items-center gap-2">
      {/* Filters dropdown button */}
      <div className="relative">
        <button
          ref={filtersBtnRef}
          onClick={() => {
            setShowFiltersMenu(!showFiltersMenu);
            setFiltersMenuField(null);
          }}
          className={
            "inline-flex items-center gap-1.5 px-3 py-1.5 rounded-md border text-sm font-medium " +
            "transition-colors cursor-pointer bg-transparent " +
            (showFiltersMenu
              ? "border-ring ring-1 ring-ring text-foreground"
              : "border-border/50 text-muted-foreground hover:text-foreground hover:border-border")
          }
        >
          Filters
          <ChevronDown className="w-3 h-3" />
        </button>

        {/* Filters menu — rendered via portal */}
        {showFiltersMenu &&
          filtersBtnRef.current &&
          createPortal(
            <div
              ref={filtersDropdownRef}
              style={{
                position: "fixed",
                top: filtersBtnRef.current.getBoundingClientRect().bottom + 4,
                left: filtersBtnRef.current.getBoundingClientRect().left,
                width: 280,
                zIndex: 9999,
              }}
              className="rounded-md border border-border/50 bg-popover shadow-lg overflow-hidden max-h-[380px] overflow-y-auto"
            >
              {/* Back to top level when viewing a field's values */}
              {filtersMenuField ? (
                <>
                  <div className="flex items-center justify-between px-3 py-2.5 border-b border-border/30">
                    <button
                      onClick={() => setFiltersMenuField(null)}
                      className="text-xs text-muted-foreground hover:text-foreground bg-transparent border-none cursor-pointer p-0"
                    >
                      ← Back
                    </button>
                    <span className="text-xs font-semibold text-muted-foreground">
                      {filtersMenuField.label}
                    </span>
                    <button
                      onClick={() => {
                        setShowFiltersMenu(false);
                        setFiltersMenuField(null);
                      }}
                      className="p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>
                  {filtersMenuField.values?.map((v) => {
                    const isActive = filters.some(
                      (f) => f.key === filtersMenuField.key && f.value === v.value,
                    );
                    return (
                      <button
                        key={v.value}
                        onClick={() => toggleFieldValue(filtersMenuField.key, v.value)}
                        className="w-full flex items-center gap-2 px-3 py-2 text-sm text-left border-none cursor-pointer transition-colors bg-transparent text-foreground hover:bg-accent/50"
                      >
                        <span className="w-4 h-4 flex items-center justify-center shrink-0">
                          {isActive && <Check className="w-3.5 h-3.5 text-primary" />}
                        </span>
                        <span className="flex-1">{v.label}</span>
                      </button>
                    );
                  })}
                </>
              ) : (
                <>
                  {/* Header */}
                  <div className="flex items-center justify-between px-3 py-2.5 border-b border-border/30">
                    <span className="text-xs font-semibold text-muted-foreground">Filter</span>
                    <button
                      onClick={() => setShowFiltersMenu(false)}
                      className="p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground"
                    >
                      <X className="w-3.5 h-3.5" />
                    </button>
                  </div>

                  {/* Presets */}
                  {presets && presets.length > 0 && (
                    <>
                      {presets.map((preset) => (
                        <button
                          key={preset.label}
                          onClick={() => togglePreset(preset)}
                          className="w-full flex items-center gap-2 px-3 py-2 text-sm text-left border-none cursor-pointer transition-colors bg-transparent text-foreground hover:bg-accent/50"
                        >
                          <span className="w-4 h-4 flex items-center justify-center shrink-0">
                            {isPresetActive(preset) && (
                              <Check className="w-3.5 h-3.5 text-primary" />
                            )}
                          </span>
                          <span>{preset.label}</span>
                        </button>
                      ))}
                      <div className="border-t border-border/30 my-1" />
                    </>
                  )}

                  {/* Field keys — drill into values */}
                  {fields.map((field) => (
                    <button
                      key={field.key}
                      onClick={() => (field.values ? setFiltersMenuField(field) : undefined)}
                      className="w-full flex items-center gap-3 px-3 py-2 text-sm text-left border-none cursor-pointer transition-colors bg-transparent text-foreground hover:bg-accent/50"
                    >
                      <span className="font-medium w-[80px] shrink-0">{field.key}:</span>
                      <span className="text-muted-foreground text-[12px]">
                        {field.hint ?? "filter by " + field.label.toLowerCase()}
                      </span>
                    </button>
                  ))}
                </>
              )}
            </div>,
            document.body,
          )}
      </div>

      {/* Search input with token badges */}
      <div className="flex-1 min-w-[200px]">
        <div
          className="flex items-center flex-wrap gap-1 px-3 py-1.5 rounded-md border border-border/50 bg-transparent
                     focus-within:ring-1 focus-within:ring-ring transition-shadow cursor-text min-h-[36px]"
          onClick={() => inputRef.current?.focus()}
        >
          <Search className="w-3.5 h-3.5 text-muted-foreground shrink-0" />

          {filters.map((token, i) => (
            <Badge
              key={token.key + "-" + token.value + "-" + i}
              variant="secondary"
              className="gap-1 pl-1.5 pr-1 py-0 text-[11px] font-mono shrink-0"
            >
              <span className="text-muted-foreground">{token.key}:</span>
              <span>{token.value}</span>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  removeFilter(i);
                }}
                className="ml-0.5 p-0 bg-transparent border-none cursor-pointer text-muted-foreground hover:text-foreground transition-colors"
              >
                <X className="w-3 h-3" />
              </button>
            </Badge>
          ))}

          <input
            ref={inputRef}
            data-testid="filterbar-search"
            type="text"
            value={inputValue}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            onFocus={() => {
              setInputFocused(true);
              setShowAutocomplete(true);
            }}
            onBlur={() => setInputFocused(false)}
            placeholder={filters.length === 0 && !inputValue ? placeholder : ""}
            className="flex-1 min-w-[120px] bg-transparent border-none outline-none text-sm text-foreground placeholder:text-muted-foreground"
          />
        </div>

        {/* Inline autocomplete — shows filter key hints on focus, values when typing key: */}
        {showAutocomplete &&
          inputFocused &&
          acItems.length > 0 &&
          containerRef.current &&
          createPortal(
            <div
              ref={autocompleteRef}
              style={{
                position: "fixed",
                top: containerRef.current.getBoundingClientRect().bottom + 4,
                left: containerRef.current.getBoundingClientRect().left,
                width: Math.min(containerRef.current.getBoundingClientRect().width, 380),
                zIndex: 9999,
              }}
              className="rounded-md border border-border/50 bg-popover shadow-lg overflow-hidden max-h-[300px] overflow-y-auto"
              onMouseDown={(e) => e.preventDefault()}
            >
              {acItems.map((item, i) => {
                if (item.type === "heading") {
                  return (
                    <div
                      key={"h-" + i}
                      className="px-3 py-1.5 text-[10px] font-medium text-muted-foreground/70 uppercase tracking-wider"
                    >
                      {item.label}
                    </div>
                  );
                }
                if (item.type === "field") {
                  const actionIdx = acActionItems.indexOf(item as any);
                  return (
                    <button
                      key={item.display}
                      onClick={item.action}
                      className={
                        "w-full flex items-center gap-2.5 px-3 py-1.5 text-left border-none cursor-pointer transition-colors " +
                        (actionIdx === selectedAC
                          ? "bg-accent/60"
                          : "bg-transparent hover:bg-accent/40")
                      }
                    >
                      <span className="text-[12px] font-semibold text-foreground/80 shrink-0">
                        {item.display}
                      </span>
                      <span className="text-[12px] text-muted-foreground/60">{item.hint}</span>
                    </button>
                  );
                }
                if (item.type === "value") {
                  const actionIdx = acActionItems.indexOf(item as any);
                  return (
                    <button
                      key={item.detail}
                      onClick={item.action}
                      className={
                        "w-full flex items-center gap-2 px-3 py-1.5 text-left border-none cursor-pointer transition-colors " +
                        (actionIdx === selectedAC
                          ? "bg-accent/60"
                          : "bg-transparent hover:bg-accent/40")
                      }
                    >
                      <span className="w-3.5 h-3.5 flex items-center justify-center shrink-0">
                        {item.active && <Check className="w-3 h-3 text-primary" />}
                      </span>
                      <span className="text-[12px] flex-1">{item.display}</span>
                      <span className="text-[10px] text-muted-foreground/50 font-mono">
                        {item.detail}
                      </span>
                    </button>
                  );
                }
                return null;
              })}
            </div>,
            document.body,
          )}
      </div>
    </div>
  );
}
