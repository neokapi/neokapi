// Pure grouping transforms shared by the concept-view panels (Apache-2.0):
//
//   • groupRelations  — a concept's direct relations bucketed by type, each
//     resolved to the neighbour on the far end (feeds the local relations
//     widget; a type with many targets collapses behind one affordance).
//   • termsByLocale / termsByMarket — terms grouped for the geography panels.
//   • deriveMarketsFromTerms — the FRAMEWORK-ONLY geography axis: anonymous
//     markets synthesised from term validity tags when a source has no named
//     markets.
//
// No React here so the bucketing and ordering rules are unit-tested directly.

import { RELATION_TYPES } from "./types";
import type { Market, Relation, RelationType, Term } from "./types";

// ── Relations ────────────────────────────────────────────────────────────────

export interface RelationItem {
  relation: Relation;
  /** The concept on the far end, from the subject's point of view. */
  otherId: string;
  /** True when the stored relation points away from the subject. */
  outgoing: boolean;
}

export interface RelationGroup {
  type: RelationType;
  items: RelationItem[];
}

/**
 * Group a concept's relations by type (in canonical RELATION_TYPES order),
 * resolving each to the neighbour relative to `subjectId`. Only non-empty groups
 * are returned. Self-relations (source === target === subject) are dropped.
 */
export function groupRelations(relations: Relation[], subjectId: string): RelationGroup[] {
  const byType = new Map<RelationType, RelationItem[]>();
  for (const r of relations) {
    const touchesSubject = r.sourceId === subjectId || r.targetId === subjectId;
    if (!touchesSubject) continue;
    const outgoing = r.sourceId === subjectId;
    const otherId = outgoing ? r.targetId : r.sourceId;
    if (otherId === subjectId) continue; // self-loop
    const arr = byType.get(r.type) ?? [];
    arr.push({ relation: r, otherId, outgoing });
    byType.set(r.type, arr);
  }
  return RELATION_TYPES.filter((t) => byType.has(t)).map((type) => ({
    type,
    items: byType.get(type)!,
  }));
}

/**
 * The default count past which a relation group collapses to a single
 * "N related →" affordance in the local relations widget. Groups at or below it
 * render their neighbours inline.
 */
export const RELATION_COLLAPSE_THRESHOLD = 4;

/** Whether a group should collapse, given a threshold (defaults to the above). */
export function shouldCollapse(
  group: RelationGroup,
  threshold: number = RELATION_COLLAPSE_THRESHOLD,
): boolean {
  return group.items.length > threshold;
}

// ── Terms by locale / market ─────────────────────────────────────────────────

export interface LocaleTerms {
  locale: string;
  terms: Term[];
}

/** Group terms by locale, preserving first-seen locale order. */
export function termsByLocale(terms: Term[]): LocaleTerms[] {
  const map = new Map<string, Term[]>();
  for (const t of terms) {
    const arr = map.get(t.locale) ?? [];
    arr.push(t);
    map.set(t.locale, arr);
  }
  return [...map.entries()].map(([locale, ts]) => ({ locale, terms: ts }));
}

export interface MarketTermGroup {
  /** The matching market, or null for the trailing "other locales" bucket. */
  market: Market | null;
  name: string;
  locales: LocaleTerms[];
}

/**
 * Group a concept's terms by market: each market covering at least one term
 * locale becomes a section listing its locales and their terms; any locale
 * covered by no market falls into a trailing "Other locales" section. A locale
 * shared by several markets appears under each, so the per-market truth reads
 * side by side.
 */
export function termsByMarket(terms: Term[], markets: Market[]): MarketTermGroup[] {
  const byLocale = new Map(termsByLocale(terms).map((g) => [g.locale, g.terms]));
  const groups: MarketTermGroup[] = [];
  const covered = new Set<string>();

  for (const market of markets) {
    const locales = market.locales
      .filter((loc) => byLocale.has(loc))
      .map((loc) => ({ locale: loc, terms: byLocale.get(loc)! }));
    if (locales.length === 0) continue;
    for (const { locale } of locales) covered.add(locale);
    groups.push({ market, name: market.name, locales });
  }

  const other = [...byLocale.keys()].filter((loc) => !covered.has(loc));
  if (other.length > 0) {
    groups.push({
      market: null,
      name: "Other locales",
      locales: other.map((loc) => ({ locale: loc, terms: byLocale.get(loc)! })),
    });
  }
  return groups;
}

/**
 * FRAMEWORK-ONLY geography: synthesise anonymous markets from the `market`
 * validity tag on a concept's terms. A source with no named markets can still
 * render market panels — each distinct `validity.tags.market` value becomes a
 * Market whose locales are those of the terms carrying it. The tag key defaults
 * to "market". Returned in first-seen order.
 */
export function deriveMarketsFromTerms(terms: Term[], tagKey = "market"): Market[] {
  const order: string[] = [];
  const locales = new Map<string, Set<string>>();
  for (const t of terms) {
    const name = t.validity?.tags?.[tagKey];
    if (!name) continue;
    if (!locales.has(name)) {
      locales.set(name, new Set());
      order.push(name);
    }
    locales.get(name)!.add(t.locale);
  }
  return order.map((name) => ({ name, locales: [...locales.get(name)!] }));
}
