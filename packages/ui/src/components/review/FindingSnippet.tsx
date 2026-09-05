import * as React from "react";
import { findingToneMarkClass, type FindingTone } from "../../lib/finding-severity";
import { DirectionalText } from "../../lib/text-direction";
import { cn } from "../../lib/utils";
import { resolveHighlight } from "../preview/highlights";
import { InlineCodeChip, PlainMark, codeMarkFor, weaveInline } from "../preview/inlineContent";
import { segmentText } from "../preview/overlayHighlight";
import { runsCodes, runsText } from "../preview/renderDoc";
import type { Anchor, Run } from "../preview/types";
import styles from "../preview/FormatPreview.module.css";

/**
 * A finding in the text it was raised on: the block's runs read as the document
 * reads them, with the finding's span marked in its tone.
 *
 * A finding card that quotes the offending words alone leaves the reader to
 * find them; this shows the words where they stand, with the placeholders and
 * paired codes around them drawn as chips rather than dropped, through the same
 * declared projection the document preview reads a line with (`runsText` and
 * `runsCodes` over renderDoc's DOCUMENT_PIECES). The span comes from the
 * finding's run anchor by the same arithmetic the preview marks it with, so the
 * card and the opened document agree about which words are meant.
 *
 * Kapi Desktop's Checks page and the platform's problems panel both read
 * through this, so the same finding reads the same on either surface.
 */
export interface FindingSnippetProps {
  /** The block's runs on the side the finding is about. */
  runs?: Run[];
  /** The locale those runs are written in, for direction and language. */
  locale?: string;
  /**
   * Where in `runs` the finding sits. Absent, the text is shown unmarked; a
   * block anchor marks all of it, and a run anchor marks the code's chip.
   */
  anchor?: Anchor;
  /** How hard the finding bites, on the shared scale; it paints the mark. */
  tone: FindingTone;
  /** What the mark says on hover: the finding's message. */
  label?: string;
  /**
   * The offending text as the checker quoted it, shown marked on its own when
   * the block's runs are unavailable.
   */
  fallbackText?: string;
  className?: string;
  "data-testid"?: string;
}

/** The side name a lone snippet resolves its highlight under. */
const OWN_SIDE = "snippet";

export function FindingSnippet({
  runs,
  locale,
  anchor,
  tone,
  label,
  fallbackText,
  className,
  "data-testid": testID,
}: FindingSnippetProps): React.ReactElement | null {
  const hasRuns = !!runs && runs.length > 0;
  const text = React.useMemo(() => (hasRuns ? runsText(runs) : ""), [hasRuns, runs]);
  const codes = React.useMemo(() => (hasRuns ? runsCodes(runs) : []), [hasRuns, runs]);
  const marks = React.useMemo(() => {
    if (!hasRuns || !anchor) return [];
    const mark = resolveHighlight(runs, { side: OWN_SIDE, anchor, tone, label: label ?? "" });
    return mark ? [mark] : [];
  }, [hasRuns, runs, anchor, tone, label]);
  const nodes = React.useMemo(() => {
    if (!hasRuns) return null;
    return weaveInline(
      segmentText(text, marks),
      codes,
      text.length,
      (seg, value, key) =>
        seg.overlay ? (
          <PlainMark key={key} overlay={seg.overlay}>
            {value}
          </PlainMark>
        ) : (
          <React.Fragment key={key}>{value}</React.Fragment>
        ),
      (code, key) => {
        const mark = codeMarkFor(marks, code);
        const chip = <InlineCodeChip code={code} />;
        return mark ? (
          <PlainMark key={key} overlay={mark}>
            {chip}
          </PlainMark>
        ) : (
          <React.Fragment key={key}>{chip}</React.Fragment>
        );
      },
    );
  }, [hasRuns, text, marks, codes]);

  // The quote alone is the whole span, so the mark is the element that carries
  // its direction and language too.
  if (!hasRuns) {
    if (!fallbackText) return null;
    return (
      <DirectionalText
        as="mark"
        locale={locale}
        translate="no"
        className={cn(styles.overlay, findingToneMarkClass(tone), "whitespace-pre-wrap", className)}
        data-overlay-type="finding"
        data-snippet="fallback"
        data-testid={testID}
        title={label}
      >
        {fallbackText}
      </DirectionalText>
    );
  }

  return (
    <DirectionalText
      as="span"
      locale={locale}
      translate="no"
      className={cn("whitespace-pre-wrap", className)}
      data-snippet="runs"
      data-testid={testID}
    >
      {nodes}
    </DirectionalText>
  );
}
