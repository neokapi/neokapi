// Pure view-model for the local relations widget (Apache-2.0). The widget shows
// the current concept and its DIRECT relations only, grouped by type. This module
// turns raw relations into the ordered, collapse-decided groups the panel renders
// — composing groupRelations + shouldCollapse from ./grouping behind one tested
// function. No React here, so the reading order and the collapse rule are checked
// directly.

import { groupRelations, shouldCollapse } from "./grouping";
import type { RelationGroup } from "./grouping";
import type { Relation, RelationType } from "./types";

/**
 * Reading order for the relation lanes. Lifecycle guidance first (what to say
 * instead, what replaced this), then the hierarchy (broader/narrower, part-of),
 * then lateral links, then cross-vocabulary matches, then competitors last —
 * the order a writer scans when deciding the wording to use.
 */
export const RELATION_DISPLAY_ORDER: readonly RelationType[] = [
  "USE_INSTEAD",
  "REPLACED_BY",
  "BROADER",
  "NARROWER",
  "PART_OF",
  "HAS_PART",
  "RELATED",
  "EXACT_MATCH",
  "CLOSE_MATCH",
  "COMPETITOR",
];

/**
 * The default neighbour count past which a lane collapses to a single
 * "N related →" affordance. Kept a touch higher than the framework's
 * RELATION_COLLAPSE_THRESHOLD: the widget can show a handful of small nodes
 * inline before a group is worth folding away.
 */
export const DEFAULT_RELATIONS_COLLAPSE = 6;

/** A relation group plus the panel's collapse decision and neighbour count. */
export interface RelationView extends RelationGroup {
  /** Neighbour count (items.length), surfaced in the lane header/affordance. */
  count: number;
  /** True when the lane folds to one affordance by default (too many inline). */
  collapsed: boolean;
}

function orderIndex(type: RelationType): number {
  const i = RELATION_DISPLAY_ORDER.indexOf(type);
  return i === -1 ? RELATION_DISPLAY_ORDER.length : i;
}

/**
 * Build the relations widget view-model from a concept's direct relations: bucket
 * by type (resolving each edge to the neighbour relative to `subjectId`), order
 * the lanes for reading, and mark which lanes collapse past `threshold`. Stable
 * and side-effect free.
 */
export function buildRelationView(
  relations: Relation[],
  subjectId: string,
  threshold: number = DEFAULT_RELATIONS_COLLAPSE,
): RelationView[] {
  return groupRelations(relations, subjectId)
    .slice()
    .sort((a, b) => orderIndex(a.type) - orderIndex(b.type))
    .map((group) => ({
      ...group,
      count: group.items.length,
      collapsed: shouldCollapse(group, threshold),
    }));
}

/** The distinct neighbour ids across every lane, first-seen order, de-duplicated. */
export function neighbourIds(views: RelationView[]): string[] {
  const seen = new Set<string>();
  const ids: string[] = [];
  for (const v of views) {
    for (const item of v.items) {
      if (seen.has(item.otherId)) continue;
      seen.add(item.otherId);
      ids.push(item.otherId);
    }
  }
  return ids;
}
