// Pure derivation helpers for the Brand dashboard (AD-021). Kept free of React
// so they can be unit-tested directly: locale coverage from concepts, score
// aggregation from stored brand checks, and the waiting order for the pending
// governance queue.
import type { ConceptInfo } from "../../types/api";
import type { ChangeSet } from "../../types/brand-graph";
import type { DimensionScore, StoredScore } from "../../brand/types";

/** Per-locale completeness of the concept vocabulary. */
export interface LocaleCoverage {
  locale: string;
  /** Concepts carrying at least one term in this locale. */
  present: number;
  /** Concepts considered (the full sample). */
  total: number;
  /** present / total as a 0–100 integer. */
  pct: number;
}

/** Canonical dimension order so breakdowns read the same everywhere. */
const DIMENSION_ORDER = ["tone", "style", "vocabulary", "clarity", "brand_compliance"];

/**
 * computeLocaleCoverage reports, for every locale that appears across the given
 * concepts, how many concepts define a term in it. The result is sorted by
 * coverage (then locale) so the most-complete locales lead.
 */
export function computeLocaleCoverage(concepts: ConceptInfo[]): LocaleCoverage[] {
  const total = concepts.length;
  if (total === 0) return [];
  const present = new Map<string, number>();
  for (const concept of concepts) {
    const locales = new Set<string>();
    for (const term of concept.terms) {
      if (term.locale) locales.add(term.locale);
    }
    for (const locale of locales) {
      present.set(locale, (present.get(locale) ?? 0) + 1);
    }
  }
  return [...present.entries()]
    .map(([locale, count]) => ({
      locale,
      present: count,
      total,
      pct: Math.round((count / total) * 100),
    }))
    .sort((a, b) => b.present - a.present || a.locale.localeCompare(b.locale));
}

/** averageScore is the rounded mean of stored brand-check scores, or null when empty. */
export function averageScore(scores: StoredScore[]): number | null {
  if (scores.length === 0) return null;
  const sum = scores.reduce((acc, s) => acc + s.score, 0);
  return Math.round(sum / scores.length);
}

/**
 * aggregateDimensions averages each dimension across the stored scores, summing
 * issue counts, and returns them in canonical order. Dimensions absent from the
 * data are omitted.
 */
export function aggregateDimensions(scores: StoredScore[]): DimensionScore[] {
  const acc = new Map<string, { score: number; penalty: number; issues: number; n: number }>();
  for (const stored of scores) {
    for (const dim of stored.dimensions) {
      const cur = acc.get(dim.dimension) ?? { score: 0, penalty: 0, issues: 0, n: 0 };
      cur.score += dim.score;
      cur.penalty += dim.penalty;
      cur.issues += dim.issues;
      cur.n += 1;
      acc.set(dim.dimension, cur);
    }
  }
  const out: DimensionScore[] = [];
  for (const [dimension, v] of acc) {
    out.push({
      dimension: dimension as DimensionScore["dimension"],
      score: Math.round(v.score / v.n),
      penalty: Math.round(v.penalty / v.n),
      issues: v.issues,
    });
  }
  return out.sort((a, b) => dimRank(a.dimension) - dimRank(b.dimension));
}

function dimRank(dimension: string): number {
  const i = DIMENSION_ORDER.indexOf(dimension);
  return i === -1 ? DIMENSION_ORDER.length : i;
}

/** The instant a change-set started waiting on a steward. */
export function waitingSince(cs: ChangeSet): string {
  return cs.submitted_at || cs.updated_at || cs.created_at;
}

/**
 * sortByWaiting orders change-sets oldest-first, so the proposal that has been
 * waiting longest sits at the top of the governance queue.
 */
export function sortByWaiting(changesets: ChangeSet[]): ChangeSet[] {
  return [...changesets].sort(
    (a, b) => new Date(waitingSince(a)).getTime() - new Date(waitingSince(b)).getTime(),
  );
}

/** sortByRecent orders change-sets newest-first by their last update. */
export function sortByRecent(changesets: ChangeSet[]): ChangeSet[] {
  return [...changesets].sort(
    (a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime(),
  );
}

/** Change-sets that are not yet settled (a draft, in review, or approved-not-merged). */
export function activeExperiments(changesets: ChangeSet[]): ChangeSet[] {
  return changesets.filter((c) => c.status !== "merged" && c.status !== "abandoned");
}

/** Change-sets a steward must act on: in review, or approved and awaiting merge. */
export function pendingDecisions(changesets: ChangeSet[]): ChangeSet[] {
  return sortByWaiting(
    changesets.filter((c) => c.status === "in_review" || c.status === "approved"),
  );
}
