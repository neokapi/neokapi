import { findingSeverityTone } from "@neokapi/ui-primitives";
import type { PreviewHighlight, PreviewHighlights } from "@neokapi/ui-primitives/preview";
import type { DesktopFinding } from "../types/api";

/**
 * The side of the document a finding is read on: the source, or the target
 * locale it names. A target-side finding without a locale has nothing to open
 * on but the source.
 */
export function findingSide(finding: DesktopFinding): string {
  return finding.field === "target" && finding.locale ? finding.locale : "source";
}

/**
 * A file's findings as marks on its document, by block id, with the one the
 * reader came for in focus and the rest dimmed. With no finding in focus every
 * mark is drawn plainly.
 *
 * A finding's position is anchored to the block's source runs (core/check), so
 * the span is always drawn on the source side. A target-side finding also marks
 * its block whole on the target locale, which is the side the document opens
 * on for it, and a finding with no position marks its block whole on the side
 * it names. A finding with no block (a file that could not be read) marks
 * nothing.
 */
export function findingHighlights(
  findings: readonly DesktopFinding[],
  focused: DesktopFinding | null,
): PreviewHighlights {
  const out: Record<string, PreviewHighlight[]> = {};
  for (const f of findings) {
    if (!f.block_id) continue;
    const tone = findingSeverityTone(f.severity);
    const emphasis: PreviewHighlight["emphasis"] = focused
      ? f === focused
        ? "focus"
        : "dim"
      : undefined;
    const base: Omit<PreviewHighlight, "side" | "anchor"> = {
      tone,
      label: f.message,
      ...(emphasis ? { emphasis } : {}),
    };
    const list = (out[f.block_id] ??= []);
    if (f.field === "target" && f.locale) {
      list.push({ ...base, side: f.locale, anchor: { kind: "block" } });
      if (f.position) list.push({ ...base, side: "source", anchor: f.position });
    } else {
      list.push({ ...base, side: "source", anchor: f.position ?? { kind: "block" } });
    }
  }
  return out;
}
