import type { SpanInfo } from "../../types/span";
import { parseCodedSegments } from "../editor/codedText";
import { TagChipComponent } from "../editor/TagChipComponent";

interface CodedTextDisplayProps {
  /** Plain text (used when no coded text is available). */
  text: string;
  /** Coded text with Unicode markers. If empty/undefined, falls back to text. */
  codedText?: string;
  /** Span metadata for inline codes. */
  spans?: SpanInfo[];
  /** Additional CSS class. */
  className?: string;
}

/**
 * Renders text with inline codes as tag chips.
 * Falls back to plain text when no spans are present.
 */
export function CodedTextDisplay({ text, codedText, spans, className }: CodedTextDisplayProps) {
  if (!codedText || !spans || spans.length === 0) {
    return <span className={className}>{text}</span>;
  }

  const segments = parseCodedSegments(codedText, spans);

  return (
    <span className={className}>
      {segments.map((seg, i) =>
        seg.type === "text" ? (
          <span key={i}>{seg.value}</span>
        ) : (
          <TagChipComponent key={i} spanInfo={seg.spanInfo} index={i + 1} />
        ),
      )}
    </span>
  );
}
