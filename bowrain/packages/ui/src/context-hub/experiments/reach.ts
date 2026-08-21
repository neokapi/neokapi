// Selectors for the reach split. Pure functions over knowledge.Reach so the
// panel renders and the tests assert against the same arithmetic.
import type { ChangeSetImpact, Reach, ReachClass } from "../../types/brand-graph";

/** The two cost classes, in the order the panel reads them. */
export type ReachKind = "annotate" | "transform";

/** One segment of the split bar: its share of the affected blocks. */
export interface ReachSegment {
  kind: ReachKind;
  blocks: number;
  /** Share of the split's total blocks, 0–1. Zero when nothing is affected. */
  share: number;
}

/** The blocks the split covers — annotate plus transform. */
export function reachTotal(reach: Reach): number {
  return reach.annotate.blocks + reach.transform.blocks;
}

/**
 * Segments of the split bar, largest class first so the bar reads left to right
 * from the cheap end when a draft is mostly annotation and from the expensive
 * end when it is not — the shape of the bar is the headline.
 */
export function reachSegments(reach: Reach): ReachSegment[] {
  const total = reachTotal(reach);
  const of = (kind: ReachKind, cls: ReachClass): ReachSegment => ({
    kind,
    blocks: cls.blocks,
    share: total > 0 ? cls.blocks / total : 0,
  });
  return [of("annotate", reach.annotate), of("transform", reach.transform)]
    .filter((s) => s.blocks > 0)
    .sort((a, b) => b.blocks - a.blocks);
}

/**
 * Whether the split can be shown at all. A report from the stored summary
 * carries the counts but not the locale lists, and a report with nothing
 * affected has no split to draw.
 */
export function hasReach(impact?: ChangeSetImpact): impact is ChangeSetImpact & { reach: Reach } {
  return !!impact?.reach && reachTotal(impact.reach) > 0;
}

/**
 * The split covers the affected blocks exactly, unless the report came from a
 * walk that ran out of budget. A mismatch outside that case means the panel is
 * describing a different population from the hero above it, which is worth
 * saying rather than papering over.
 */
export function splitCoversAffected(impact: ChangeSetImpact): boolean {
  if (!impact.reach) return false;
  return reachTotal(impact.reach) === impact.affected_blocks;
}

/** A locale list rendered for reading: "de, nb" — or "" when there are none. */
export function formatLocales(locales: string[] | undefined): string {
  return (locales ?? []).join(", ");
}
