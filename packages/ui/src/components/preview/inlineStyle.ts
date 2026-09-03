// The one classification every reading of a block's inline codes shares: which
// codes carry inline STYLE (render as their real element) and which are OPAQUE
// (render as a chip, because the reader cannot see what they stand for).
//
// A presentational code (bold, italic, monospace code, underline, strike,
// sub/superscript, highlight) is a pair around text that reads as that style.
// An opaque code (a placeholder or variable, a link, a line break, a structural
// or unknown code) stands for something with no rendered form, so it stays a
// chip. The line is drawn by the vocabulary CATEGORY, not a per-name list here,
// so a formatting type added to the vocabulary is styled and a placeholder type
// is chipped by declaration.
//
// This is the single source of truth both the faithful RenderedDocument and the
// structured FormatPreview / KeyedTable consume, so a bold pair reads as
// <strong> and a code pair as monospace <code> in either.

import type * as React from "react";
import type { SpanInfo } from "../../types/span";
import { semanticCategory } from "../editor/tagSemantics";

// Canonical run type → inline HTML element. Mirrors the writers' vocabulary
// (core/model/vocabularies/common-formatting.json) so the preview agrees with
// cross-format output.
export const INLINE_TAG: Record<string, keyof React.JSX.IntrinsicElements> = {
  "fmt:bold": "strong",
  "fmt:italic": "em",
  "fmt:underline": "u",
  "fmt:strikethrough": "s",
  "fmt:code": "code",
  "fmt:highlight": "mark",
  "fmt:superscript": "sup",
  "fmt:subscript": "sub",
};

/**
 * True when a code carries inline style rather than standing for hidden content.
 * Formatting codes render as their real element; everything else is a chip.
 */
export function isPresentationalCode(span: SpanInfo): boolean {
  return semanticCategory(span) === "formatting";
}

/**
 * The inline element a presentational code renders as, or null to render its
 * content without markup. A formatting type the shared map does not name (a
 * bidi isolate, a handwriting span) shows its text rather than a chip.
 */
export function presentationalTag(span: SpanInfo): keyof React.JSX.IntrinsicElements | null {
  return INLINE_TAG[span.type] ?? null;
}
