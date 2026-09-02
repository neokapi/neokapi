import { useCallback, useMemo, useState } from "react";
import {
  byteToCharOffset,
  rangeAnchorForChars,
  segmentText,
  type ResolvedSpan,
} from "@neokapi/ui-primitives/preview";
import type { BlockTermMatch, EntityInfo } from "../../types/api";
import { EntityPopover } from "./EntityPopover";
import { entityLabel, entityTypeColors } from "./entityMarks";

/**
 * Plain source text with its terminology hits and entity annotations marked.
 *
 * The flattening is the preview kit's `segmentText`: overlapping marks are cut
 * into non-overlapping segments with the innermost one winning, so a term
 * inside an entity still shows. The accents stay bowrain's, because an entity
 * is coloured by its type from the app theme's entity tokens and a term is
 * underlined rather than filled, neither of which the kit's palette expresses.
 *
 * Offsets arrive as UTF-8 byte offsets, the way the server reports them from
 * `model.Anchor.ByteSpan`, and are resolved to code points before anything is
 * sliced.
 */

/** What a mark is: the two kinds this surface knows how to draw. */
type MarkKind = "term" | "entity";

interface MarkedSourceProps {
  text: string;
  termMatches: BlockTermMatch[];
  entities?: EntityInfo[];
  onEntityUpdate?: (entity: EntityInfo) => void;
  onEntityDelete?: (entityKey: string) => void;
  onEntityPromote?: (entityKey: string) => void;
}

const TERM_CLASS =
  "underline decoration-dotted decoration-orange-600 underline-offset-2 cursor-help";

/** The tooltip for a terminology hit: what it matched and what to say instead. */
function termTooltip(term: BlockTermMatch): string {
  return `${term.source_term} → ${term.target_terms?.join(", ") || "?"} (${term.status})`;
}

/** The tooltip for an entity: its type, and whether it is marked do-not-translate. */
function entityTooltip(entity: EntityInfo): string {
  return `${entityLabel(entity.type)}${entity.dnt ? " (DNT)" : ""}`;
}

/**
 * Term hits and entities as resolved spans over `text`, in the kit's shape.
 * A mark whose byte range does not locate inside the text is dropped.
 */
function marksOf(
  text: string,
  termMatches: BlockTermMatch[],
  entities: EntityInfo[],
): { spans: ResolvedSpan[]; entityOf: Map<ResolvedSpan, EntityInfo> } {
  const runs = [{ text }];
  const spans: ResolvedSpan[] = [];
  const entityOf = new Map<ResolvedSpan, EntityInfo>();

  const add = (kind: MarkKind, from: number, to: number, className: string, tooltip: string) => {
    const start = byteToCharOffset(text, from);
    const end = byteToCharOffset(text, to);
    if (end <= start) return undefined;
    const span: ResolvedSpan = {
      start,
      end,
      type: kind,
      style: { className, label: kind === "term" ? "Term" : "Entity" },
      span: { range: rangeAnchorForChars(runs, start, end) },
      tooltip,
    };
    spans.push(span);
    return span;
  };

  for (const term of termMatches) {
    if (term.start < 0 || term.end <= term.start) continue;
    add("term", term.start, term.end, TERM_CLASS, termTooltip(term));
  }
  for (const entity of entities) {
    if (entity.start < 0 || entity.end <= entity.start) continue;
    const span = add("entity", entity.start, entity.end, "", entityTooltip(entity));
    if (span) entityOf.set(span, entity);
  }
  return { spans, entityOf };
}

/** Source text with its terminology hits and entities marked. */
export function MarkedSource({
  text,
  termMatches,
  entities = [],
  onEntityUpdate,
  onEntityDelete,
  onEntityPromote,
}: MarkedSourceProps) {
  const [activePopover, setActivePopover] = useState<string | null>(null);

  const handleEntityClick = useCallback((key: string) => {
    setActivePopover((prev) => (prev === key ? null : key));
  }, []);

  const { spans, entityOf } = useMemo(
    () => marksOf(text, termMatches, entities),
    [text, termMatches, entities],
  );
  const segments = useMemo(() => segmentText(text, spans), [text, spans]);

  if (spans.length === 0) return <>{text}</>;

  // An entity cut in two by a nested term yields several segments; its popover
  // is anchored to the first of them so one click opens one popover.
  const anchored = new Set<string>();

  return (
    <>
      {segments.map((segment, i) => {
        const mark = segment.overlay;
        if (!mark) return <span key={i}>{segment.text}</span>;

        if (mark.type === "term") {
          return (
            <span key={i} className={mark.style.className} title={mark.tooltip}>
              {segment.text}
            </span>
          );
        }

        const entity = entityOf.get(mark);
        if (!entity) return <span key={i}>{segment.text}</span>;
        const colors = entityTypeColors(entity.type);
        const showPopover = activePopover === entity.key && !anchored.has(entity.key);
        anchored.add(entity.key);

        return (
          <span key={i} className="relative inline">
            <span
              className="cursor-pointer rounded-sm px-px"
              style={{ backgroundColor: colors.bg, borderBottom: `2px solid ${colors.border}` }}
              title={mark.tooltip}
              data-entity-key={entity.key}
              onClick={() => handleEntityClick(entity.key)}
            >
              {segment.text}
            </span>
            {showPopover && (
              <EntityPopover
                entity={entity}
                onClose={() => setActivePopover(null)}
                onUpdate={onEntityUpdate}
                onDelete={onEntityDelete}
                onPromote={onEntityPromote}
              />
            )}
          </span>
        );
      })}
    </>
  );
}
