// inlineContent — the two things every reading of a block's inline content has
// to draw, and the weave that puts them in the right order.
//
// A run sequence flattens into text plus the codes positioned in it (see
// `runsText` / `runsCodes`, both declared projections of the same sequence), and
// a surface layers overlay marks over that text. Text, codes and marks are three
// segmentations of one string, and reading them back in order is fiddly enough
// that the document preview and the keyed table would drift if each wrote it:
// a code inside an overlay splits the mark in two around it, and a code at the
// very end of a line lands after the last character.
//
// A code is read one of two ways (see ./inlineStyle): a PRESENTATIONAL code —
// bold, italic, monospace — wraps the text between its open and close in the
// real element, so a `<code>` pair reads as monospace rather than `[CODE]…/code`
// chips around plain text. An OPAQUE code — a placeholder, a link, a break —
// stands for something with no rendered form, so it stays a chip.

import React from "react";
import { cn } from "../../lib/utils";
import {
  chipDisplayLabel,
  isLineBreakCode,
  semanticTooltip,
  tagColors,
} from "../editor/tagSemantics";
import { isPresentationalCode, presentationalTag } from "./inlineStyle";
import type { InlineCode } from "./renderDoc";
import type { ResolvedSpan, TextSegment } from "./overlayHighlight";
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
      {chipDisplayLabel(code.span)}
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
  return <PlainMark overlay={overlay}>{segment.text}</PlainMark>;
}

/**
 * The plain mark around whatever a resolved span covers: a stretch of text, or
 * the chip of an inline code a run anchor names. It carries the span's overlay
 * type and its emphasis as data attributes, so a style or a test can address
 * the one the reader came for apart from the ones drawn beside it.
 */
export function PlainMark({
  overlay,
  children,
}: {
  overlay: ResolvedSpan;
  children: React.ReactNode;
}): React.ReactElement {
  return (
    <mark
      className={cn(styles.overlay, overlay.style.className)}
      data-overlay-type={overlay.type}
      data-emphasis={overlay.emphasis}
      title={overlay.tooltip}
    >
      {children}
    </mark>
  );
}

/**
 * The zero-width span that marks an inline code, if one does: a run anchor on a
 * placeholder or a paired code names the code by id and sits at its offset,
 * because the code has no text of its own for a mark to cover.
 */
export function codeMarkFor(
  marks: readonly ResolvedSpan[] | undefined,
  code: InlineCode,
): ResolvedSpan | undefined {
  if (!marks) return undefined;
  return marks.find(
    (m) =>
      m.start === m.end &&
      m.code !== undefined &&
      m.code === code.span.id &&
      m.start === code.offset,
  );
}

/** One nesting level of the weave: a presentational element being filled, or the root. */
interface Frame {
  /** The element the frame closes into, or null to close into a fragment. */
  tag: keyof React.JSX.IntrinsicElements | null;
  children: React.ReactNode[];
  key: string;
}

/**
 * Weave a line's inline codes back into its overlay segments, in reading order,
 * rendering presentational codes as their real inline element.
 *
 * The two segmentations are independent: overlays cover ranges of text, codes
 * sit *between* characters, so a code inside an overlay splits that segment into
 * two marks around the code rather than dropping either. A presentational pair
 * (bold, italic, code) wraps the text and marks between its open and close in
 * the element, so an overlay over text inside a `<strong>` renders as a `<mark>`
 * inside that `<strong>`, and a code inside an overlay composes the same way.
 * Opaque codes (placeholders, links, breaks) render as chips in place.
 *
 * `limit` is how far the text has been revealed (a typewriter's visible prefix),
 * so a code appears only once its position has been reached; pass the full
 * length for a static reading. `renderText` draws one stretch of text, so a
 * caller can wrap it in its own mark or word diff; `renderCode` draws one
 * opaque code, so a caller can mark the chip a run anchor names. Absent, the
 * chip is drawn as it is.
 */
export function weaveInline(
  segments: TextSegment[],
  codes: InlineCode[],
  limit: number,
  renderText: (segment: TextSegment, text: string, key: string) => React.ReactNode,
  renderCode?: (code: InlineCode, key: string) => React.ReactNode,
): React.ReactNode[] {
  const root: Frame = { tag: null, children: [], key: "root" };
  const stack: Frame[] = [root];
  const top = () => stack[stack.length - 1];
  let ci = 0;
  let key = 0;

  const emitText = (seg: TextSegment, text: string) => {
    if (!text) return;
    top().children.push(renderText(seg, text, `t${key++}`));
  };
  const closeFrame = () => {
    const frame = stack.pop()!;
    top().children.push(
      frame.tag
        ? React.createElement(frame.tag, { key: frame.key }, ...frame.children)
        : React.createElement(React.Fragment, { key: frame.key }, ...frame.children),
    );
  };
  const emitCode = (code: InlineCode, i: number) => {
    const span = code.span;
    // A presentational pair opens a frame and closes it into its real element;
    // everything in between (text, marks, other codes) nests inside.
    if (span.span_type === "opening" && isPresentationalCode(span)) {
      stack.push({ tag: presentationalTag(span), children: [], key: `s${i}` });
      return;
    }
    if (span.span_type === "closing" && isPresentationalCode(span)) {
      if (stack.length > 1) closeFrame();
      return;
    }
    // An opaque code (placeholder, link, break, unknown) stays a chip.
    top().children.push(
      renderCode ? renderCode(code, `c${i}`) : <InlineCodeChip key={`c${i}`} code={code} />,
    );
  };
  const emitCodesAt = (offset: number) => {
    while (ci < codes.length && codes[ci].offset <= offset) {
      emitCode(codes[ci], ci);
      ci++;
    }
  };

  let pos = 0;
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
  // Close any presentational pair whose closing never came, so its text still
  // renders rather than vanishing with the open frame.
  while (stack.length > 1) closeFrame();
  return root.children;
}
