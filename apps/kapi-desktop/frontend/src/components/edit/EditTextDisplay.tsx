import { DirectionalText, TagChipComponent, editTextToSegments } from "@neokapi/ui-primitives";
import type { CodeRead } from "@neokapi/contract-types";

export interface EditTextDisplayProps {
  /** Content in the placeholder form a read shows. */
  text: string;
  /** The read's codes, which type and label each chip. */
  codes?: Readonly<Record<string, CodeRead>>;
  /** The language the text is written in. */
  locale?: string;
  className?: string;
  "data-slot"?: string;
}

/**
 * Content as a change-service read shows it, drawn with each inline code as a
 * chip: the words a person reads, and the codes in the places they hold.
 */
export function EditTextDisplay({
  text,
  codes,
  locale,
  className,
  "data-slot": dataSlot,
}: EditTextDisplayProps) {
  const segments = editTextToSegments(text, codes ?? {});
  return (
    <DirectionalText
      locale={locale}
      className={className ?? "whitespace-pre-wrap text-sm"}
      data-slot={dataSlot}
      translate="no"
    >
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
