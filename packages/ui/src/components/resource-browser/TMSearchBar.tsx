import { useState, useCallback, useRef, useEffect, useMemo } from "react";
import type { EntityAnnotationDTO, TMMatchDTO, LookupTMRequest } from "./types";
import { ENTITY_TYPES } from "./types";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { MatchScoreBar } from "./MatchScoreBar";
import { LocalePill } from "./LocalePill";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "../ui/collapsible";
import { Search, ChevronDown, X } from "lucide-react";
import { cn } from "../../lib/utils";

interface MarkedEntity extends EntityAnnotationDTO {
  id: string;
}

/** A parsed filter token like { key: "language", value: "fr-FR" } */
export interface FilterToken {
  key: string;
  value: string;
}

/** Definition of a filterable field */
export interface FilterField {
  key: string;
  label: string;
  values?: { value: string; label: string }[];
}

interface TMSearchBarProps {
  /** Current submitted search value (drives the results). */
  value: string;
  /** Called when the user submits a search (Enter or icon click). */
  onChange: (value: string) => void;
  /** Active filter tokens. */
  filters?: FilterToken[];
  /** Called when filter tokens change. */
  onFiltersChange?: (filters: FilterToken[]) => void;
  /** Available filter fields for tokens (e.g. language, concept). */
  filterFields?: FilterField[];
  /** Optional fuzzy TM lookup — triggered when entities are marked + Enter. */
  onLookup?: (req: LookupTMRequest) => Promise<TMMatchDTO[]>;
  /** Called whenever the marked entity list changes — used for filter wiring. */
  onEntitiesChange?: (entities: EntityAnnotationDTO[]) => void;
  sourceLocale: string;
  targetLocale: string;
  placeholder?: string;
}

/**
 * Google-style search bar for the TM browser.
 * - Centered, rounded, large input.
 * - Search is submitted on Enter or search icon click (NOT as you type).
 * - Filter tokens appear as inline badges at the left of the input.
 * - Select text to mark as entities; entities feed into the parent's filter.
 */
