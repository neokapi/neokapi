import { useState, useMemo } from "react";
import type { Run } from "@neokapi/kapi-format";
import type { EntityInfo } from "../../types/api";
import {
  DirectionalText,
  TagChipComponent,
  buildPairs,
  runsToSegments,
} from "@neokapi/ui-primitives";
import { entityMarks, markEntities } from "./entityMarks";

interface SourceCellDisplayProps {
  /** The content to show, as the runs the server serves it in. */
  runs: readonly Run[];
  entities?: EntityInfo[];
  /** The locale this text is written in — see FormattedSourceDisplay's `locale`. */
  locale?: string;
}

/**
 * Read-only display of a run sequence as text with inline tag chips and
 * optional entity underlines. Entity ranges are defined on the plain text;
 * the running plain-text offset locates them within the text segments.
 */
export function SourceCellDisplay({ runs, entities, locale }: SourceCellDisplayProps) {
  const segments = useMemo(() => runsToSegments(runs), [runs]);
  const pairs = useMemo(
    () => buildPairs(segments.flatMap((seg) => (seg.type === "tag" ? [seg.spanInfo] : []))),
    [segments],
  );
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
