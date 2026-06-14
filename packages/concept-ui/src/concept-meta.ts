// Shared, framework-level vocabulary for the concept UI (Apache-2.0): human
// labels for term statuses and relation types, the status colour map, and the
// rule for a concept's display name. Kept pure (no React) so every section
// renders the same vocabulary and so the rules are unit-tested directly.

import type { ConceptSummary, RelationType, TermStatus } from "./types";

// ── Term status ──────────────────────────────────────────────────────────────

/** Tailwind classes for a status chip, keyed by @neokapi/ui design tokens. */
export const TERM_STATUS_CLASS: Record<TermStatus, string> = {
  preferred: "border-transparent bg-success/15 text-success",
  approved: "border-transparent bg-primary/15 text-primary",
  admitted: "border-transparent bg-warning/15 text-warning",
  proposed: "border-border bg-muted text-muted-foreground",
  deprecated: "border-border bg-muted text-muted-foreground line-through",
  forbidden: "border-transparent bg-destructive/15 text-destructive",
};

/** A short, sentence-case label for a status (matches the value itself). */
export const TERM_STATUS_LABEL: Record<TermStatus, string> = {
  preferred: "Preferred",
  approved: "Approved",
  admitted: "Admitted",
  proposed: "Proposed",
  deprecated: "Deprecated",
  forbidden: "Forbidden",
};

/** True for statuses that mark a term as not-to-be-used (banned). */
export function isBannedStatus(status: TermStatus): boolean {
  return status === "forbidden" || status === "deprecated";
}

/** True for statuses that bless a term (the one to reach for). */
export function isPreferredStatus(status: TermStatus): boolean {
  return status === "preferred";
}

// ── Relations ────────────────────────────────────────────────────────────────

/** A reading label for a relation, phrased from the subject's point of view. */
export const RELATION_LABEL: Record<RelationType, string> = {
  BROADER: "broader than",
  NARROWER: "narrower than",
  PART_OF: "part of",
  HAS_PART: "has part",
  RELATED: "related to",
  REPLACED_BY: "replaced by",
  USE_INSTEAD: "use instead",
  EXACT_MATCH: "exact match",
  CLOSE_MATCH: "close match",
  COMPETITOR: "competitor",
};

export function relationLabel(type: RelationType): string {
  return RELATION_LABEL[type];
}

/** REPLACED_BY is governed — platforms only land it through a change-set. */
export function isGovernedRelation(type: RelationType): boolean {
  return type === "REPLACED_BY";
}

// ── Concept naming ───────────────────────────────────────────────────────────

/**
 * A human label for a concept: a preferred term, else any English term, else the
 * first term, else the domain, else the bare id. Mirrors the brand-hub rule so
 * lists and headers agree.
 */
export function primaryName(concept: ConceptSummary): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((t) => t.status === "preferred");
  const english = concept.terms.find((t) => t.locale.toLowerCase().startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}
