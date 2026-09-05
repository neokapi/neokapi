// highlights — spans a host asks the preview to mark, addressed the way a
// finding addresses them.
//
// A check finding records where it sits as a run anchor over the block it was
// raised on. The preview draws overlays from the tree it is given, but a
// finding is not in the tree: it is the host's evidence about a block, produced
// by a check run, and it arrives beside the document rather than inside it.
// `PreviewHighlight` is that evidence in the shape the preview can draw: which
// side, which anchor, how hard it bites, what to say on hover. Each one resolves
// through `charSpanForAnchor` into a `ResolvedSpan`, the same shape an overlay
// resolves into, so the rest of the rendering (segmenting, weaving codes back
// in, the mark) is the one path every span takes.

import { findingToneMarkClass, type FindingTone } from "../../lib/finding-severity";
import { charSpanForAnchor, runsPlainText } from "./anchor";
import type { ResolvedSpan } from "./overlayHighlight";
import type { Anchor, ContentNode, ContentTree, Run } from "./types";

/** How a highlight stands among the others drawn on the same document. */
export type HighlightEmphasis = "focus" | "dim";

/** One span to mark in a document, as a finding addresses it. */
export interface PreviewHighlight {
  /**
   * The side whose runs the anchor addresses, which is where the mark is drawn:
   * "source", or a target locale key.
   */
  side: string;
  /**
   * Where in that side's runs the finding sits. A block anchor marks the whole
   * text; a run anchor marks the inline code's chip.
   */
  anchor: Anchor;
  /** How hard the finding bites, on the shared scale; it paints the mark. */
  tone: FindingTone;
  /** What the mark says on hover: the finding's message. */
  label: string;
  /**
   * "focus" for the one the reader came to see, "dim" for the others drawn
   * alongside it. Unset draws the mark plainly.
   */
  emphasis?: HighlightEmphasis;
}

/** Highlights by the id of the block they sit on. */
export type PreviewHighlights = Readonly<Record<string, readonly PreviewHighlight[]>>;

/** The overlay type a highlight's mark carries, so a style or a test can address it. */
export const HIGHLIGHT_TYPE = "finding";

/**
 * One highlight located over its side's runs, or null when its anchor addresses
 * nothing in their text.
 */
export function resolveHighlight(
  runs: Run[] | undefined,
  h: PreviewHighlight,
): ResolvedSpan | null {
  const at = charSpanForAnchor(runs, h.anchor);
  if (!at) return null;
  const covered = runsPlainText(runs).slice(at.start, at.end);
  return {
    start: at.start,
    end: at.end,
    type: HIGHLIGHT_TYPE,
    style: { className: findingToneMarkClass(h.tone), label: "Finding" },
    span: {
      range: h.anchor,
      text: covered,
      props: { tone: h.tone, ...(h.emphasis ? { emphasis: h.emphasis } : {}) },
    },
    tooltip: h.label,
    ...(h.emphasis ? { emphasis: h.emphasis } : {}),
    ...(h.anchor.kind === "run" && h.anchor.runId ? { code: h.anchor.runId } : {}),
  };
}

/**
 * The highlights for one side of a block, located over that side's runs and
 * sorted by start, ready to segment the text with.
 */
export function highlightSpans(
  runs: Run[] | undefined,
  highlights: readonly PreviewHighlight[] | undefined,
  side: string,
): ResolvedSpan[] {
  if (!highlights || highlights.length === 0) return [];
  const out: ResolvedSpan[] = [];
  for (const h of highlights) {
    if (h.side !== side) continue;
    const span = resolveHighlight(runs, h);
    if (span) out.push(span);
  }
  out.sort((a, b) => a.start - b.start || b.end - a.end);
  return out;
}

/** A block's runs on one side: the source, or the target the key names. */
export function runsForSide(
  node: Pick<ContentNode, "source" | "targets">,
  side: string,
): Run[] | undefined {
  return side === "source" ? node.source : node.targets?.[side];
}

/**
 * Every highlighted block of a tree, resolved for each side its highlights name:
 * block id, then side, to the spans over that side's text. The document reading
 * looks a line up here by its block id and the side in view.
 */
export function highlightSpansByBlock(
  tree: ContentTree | null | undefined,
  highlights: PreviewHighlights | undefined,
): Map<string, Map<string, ResolvedSpan[]>> {
  const out = new Map<string, Map<string, ResolvedSpan[]>>();
  if (!tree || !highlights) return out;
  const walk = (n: ContentNode) => {
    if (n.kind === "block") {
      const own = highlights[n.id];
      if (own && own.length > 0) {
        const bySide = new Map<string, ResolvedSpan[]>();
        for (const side of new Set(own.map((h) => h.side))) {
          const spans = highlightSpans(runsForSide(n, side), own, side);
          if (spans.length > 0) bySide.set(side, spans);
        }
        if (bySide.size > 0) out.set(n.id, bySide);
      }
    }
    n.children?.forEach(walk);
  };
  tree.root.forEach(walk);
  return out;
}
