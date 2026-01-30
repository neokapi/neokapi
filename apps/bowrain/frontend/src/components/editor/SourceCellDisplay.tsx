import { useState, useMemo } from "react";
import type { SpanInfo } from "../../types/api";
import { parseCodedSegments } from "./codedText";
import { TagChipComponent } from "./TagChipComponent";
import { buildPairs } from "./tagSemantics";

interface SourceCellDisplayProps {
  codedText: string;
  spans: SpanInfo[];
}

/**
 * Read-only display of source text with inline tag chips.
 * Uses lightweight parsing — no Lexical overhead.
 * Supports pair-aware hover highlighting.
 */
export function SourceCellDisplay({ codedText, spans }: SourceCellDisplayProps) {
  const segments = parseCodedSegments(codedText, spans);
  const pairs = useMemo(() => buildPairs(spans), [spans]);
  const [hoveredPairIndex, setHoveredPairIndex] = useState<number | null>(null);

  let tagIndex = 0;

  return (
    <span>
      {segments.map((seg, i) => {
        if (seg.type === "text") {
          return <span key={i}>{seg.value}</span>;
        }
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
