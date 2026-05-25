import type { Run } from "@neokapi/kapi-format";

import { runsToSegments } from "../editor/codedText";
import { TagChipComponent } from "../editor/TagChipComponent";

interface CodedTextDisplayProps {
  /** Inline content as an RFC 0001 Run sequence. */
  runs?: Run[];
  /** Plain-text fallback used when `runs` is empty/absent. */
  text?: string;
  /** Additional CSS class. */
  className?: string;
}

/**
 * Renders an inline Run sequence as text interleaved with tag chips.
 * Text runs render as plain text; ph / pcOpen / pcClose / sub runs
 * render as inline code chips. Falls back to plain `text` when no
 * runs are present.
 */
export function CodedTextDisplay({ runs, text, className }: CodedTextDisplayProps) {
  if (!runs || runs.length === 0) {
    return <span className={className}>{text ?? ""}</span>;
  }

  const segments = runsToSegments(runs);

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
