import { useState, useMemo } from "react";
import type { SpanInfo, EntityInfo } from "../../types/api";
import { parseCodedSegments } from "./codedText";
import { TagChipComponent } from "./TagChipComponent";
import { buildPairs } from "./tagSemantics";

/** Color config per entity type — mirrors HighlightedSource. */
const entityColors: Record<string, { bg: string; border: string }> = {
  "entity:person": { bg: "var(--entity-person-bg)", border: "var(--entity-person-border)" },
  "entity:organization": { bg: "var(--entity-org-bg)", border: "var(--entity-org-border)" },
  "entity:location": { bg: "var(--entity-location-bg)", border: "var(--entity-location-border)" },
  "entity:date": { bg: "var(--entity-date-bg)", border: "var(--entity-date-border)" },
  "entity:product": { bg: "var(--entity-product-bg)", border: "var(--entity-product-border)" },
};

function getEntityColors(entityType: string) {
  return entityColors[entityType] ?? { bg: "var(--entity-default-bg)", border: "var(--entity-default-border)" };
}

interface SourceCellDisplayProps {
  codedText: string;
  spans: SpanInfo[];
  entities?: EntityInfo[];
}

/**
 * Read-only display of source text with inline tag chips and optional entity underlines.
 * Entity ranges are defined on the plain source text; we track running plaintext offset
 * to apply entity styling to text segments in the coded view.
 */
export function SourceCellDisplay({ codedText, spans, entities = [] }: SourceCellDisplayProps) {
  const segments = parseCodedSegments(codedText, spans);
  const pairs = useMemo(() => buildPairs(spans), [spans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);

  // Build a set of sorted entity ranges for quick lookup.
  const sortedEntities = useMemo(
    () => [...entities].filter(e => e.start >= 0 && e.end > e.start).sort((a, b) => a.start - b.start),
    [entities],
  );

  let tagIndex = 0;
  let plainOffset = 0; // Running offset in the plain source text.

  return (
    <span>
      {segments.map((seg, i) => {
        if (seg.type === "text") {
          const segStart = plainOffset;
          const segEnd = segStart + seg.value.length;
          plainOffset = segEnd;

          // Check if any entity overlaps this text segment.
          if (sortedEntities.length === 0) {
            return <span key={i}>{seg.value}</span>;
          }

          // Split text by entity ranges.
          const parts: React.ReactNode[] = [];
          let cursor = segStart;

          for (const e of sortedEntities) {
            if (e.start >= segEnd || e.end <= segStart) continue;
            const overlapStart = Math.max(e.start, segStart);
            const overlapEnd = Math.min(e.end, segEnd);

            // Pre-entity text.
            if (overlapStart > cursor) {
              parts.push(<span key={`t-${cursor}`}>{seg.value.slice(cursor - segStart, overlapStart - segStart)}</span>);
            }

            // Entity-highlighted text.
            const colors = getEntityColors(e.type);
            parts.push(
              <span
                key={`e-${overlapStart}`}
                className="rounded-sm px-px"
                style={{ backgroundColor: colors.bg, borderBottom: `2px solid ${colors.border}` }}
                title={`${e.type.replace("entity:", "")}${e.dnt ? " (DNT)" : ""}`}
              >
                {seg.value.slice(overlapStart - segStart, overlapEnd - segStart)}
              </span>,
            );

            cursor = overlapEnd;
          }

          // Trailing text.
          if (cursor < segEnd) {
            parts.push(<span key={`t-${cursor}`}>{seg.value.slice(cursor - segStart)}</span>);
          }

          return parts.length > 0 ? <span key={i}>{parts}</span> : <span key={i}>{seg.value}</span>;
        }

        // Tag segment — no entity styling, just tag chips.
        const currentTagIndex = tagIndex;
        tagIndex++;
        const pairInfo = pairs.get(currentTagIndex);
        const pairIdx = pairInfo?.pairIndex;

        return (
          <span
            key={i}
            onMouseEnter={() => pairIdx != null && setHoveredPairIndex(pairIdx)}
            onMouseLeave={() => setHoveredPairIndex(null)}
          >
            <TagChipComponent
              spanInfo={seg.spanInfo}
              index={currentTagIndex + 1}
              pairIndex={pairIdx}
              highlighted={hoveredPairIndex != null && pairIdx === hoveredPairIndex}
            />
          </span>
        );
      })}
    </span>
  );
}