export function TMSearchBar({
  value,
  onChange,
  filters = [],
  onFiltersChange,
  filterFields = [],
  onLookup,
  onEntitiesChange,
  sourceLocale,
  targetLocale,
  placeholder = "Search translation memory...",
}: TMSearchBarProps) {
  const [draft, setDraft] = useState(value);
  const [entities, setEntities] = useState<MarkedEntity[]>([]);
  const [showEntityPopover, setShowEntityPopover] = useState(false);
  const [selectionRange, setSelectionRange] = useState<{
    start: number;
    end: number;
    text: string;
  } | null>(null);
  const [matches, setMatches] = useState<TMMatchDTO[]>([]);
  const [showMatches, setShowMatches] = useState(false);
  const [lookupLoading, setLookupLoading] = useState(false);
  const [showFilterMenu, setShowFilterMenu] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const entityIdRef = useRef(0);

  // Sync draft with external value (e.g. when parent clears search).
  useEffect(() => {
    setDraft(value);
  }, [value]);

  // Propagate entity changes to parent for filter wiring.
  useEffect(() => {
    if (onEntitiesChange) {
      onEntitiesChange(entities.map(({ text, type, start, end }) => ({ text, type, start, end })));
    }
  }, [entities, onEntitiesChange]);

  const submit = useCallback(async () => {
    onChange(draft);
    if (onLookup && entities.length > 0 && draft.trim()) {
      setLookupLoading(true);
      try {
        const result = await onLookup({
          text: draft,
          entities: entities.map(({ text, type, start, end }) => ({ text, type, start, end })),
          source_locale: sourceLocale,
          target_locale: targetLocale,
          min_score: 0.5,
          max_results: 10,
        });
        setMatches(result);
        setShowMatches(true);
      } finally {
        setLookupLoading(false);
      }
    }
  }, [onChange, draft, onLookup, entities, sourceLocale, targetLocale]);

  const handleInputKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>) => {
      if (e.key === "Enter") {
        e.preventDefault();
        void submit();
      } else if (e.key === "Backspace" && draft === "" && filters.length > 0 && onFiltersChange) {
        // Backspace on empty input removes the last filter token.
        onFiltersChange(filters.slice(0, -1));
      }
    },
    [submit, draft, filters, onFiltersChange],
  );

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newText = e.target.value;
      setDraft(newText);
      if (entities.length > 0) {
        const valid = entities.filter((ent) => {
          if (ent.end > newText.length) return false;
          return newText.substring(ent.start, ent.end) === ent.text;
        });
        if (valid.length !== entities.length) setEntities(valid);
      }
    },
    [entities],
  );

  const handleTextSelect = useCallback(() => {
    if (!onLookup) return;
    const el = inputRef.current;
    if (!el) return;
    const start = el.selectionStart ?? 0;
    const end = el.selectionEnd ?? 0;
    if (start === end) {
      setShowEntityPopover(false);
      return;
    }
    setSelectionRange({ start, end, text: draft.substring(start, end) });
    setShowEntityPopover(true);
  }, [draft, onLookup]);

  const markEntity = useCallback(
    (entityType: string) => {
      if (!selectionRange) return;
      const newEntity: MarkedEntity = {
        id: `e-${++entityIdRef.current}`,
        text: selectionRange.text,
        type: entityType,
        start: selectionRange.start,
        end: selectionRange.end,
      };
      setEntities((prev) =>
        [
          ...prev.filter((e) => e.end <= newEntity.start || e.start >= newEntity.end),
          newEntity,
        ].sort((a, b) => a.start - b.start),
      );
      setShowEntityPopover(false);
    },
    [selectionRange],
  );

  const removeEntity = useCallback((id: string) => {
    setEntities((prev) => prev.filter((e) => e.id !== id));
  }, []);

  const removeFilter = useCallback(
    (idx: number) => {
      if (!onFiltersChange) return;
      onFiltersChange(filters.filter((_, i) => i !== idx));
    },
    [filters, onFiltersChange],
  );

  const addFilter = useCallback(
    (key: string, value: string) => {
      if (!onFiltersChange) return;
      // Replace existing token with same key.
      const next = filters.filter((f) => f.key !== key);
      next.push({ key, value });
      onFiltersChange(next);
      setShowFilterMenu(false);
      inputRef.current?.focus();
    },
    [filters, onFiltersChange],
  );

  const fieldsByKey = useMemo(() => {
    const m = new Map<string, FilterField>();
    for (const f of filterFields) m.set(f.key, f);
    return m;
  }, [filterFields]);

  const filterLabel = useCallback(
    (token: FilterToken) => {
      const field = fieldsByKey.get(token.key);
      const valueLabel = field?.values?.find((v) => v.value === token.value)?.label ?? token.value;
      return { key: field?.label ?? token.key, value: valueLabel };
    },
    [fieldsByKey],
  );

  return (
    <div className="flex flex-col items-center gap-3 w-full">
      {/* Main search bar — centered, Google-like */}
      <div className="relative w-full max-w-2xl">
        <div className="flex items-center gap-1.5 w-full rounded-full border border-input bg-background pl-4 pr-1 py-1 shadow-sm hover:shadow transition-shadow focus-within:shadow-md focus-within:border-ring">
          {/* Filter tokens inline */}
          {filters.map((token, i) => {
            const { key, value } = filterLabel(token);
            const isLanguage = token.key === "language";
            return (
              <Badge key={i} variant="secondary" className="gap-1 pr-1 rounded-full">
                {isLanguage ? (
                  <LocalePill locale={token.value} />
                ) : (
                  <>
                    <span className="text-[9px] uppercase tracking-wider text-muted-foreground">
                      {key}
                    </span>
                    <span>{value}</span>
                  </>
                )}
                {onFiltersChange && (
                  <button
                    onClick={() => removeFilter(i)}
                    className="text-muted-foreground hover:text-destructive transition-colors ml-0.5"
                    aria-label={`Remove ${key} filter`}
                  >
                    <X className="size-2.5" />
                  </button>
                )}
              </Badge>
            );
          })}

          <input
            ref={inputRef}
            data-testid="tm-search"
            type="text"
            value={draft}
            onChange={handleTextChange}
            onMouseUp={handleTextSelect}
            onKeyUp={handleTextSelect}
            onKeyDown={handleInputKeyDown}
            placeholder={filters.length === 0 ? placeholder : ""}
            className="flex-1 min-w-0 bg-transparent text-base outline-none border-none placeholder:text-muted-foreground"
          />

          {/* Filter chip dropdown */}
          {filterFields.length > 0 && onFiltersChange && (
            <div className="relative">
              <Button
                variant="ghost"
                size="icon-xs"
                onClick={() => setShowFilterMenu((v) => !v)}
                className="rounded-full"
                title="Add filter"
              >
                <ChevronDown className="size-4" />
              </Button>
              {showFilterMenu && (
                <div className="absolute z-50 right-0 top-full mt-2 w-56 rounded-lg border bg-popover p-1 shadow-lg">
                  {filterFields.map((field) => (
                    <div key={field.key} className="p-1">
                      <div className="text-[10px] uppercase tracking-wider text-muted-foreground px-1.5 py-1">
                        {field.label}
                      </div>
                      <div
                        className={
                          field.key === "language" ? "flex flex-wrap gap-1 px-1.5 py-1" : undefined
                        }
                      >
                        {field.values?.map((v) =>
                          field.key === "language" ? (
                            <button
                              key={v.value}
                              onClick={() => addFilter(field.key, v.value)}
                              className="hover:opacity-80 transition-opacity"
                            >
                              <LocalePill locale={v.value} />
                            </button>
                          ) : (
                            <button
                              key={v.value}
                              onClick={() => addFilter(field.key, v.value)}
                              className="flex w-full items-center rounded px-1.5 py-1 text-sm hover:bg-accent text-left"
                            >
                              {v.label}
                            </button>
                          ),
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Search button */}
          <Button
            onClick={() => void submit()}
            size="icon"
            className="rounded-full size-9 shrink-0"
            title="Search (Enter)"
            disabled={lookupLoading}
          >
            <Search className="size-4" />
          </Button>
        </div>

        {/* Entity popover (absolutely positioned below selection) */}
        {showEntityPopover && selectionRange && (
          <div className="absolute z-50 top-full left-8 mt-2 rounded-lg border bg-popover shadow-lg p-1.5 flex flex-wrap gap-1 max-w-md">
            <span className="text-[10px] text-muted-foreground px-1 py-0.5 w-full">
              Mark &ldquo;{selectionRange.text}&rdquo; as:
            </span>
            {ENTITY_TYPES.map((et) => (
              <Button
                key={et.value}
                variant="outline"
                size="sm"
                className="h-5 text-[10px] px-2"
                onClick={() => markEntity(et.value)}
              >
                {et.label}
              </Button>
            ))}
          </div>
        )}
      </div>

      {/* Entity chips */}
      {entities.length > 0 && (
        <div className="flex flex-wrap items-center gap-1 w-full max-w-2xl justify-center">
          {entities.map((e) => {
            const label = ENTITY_TYPES.find((t) => t.value === e.type)?.label ?? e.type;
            return (
              <Badge key={e.id} variant="outline" className="gap-1 pr-1 rounded-full">
                <span className="text-muted-foreground uppercase tracking-wider text-[9px]">
                  {label}
                </span>
                <span>{e.text}</span>
                <button
                  onClick={() => removeEntity(e.id)}
                  className="text-muted-foreground hover:text-destructive transition-colors"
                  aria-label={`Remove ${e.text} entity`}
                >
                  <X className="size-2.5" />
                </button>
              </Badge>
            );
          })}
          <span className="text-[10px] text-muted-foreground ml-1">
            {lookupLoading ? "Looking up..." : onLookup ? "Enter to lookup" : ""}
          </span>
        </div>
      )}

      {/* Lookup results (collapsible) */}
      {matches.length > 0 && (
        <div className="w-full max-w-2xl">
          <Collapsible open={showMatches} onOpenChange={setShowMatches}>
            <CollapsibleTrigger className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-muted-foreground hover:text-foreground transition-colors rounded-lg border bg-card/50">
              <ChevronDown
                className={cn("size-3 transition-transform", showMatches && "rotate-180")}
              />
              {matches.length} {matches.length === 1 ? "match" : "matches"} found
              {!showMatches && (
                <span className="text-[10px] ml-auto">
                  Best: {Math.round(matches[0].score * 100)}%
                </span>
              )}
            </CollapsibleTrigger>
            <CollapsibleContent>
              <div className="flex flex-col gap-1.5 px-3 pb-2 pt-1">
                {matches.map((m, i) => {
                  const srcKey = m.entry.hint_src_lang || sourceLocale;
                  const src = srcKey ? m.entry.variants[srcKey] : undefined;
                  const tgt = targetLocale ? m.entry.variants[targetLocale] : undefined;
                  return (
                    <div key={i} className="rounded-lg border p-2">
                      <MatchScoreBar score={m.score} matchType={m.match_type} className="mb-1.5" />
                      {src && (
                        <div className="flex items-start gap-2 mb-0.5">
                          <LocalePill locale={src.locale} />
                          <CodedTextDisplay
                            text={src.text}
                            runs={src.runs}
                            className="text-[12px] text-foreground flex-1"
                          />
                        </div>
                      )}
                      {tgt && (
                        <div className="flex items-start gap-2">
                          <LocalePill locale={tgt.locale} />
                          <CodedTextDisplay
                            text={tgt.text}
                            runs={tgt.runs}
                            className="text-[12px] text-muted-foreground flex-1"
                          />
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            </CollapsibleContent>
          </Collapsible>
        </div>
      )}
    </div>
  );
}
