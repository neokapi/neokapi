// What the Concepts detail pane reads off a concept, kept free of React so each
// rule is tested directly: the term a concept is headed by, how many languages
// it holds terms in, what a discouraged term gives way to, its terms language
// by language with the source first, the coordinates its properties name, and
// who added it and who changed it last according to its story.
import {
  isBannedStatus,
  isPreferredStatus,
  primaryName,
  termsByLocale,
  type ConceptNaming,
  type ConceptSummary,
  type LocaleTerms,
  type Term,
} from "@neokapi/concept-ui";
import type { ConceptStoryEntry } from "../../types/brand-graph";

/** The term a concept is headed by: the one its name is taken from, a usable one first. */
export function headlineTerm(concept: ConceptSummary, naming: ConceptNaming): Term | undefined {
  const name = primaryName(concept, naming);
  return (
    concept.terms.find((term) => term.text === name && !isBannedStatus(term.status)) ??
    concept.terms.find((term) => term.text === name)
  );
}

/** How many languages a concept has terms in. */
export function languageCount(concept: ConceptSummary): number {
  return new Set(concept.terms.map((term) => term.locale)).size;
}

/** What to write instead of a discouraged term: the preferred term in its language, else any usable one. */
export function replacementFor(term: Term, terms: Term[]): Term | undefined {
  const sameLocale = terms.filter((other) => other.locale === term.locale && other !== term);
  return (
    sameLocale.find((other) => isPreferredStatus(other.status)) ??
    sameLocale.find((other) => !isBannedStatus(other.status))
  );
}

/** The terms a concept discourages: deprecated and forbidden ones. */
export function discouragedTerms(concept: ConceptSummary): Term[] {
  return concept.terms.filter((term) => isBannedStatus(term.status));
}

/**
 * A concept's usable terms grouped by language, the source language first and
 * the rest in the order the store keeps them.
 */
export function languagesInOrder(concept: ConceptSummary, sourceLocale?: string): LocaleTerms[] {
  const groups = termsByLocale(concept.terms.filter((term) => !isBannedStatus(term.status)));
  if (!sourceLocale) return groups;
  return [
    ...groups.filter((group) => group.locale === sourceLocale),
    ...groups.filter((group) => group.locale !== sourceLocale),
  ];
}

/** The axes a concept's `coordinates` property names: `axis=value` pairs joined by commas. */
export function coordinatesOf(properties?: Record<string, string>): Array<[string, string]> {
  const text = properties?.coordinates ?? "";
  const out: Array<[string, string]> = [];
  for (const part of text.split(",")) {
    const cut = part.indexOf("=");
    if (cut <= 0) continue;
    const axis = part.slice(0, cut).trim();
    const value = part.slice(cut + 1).trim();
    if (axis && value) out.push([axis, value]);
  }
  return out;
}

/** Who added a concept and who changed it last, by the entries of its story that name someone. */
export interface ConceptProvenance {
  /** The oldest entry with an actor: who brought the concept into the workspace. */
  added?: ConceptStoryEntry;
  /** The newest revision or change-set with an actor, when it is not the entry that added it. */
  changed?: ConceptStoryEntry;
}

/** The story entries that change a concept itself, rather than talk about it. */
const CHANGING_KINDS: ReadonlySet<ConceptStoryEntry["kind"]> = new Set(["revision", "changeset"]);

function time(entry: ConceptStoryEntry): number {
  const at = Date.parse(entry.at);
  return Number.isNaN(at) ? 0 : at;
}

/** Read a concept's story for who added it and who changed it last. */
export function provenanceOf(entries: ConceptStoryEntry[] | undefined): ConceptProvenance {
  const named = (entries ?? []).filter((entry) => !!entry.actor);
  if (named.length === 0) return {};
  const ordered = [...named].sort((a, b) => time(a) - time(b));
  const added = ordered[0];
  const changing = ordered.filter((entry) => CHANGING_KINDS.has(entry.kind));
  const last = changing[changing.length - 1];
  return { added, changed: last && last !== added ? last : undefined };
}

/** The markets that cover a locale, by name. */
export function marketsFor(
  locale: string,
  markets: ReadonlyArray<{ name: string; locales: string[] }> | undefined,
): string[] {
  return (markets ?? []).filter((m) => m.locales.includes(locale)).map((m) => m.name);
}
