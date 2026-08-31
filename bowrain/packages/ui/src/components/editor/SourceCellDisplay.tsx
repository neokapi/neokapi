import { useState, useMemo } from "react";
import type { SpanInfo, EntityInfo } from "../../types/api";
import {
  DirectionalText,
  parseCodedSegments,
  TagChipComponent,
  buildPairs,
} from "@neokapi/ui-primitives";
import { entityMarks, markEntities } from "./entityMarks";

interface SourceCellDisplayProps {
  codedText: string;
  spans: SpanInfo[];
  entities?: EntityInfo[];
  /** The locale this text is written in — see FormattedSourceDisplay's `locale`. */
  locale?: string;
}

/**
 * Read-only display of source text with inline tag chips and optional entity underlines.
 * Entity ranges are defined on the plain source text; we track running plaintext offset
 * to apply entity styling to text segments in the coded view.
 */
export function SourceCellDisplay({ codedText, spans, entities, locale }: SourceCellDisplayProps) {
  const segments = useMemo(() => parseCodedSegments(codedText, spans), [codedText, spans]);
  const pairs = useMemo(() => buildPairs(spans), [spans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);

  // Entity positions are stated over the plain text — the text segments
  // concatenated, the inline codes contributing nothing.
  const marks = useMemo(
    () =>
      entityMarks(
        segments.reduce((text, seg) => (seg.type === "text" ? text + seg.value : text), ""),
        entities,
      ),
    [segments, entities],
  );

  let tagIndex = 0;
  let plainOffset = 0; // Running code-point offset in the plain source text.

  return (
    <DirectionalText locale={locale}>
      {segments.map((seg, i) => {
        if (seg.type === "text") {
          const offset = plainOffset;
          plainOffset += Array.from(seg.value).length;
          return <span key={i}>{markEntities(seg.value, offset, marks)}</span>;
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
    </DirectionalText>
  );
}
