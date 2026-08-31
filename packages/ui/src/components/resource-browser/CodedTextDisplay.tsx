import type { Run } from "@neokapi/kapi-format";

import { DirectionalText } from "../../lib/text-direction";
import { runsToSegments } from "../editor/codedText";
import { TagChipComponent } from "../editor/TagChipComponent";

interface CodedTextDisplayProps {
  /** Inline content as an RFC 0001 Run sequence. */
  runs?: Run[];
  /** Plain-text fallback used when `runs` is empty/absent. */
  text?: string;
  /** Additional CSS class. */
  className?: string;
  /** The locale this text is written in — every caller should pass it. */
  locale?: string;
}

/**
 * Renders an inline Run sequence as text interleaved with tag chips.
 * Text runs render as plain text; ph / pcOpen / pcClose / sub runs
 * render as inline code chips. Falls back to plain `text` when no
 * runs are present.
 */
export function CodedTextDisplay({ runs, text, className, locale }: CodedTextDisplayProps) {
  if (!runs || runs.length === 0) {
    return (
      <DirectionalText locale={locale} className={className}>
        {text ?? ""}
      </DirectionalText>
    );
  }

  const segments = runsToSegments(runs);

  return (
    <DirectionalText locale={locale} className={className}>
      {segments.map((seg, i) =>
        seg.type === "text" ? (
          <span key={i}>{seg.value}</span>
        ) : (
          <TagChipComponent key={i} spanInfo={seg.spanInfo} index={i + 1} />
        ),
      )}
    </DirectionalText>
  );
}
