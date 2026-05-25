import { useState, useCallback, useRef } from "react";
import type {
  EntityAdaptationDTO,
  EntityAnnotationDTO,
  TMMatchDTO,
  LookupTMRequest,
} from "./types";
import { ENTITY_TYPES } from "./types";
import { CodedTextDisplay } from "./CodedTextDisplay";
import { MatchScoreBar } from "./MatchScoreBar";
import { LocalePill } from "./LocalePill";

interface TMLookupPanelProps {
  sourceLocale: string;
  targetLocale: string;
  onLookup: (req: LookupTMRequest) => Promise<TMMatchDTO[]>;
}

interface MarkedEntity extends EntityAnnotationDTO {
  id: string;
}

/**
 * Entity-aware TM lookup panel.
 * Users type text, select portions to mark as entities, then lookup.
 * Results show match scores and entity adaptation indicators.
 */
export function TMLookupPanel({ sourceLocale, targetLocale, onLookup }: TMLookupPanelProps) {
  const [text, setText] = useState("");
  const [entities, setEntities] = useState<MarkedEntity[]>([]);
  const [minScore, setMinScore] = useState(0.7);
  const [matches, setMatches] = useState<TMMatchDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [showEntityPopover, setShowEntityPopover] = useState(false);
  const [selectionRange, setSelectionRange] = useState<{
    start: number;
    end: number;
    text: string;
  } | null>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const entityIdRef = useRef(0);

  const handleTextChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const newText = e.target.value;
      setText(newText);
      // Only clear entities if the text changed substantially (not just appending/minor edit).
      // Clear if length difference is large or if entity positions would be invalid.
      if (entities.length > 0) {
        const valid = entities.filter((ent: MarkedEntity) => {
          if (ent.end > newText.length) return false;
          // Check the entity text still matches at its position.
          return newText.substring(ent.start, ent.end) === ent.text;
        });
        if (valid.length !== entities.length) {
          setEntities(valid);
        }
      }
    },
    [text, entities],
  );

  const handleTextSelect = useCallback(() => {
    const el = inputRef.current;
    if (!el) return;
    const start = el.selectionStart;
    const end = el.selectionEnd;
    if (start === end) {
      setShowEntityPopover(false);
      return;
    }
    const selectedText = text.substring(start, end);
    setSelectionRange({ start, end, text: selectedText });
    setShowEntityPopover(true);
  }, [text]);

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
      setEntities((prev: MarkedEntity[]) =>
        [
          ...prev.filter((e: MarkedEntity) => e.end <= newEntity.start || e.start >= newEntity.end),
          newEntity,
        ].sort((a, b) => a.start - b.start),
      );
      setShowEntityPopover(false);
    },
    [selectionRange],
  );

  const removeEntity = useCallback((id: string) => {
    setEntities((prev: MarkedEntity[]) => prev.filter((e: MarkedEntity) => e.id !== id));
  }, []);

  const [error, setError] = useState<string | null>(null);

  const handleLookup = useCallback(async () => {
    if (!text.trim()) return;
    setLoading(true);
    setError(null);
    try {
      const result = await onLookup({
        text,
        entities: entities.map(({ text, type, start, end }: MarkedEntity) => ({
          text,
          type,
          start,
          end,
        })),
        source_locale: sourceLocale,
        target_locale: targetLocale,
        min_score: minScore,
        max_results: 10,
      });
      setMatches(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [text, entities, sourceLocale, targetLocale, minScore, onLookup]);

  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-[13px] font-semibold text-foreground">Lookup</h3>

      {/* Input area */}
      <div className="relative">
        <textarea
          ref={inputRef}
          value={text}
          onChange={handleTextChange}
          onMouseUp={handleTextSelect}
          onKeyUp={handleTextSelect}
          placeholder="Enter text to look up..."
          rows={2}
          className="w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm outline-none focus:ring-1 focus:ring-ring resize-none"
        />

        {/* Entity popover — positioned below the textarea near the selection */}
        {showEntityPopover && selectionRange && (
          <div className="absolute z-50 top-full left-0 mt-1 max-w-full rounded-md border border-border bg-popover shadow-lg p-1.5 flex flex-wrap gap-1">
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

      {/* Marked entities */}
      {entities.length > 0 && (
        <div className="flex flex-wrap gap-1">
          {entities.map((e: MarkedEntity) => {
            const label = ENTITY_TYPES.find((t) => t.value === e.type)?.label ?? e.type;
            return (
              <span
                key={e.id}
                className="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border border-border"
              >
                <span className="text-muted-foreground uppercase tracking-wider text-[9px]">
                  {label}
                </span>
                <span>{e.text}</span>
                <button
                  onClick={() => removeEntity(e.id)}
                  className="text-muted-foreground hover:text-destructive transition-colors ml-0.5"
                  aria-label={`Remove ${e.text} entity`}
                >
                  x
                </button>
              </span>
            );
          })}
        </div>
      )}

      {/* Controls */}
      <div className="flex items-center gap-2">
        <button
          onClick={handleLookup}
          disabled={loading || !text.trim()}
          className="rounded-md bg-primary px-3 py-1.5 text-xs font-medium text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        >
          {loading ? "Looking up..." : "Lookup"}
        </button>
        <div className="flex items-center gap-1.5 text-[11px] text-muted-foreground">
          <label htmlFor="min-score">Min:</label>
          <input
            id="min-score"
            type="range"
            min={0}
            max={100}
            value={Math.round(minScore * 100)}
            onChange={(e) => setMinScore(Number(e.target.value) / 100)}
            className="w-16 h-1 accent-primary"
          />
          <span className="tabular-nums w-8">{Math.round(minScore * 100)}%</span>
        </div>
      </div>

      {/* Error */}
      {error && <p className="text-xs text-destructive">{error}</p>}

      {/* Results */}
      {matches.length > 0 && (
        <div className="flex flex-col gap-2 mt-1">
          <div className="text-[11px] text-muted-foreground">
            {matches.length} {matches.length === 1 ? "match" : "matches"}
          </div>
          {matches.map((m: TMMatchDTO, i: number) => {
            const srcKey = m.entry.hint_src_lang || sourceLocale;
            const src = srcKey ? m.entry.variants[srcKey] : undefined;
            const tgt = targetLocale ? m.entry.variants[targetLocale] : undefined;
            return (
              <div key={i} className="rounded-md border border-border p-3 bg-card">
                <MatchScoreBar score={m.score} matchType={m.match_type} className="mb-2" />
                {src && (
                  <div className="flex items-start gap-2 mb-1">
                    <span className="text-[10px] text-muted-foreground w-6 shrink-0 pt-0.5">
                      src
                    </span>
                    <CodedTextDisplay
                      text={src.text}
                      runs={src.runs}
                      className="text-[12px] text-foreground flex-1"
                    />
                    <LocalePill locale={src.locale} />
                  </div>
                )}
                {tgt && (
                  <div className="flex items-start gap-2">
                    <span className="text-[10px] text-muted-foreground w-6 shrink-0 pt-0.5">
                      tgt
                    </span>
                    <CodedTextDisplay
                      text={tgt.text}
                      runs={tgt.runs}
                      className="text-[12px] text-muted-foreground flex-1"
                    />
                    <LocalePill locale={tgt.locale} />
                  </div>
                )}

                {/* Entity adaptations */}
                {m.entity_adaptations && m.entity_adaptations.length > 0 && (
                  <div className="mt-2 pt-2 border-t border-border/50">
                    <div className="text-[10px] text-muted-foreground mb-1">Adaptations</div>
                    {m.entity_adaptations.map((ea: EntityAdaptationDTO, j: number) => {
                      const typeLabel =
                        ENTITY_TYPES.find((t) => t.value === ea.type)?.label ??
                        ea.type.replace("entity:", "");
                      return (
                        <div key={j} className="flex items-center gap-1.5 text-[11px]">
                          <span className="text-[9px] uppercase tracking-wider text-muted-foreground font-medium">
                            {typeLabel}
                          </span>
                          <span className="text-foreground">{ea.stored_value}</span>
                          <span className="text-muted-foreground">&rarr;</span>
                          <span className="text-primary font-medium">{ea.current_value}</span>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}
