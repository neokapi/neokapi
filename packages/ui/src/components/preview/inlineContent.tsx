// inlineContent — the two things every reading of a block's inline content has
// to draw, and the weave that puts them in the right order.
//
// A run sequence flattens into text plus the codes positioned in it (see
// `runsText` / `runsCodes`, both declared projections of the same sequence), and
// a surface layers overlay marks over that text. Text, codes and marks are three
// segmentations of one string, and reading them back in order is fiddly enough
// that the document preview and the keyed table would drift if each wrote it:
// a code inside an overlay splits the mark in two around the chip, and a code
// at the very end of a line lands after the last character.

import React from "react";
import { cn } from "../../lib/utils";
import { isLineBreakCode, semanticLabel, semanticTooltip, tagColors } from "../editor/tagSemantics";
import type { InlineCode } from "./renderDoc";
import type { TextSegment } from "./overlayHighlight";
import styles from "./FormatPreview.module.css";

/**
 * One inline code as a chip: the vocabulary's short label in its type color,
 * with the original markup on the tooltip. Deliberately the same pill the
 * editor's cell renderers draw, so a placeholder reads identically wherever it
 * is shown: a source that quietly lost its `{{name}}` reads as a translation
 * that invented one.
 *
 * A break reads as a break. Its chip carries the mark so the code is still
 * visible and hoverable, and the line ends after it. Drawn as a chip alone it
 * left two lines running together, which is how a review pane came to show
 * "Première ligneDeuxième ligne".
 */
export function InlineCodeChip({ code }: { code: InlineCode }): React.ReactElement {
  const colors = tagColors(code.span);
  const isBreak = isLineBreakCode(code.span);
  const pill = (
    <span
      className={styles.inlineCode}
      style={{ backgroundColor: colors.bg, borderColor: colors.border, color: colors.text }}
      title={semanticTooltip(code.span)}
      data-inline-code={code.span.span_type}
      {...(isBreak ? { "data-line-break": "true" } : {})}
      dir="ltr"
    >
      {semanticLabel(code.span)}
    </span>
  );
  if (!isBreak) return pill;
  return (
    <>
      {pill}
      <br />
    </>
  );
}

/**
 * One overlay-marked stretch of text, with its reason on the tooltip. The
 * document preview draws a richer mark (a censor bar for redaction, a roll
 * animation for a term); this is the plain one, for a surface with no motion of
 * its own.
 */
export function OverlayText({ segment }: { segment: TextSegment }): React.ReactElement {
  const overlay = segment.overlay;
  if (!overlay) return <>{segment.text}</>;
  return (
    <mark
      className={cn(styles.overlay, overlay.style.className)}
      data-overlay-type={overlay.type}
      title={overlay.tooltip}
    >
      {segment.text}
    </mark>
  );
}

/**
 * Weave a line's inline codes back into its overlay segments, in reading order.
 *
 * The two segmentations are independent: overlays cover ranges of text, codes
 * sit *between* characters, so a code inside an overlay splits that segment into
 * two marks around the chip rather than dropping either. `limit` is how far the
 * text has been revealed (a typewriter's visible prefix), so a code appears only
 * once its position has been reached; pass the full length for a static
 * reading. `renderText` draws one stretch of text, so a caller can wrap it in
 * its own mark or word diff.
 */
export function weaveInline(
  segments: TextSegment[],
  codes: InlineCode[],
  limit: number,
  renderText: (segment: TextSegment, text: string, key: string) => React.ReactNode,
): React.ReactNode[] {
  const nodes: React.ReactNode[] = [];
  let ci = 0;
  let pos = 0;

  const emitCodesAt = (offset: number) => {
    while (ci < codes.length && codes[ci].offset <= offset) {
      nodes.push(<InlineCodeChip key={`c${ci}`} code={codes[ci]} />);
      ci++;
    }
  };
  const emitText = (seg: TextSegment, text: string) => {
    if (!text) return;
    nodes.push(renderText(seg, text, `t${nodes.length}`));
  };

  for (const seg of segments) {
    const start = pos;
    const end = pos + seg.text.length;
    let cursor = start;
    while (ci < codes.length && codes[ci].offset < end) {
      const at = Math.max(codes[ci].offset, start);
      emitText(seg, seg.text.slice(cursor - start, at - start));
      cursor = at;
      emitCodesAt(at);
    }
    emitText(seg, seg.text.slice(cursor - start));
    pos = end;
  }
  // Codes trailing the last character (a closing tag at the end of a line).
  emitCodesAt(limit);
  return nodes;
}
