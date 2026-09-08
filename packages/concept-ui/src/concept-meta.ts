// Shared, framework-level vocabulary for the concept UI (Apache-2.0): human
// labels for term statuses and relation types, the status colour map, and the
// rule for a concept's display name. Kept pure (no React) so every section
// renders the same vocabulary and so the rules are unit-tested directly.

import type { ConceptSummary, Term, RelationType, TermStatus } from "./types";

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
 * Locale hints for naming a concept, strongest first. An app that knows the
 * source locale of the content the vocabulary governs supplies `sourceLocale`;
 * one that knows the language the viewer reads the app in supplies `uiLocale`.
 * With neither, naming answers to English and then to a fixed order over the
 * terms, so the same concept reads the same way on every render.
 */
export interface ConceptNaming {
  /** The source locale of the governed content, e.g. "en-US". */
  sourceLocale?: string;
  /** The locale the viewer reads the app in, e.g. "nb-NO". */
  uiLocale?: string;
}

/** The primary language subtag, lowercased: "en-US" gives "en". */
function languageOf(locale: string): string {
  const cut = locale.search(/[-_]/);
  return (cut === -1 ? locale : locale.slice(0, cut)).toLowerCase();
}

/** How well a term's locale answers to `target`: 2 exact, 1 same language, 0 no. */
function localeRank(termLocale: string, target: string): number {
  if (termLocale.toLowerCase() === target.toLowerCase()) return 2;
  return languageOf(termLocale) === languageOf(target) ? 1 : 0;
}

/**
 * The best term for `target`, preferring an exact locale match over a match on
 * the language alone and keeping the store's order within a rank. `blessedOnly`
 * restricts the search to terms marked preferred.
 */
function pickForLocale(terms: Term[], target: string, blessedOnly: boolean): Term | undefined {
  let best: Term | undefined;
  let bestRank = 0;
  for (const term of terms) {
    if (blessedOnly && !isPreferredStatus(term.status)) continue;
    const rank = localeRank(term.locale, target);
    if (rank > bestRank) {
      best = term;
      bestRank = rank;
    }
  }
  return best;
}

/** Locale then text, compared as code points so every runtime agrees. */
function byLocaleThenText(a: Term, b: Term): number {
  if (a.locale !== b.locale) return a.locale < b.locale ? -1 : 1;
  if (a.text !== b.text) return a.text < b.text ? -1 : 1;
  return 0;
}

/**
 * A human label for a concept, in the language its reader expects.
 *
 * The ladder runs source locale, then the viewer's locale, then English, taking
 * a preferred term in each before settling for any term in it. A store that
 * lists a concept's Arabic term first therefore still heads an English-source
 * card with the English term. When no hint matches, the first preferred term in
 * locale-then-text order wins, else the first term, else the domain or the id.
 */
export function primaryName(concept: ConceptSummary, naming: ConceptNaming = {}): string {
  const terms = concept.terms;
  if (terms.length === 0) return concept.domain || concept.id;

  const targets: string[] = [];
  for (const target of [naming.sourceLocale, naming.uiLocale, "en"]) {
    if (target && !targets.includes(target)) targets.push(target);
  }
  for (const target of targets) {
    const hit = pickForLocale(terms, target, true) ?? pickForLocale(terms, target, false);
    if (hit) return hit.text;
  }

  const ordered = [...terms].sort(byLocaleThenText);
  return (ordered.find((t) => isPreferredStatus(t.status)) ?? ordered[0]).text;
}
