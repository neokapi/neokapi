import type { SpanInfo } from "../../types/api";
import { parseCodedSegments } from "./codedText";
import { TagChipComponent } from "./TagChipComponent";

interface SourceCellDisplayProps {
  codedText: string;
  spans: SpanInfo[];
}

/**
 * Read-only display of source text with inline tag chips.
 * Uses lightweight parsing — no Lexical overhead.
 */
export function SourceCellDisplay({ codedText, spans }: SourceCellDisplayProps) {
  const segments = parseCodedSegments(codedText, spans);
  let tagIndex = 0;

  return (
    <span>
      {segments.map((seg, i) => {
        if (seg.type === "text") {
          return <span key={i}>{seg.value}</span>;
        }
        tagIndex++;
        return (
          <TagChipComponent
            key={i}
            spanInfo={seg.spanInfo}
            index={tagIndex}
          />
        );
      })}
    </span>
  );
}
