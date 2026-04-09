import { useState, useCallback, useRef } from "react";
import type { EntityAnnotationDTO, TMMatchDTO, LookupTMRequest } from "./types";
import { ENTITY_TYPES } from "./types";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { MatchScoreBar } from "./MatchScoreBar";
import { LocalePill } from "./LocalePill";
import { Input } from "../ui/input";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from "../ui/collapsible";
import { Search, ChevronDown, X } from "lucide-react";

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
      if (entities.length > 0) {
        const valid = entities.filter((ent) => {
          if (ent.end > newText.length) return false;
          return newText.substring(ent.start, ent.end) === ent.text;
        });
        if (valid.length !== entities.length) setEntities(valid);
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
    setSelectionRange({ start, end, text: value.substring(start, end) });
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
        [...prev.filter((e) => e.end <= newEntity.start || e.start >= newEntity.end), newEntity].sort(
          (a, b) => a.start - b.start,
        ),
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
          <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 size-3.5 text-muted-foreground pointer-events-none" />
          <Input
            ref={inputRef}
            value={value}
            onChange={handleTextChange}
            onMouseUp={handleTextSelect}
            onKeyUp={handleTextSelect}
            onKeyDown={handleKeyDown}
            placeholder={placeholder}
            className="pl-8"
          />

          {/* Entity popover */}
          {showEntityPopover && selectionRange && (
            <div className="absolute z-50 top-full left-0 mt-1 rounded-lg border bg-popover shadow-lg p-1.5 flex flex-wrap gap-1">
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
        {actions}
      </div>

      {/* Entity chips */}
      {entities.length > 0 && (
        <div className="flex flex-wrap items-center gap-1">
          {entities.map((e) => {
            const label = ENTITY_TYPES.find((t) => t.value === e.type)?.label ?? e.type;
            return (
              <Badge key={e.id} variant="outline" className="gap-1 pr-1">
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
              </Badge>
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
        <Collapsible open={showMatches} onOpenChange={setShowMatches}>
          <CollapsibleTrigger className="flex items-center gap-2 w-full px-3 py-1.5 text-[11px] text-muted-foreground hover:text-foreground transition-colors rounded-md border bg-card/50">
            <ChevronDown className="size-3 transition-transform data-[state=open]:rotate-180" />
            {matches.length} {matches.length === 1 ? "match" : "matches"} found
            {!showMatches && (
              <span className="text-[10px] ml-auto">
                Best: {Math.round(matches[0].score * 100)}%
              </span>
            )}
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="flex flex-col gap-1.5 px-3 pb-2 pt-1">
              {matches.map((m, i) => (
                <div key={i} className="rounded-lg border p-2">
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
          </CollapsibleContent>
        </Collapsible>
      )}
    </div>
  );
}
