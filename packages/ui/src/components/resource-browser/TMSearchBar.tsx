import { useState, useCallback, useRef } from "react";
import type { EntityAnnotationDTO, TMMatchDTO, LookupTMRequest } from "./types";
import { ENTITY_TYPES } from "./types";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { MatchScoreBar } from "./MatchScoreBar";
import { LocalePill } from "./LocalePill";
import { Search, ChevronDown, ChevronUp, X } from "lucide-react";

interface MarkedEntity extends EntityAnnotationDTO {
  id: string;
}

interface TMSearchBarProps {
  value: string;
  onChange: (value: string) => void;
  /** Called with entities when the user triggers a lookup (Enter with entities marked). */
  onLookup?: (req: LookupTMRequest) => Promise<TMMatchDTO[]>;
  sourceLocale: string;
  targetLocale: string;
  placeholder?: string;
  /** Actions to render on the right side of the search bar (e.g., view toggle, add button). */
  actions?: React.ReactNode;
}

/**
 * Combined search bar with inline entity annotation for TM lookup.
 * In plain search mode, acts as a text search input.
 * When text is selected, an entity popover lets users mark entities.
 * With entities marked, Enter triggers a fuzzy TM lookup.
 */
export function TMSearchBar({
  value,
  onChange,
  onLookup,
  sourceLocale,
  targetLocale,
  placeholder = "Search translation memory...",
  actions,
}: TMSearchBarProps) {
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
  const inputRef = useRef<HTMLInputElement>(null);
  const entityIdRef = useRef(0);

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const newText = e.target.value;
      onChange(newText);
      // Validate entity positions
      if (entities.length > 0) {
        const valid = entities.filter((ent) => {
          if (ent.end > newText.length) return false;
          return newText.substring(ent.start, ent.end) === ent.text;
        });
        if (valid.length !== entities.length) {
          setEntities(valid);
        }
      }
    },
    [onChange, entities],
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
    const selectedText = value.substring(start, end);
    setSelectionRange({ start, end, text: selectedText });
    setShowEntityPopover(true);
  }, [value, onLookup]);

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

  const handleKeyDown = useCallback(
    async (e: React.KeyboardEvent) => {
      if (e.key === "Enter" && onLookup && entities.length > 0 && value.trim()) {
        e.preventDefault();
        setLookupLoading(true);
        try {
          const result = await onLookup({
            text: value,
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
    },
    [onLookup, entities, value, sourceLocale, targetLocale],
  );

  return (
    <div className="flex flex-col gap-1">
      {/* Search bar row */}
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground" />
          <input
            ref={inputRef}
            type="text"
            value={value}
            onChange={handleTextChange}
            onMouseUp={handleTextSelect}
            onKeyUp={handleTextSelect}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
            className="w-full rounded-md border border-input bg-transparent pl-8 pr-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring"
          />

          {/* Entity popover */}
          {showEntityPopover && selectionRange && (
            <div className="absolute z-50 top-full left-0 mt-1 rounded-md border bg-popover shadow-lg p-1.5 flex flex-wrap gap-1">
              <span className="text-[10px] text-muted-foreground px-1 py-0.5 w-full">
                Mark &ldquo;{selectionRange.text}&rdquo; as:
              </span>
              {ENTITY_TYPES.map((et) => (
                <button
                  key={et.value}
                  onClick={() => markEntity(et.value)}
                  className="text-[10px] px-2 py-0.5 rounded border border-border hover:bg-accent hover:border-primary/30 transition-colors"
                >
                  {et.label}
                </button>
              ))}
            </div>
          )}
        </div>
        {actions}
      </div>

      {/* Entity chips */}
      {entities.length > 0 && (
        <div className="flex flex-wrap items-center gap-1">
          {entities.map((e) => {
            const label = ENTITY_TYPES.find((t) => t.value === e.type)?.label ?? e.type;
            return (
              <span
                key={e.id}
                className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border border-border bg-muted/50"
              >
                <span className="text-muted-foreground uppercase tracking-wider text-[9px]">
                  {label}
                </span>
                <span>{e.text}</span>
                <button
                  onClick={() => removeEntity(e.id)}
                  className="text-muted-foreground hover:text-destructive transition-colors"
                >
                  <X className="size-2.5" />
                </button>
              </span>
            );
          })}
          {onLookup && (
            <span className="text-[10px] text-muted-foreground">
              {lookupLoading ? "Looking up..." : "Press Enter to lookup"}
            </span>
          )}
        </div>
      )}

      {/* Lookup results (collapsible) */}
      {matches.length > 0 && (
        <div className="rounded-md border border-border bg-card/50">
          <button
            className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-muted-foreground hover:text-foreground transition-colors"
            onClick={() => setShowMatches(!showMatches)}
          >
            {showMatches ? <ChevronUp className="size-3" /> : <ChevronDown className="size-3" />}
            {matches.length} {matches.length === 1 ? "match" : "matches"} found
            {!showMatches && (
              <span className="text-[10px] ml-auto">
                Best: {Math.round(matches[0].score * 100)}%
              </span>
            )}
          </button>
          {showMatches && (
            <div className="flex flex-col gap-1.5 px-3 pb-2">
              {matches.map((m, i) => (
                <div key={i} className="rounded border border-border/50 p-2">
                  <MatchScoreBar score={m.score} matchType={m.match_type} className="mb-1.5" />
                  <div className="flex items-start gap-2 mb-0.5">
                    <CodedTextDisplay
                      text={m.entry.source_text}
                      codedText={m.entry.source_coded}
                      spans={m.entry.source_spans}
                      className="text-[12px] text-foreground flex-1"
                    />
                    <LocalePill locale={m.entry.source_locale} />
                  </div>
                  <div className="flex items-start gap-2">
                    <CodedTextDisplay
                      text={m.entry.target_text}
                      codedText={m.entry.target_coded}
                      spans={m.entry.target_spans}
                      className="text-[12px] text-muted-foreground flex-1"
                    />
                    <LocalePill locale={m.entry.target_locale} />
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
