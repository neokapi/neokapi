// Pure view-model for the geography (markets) panel (Apache-2.0). The panel shows
// one panel per market/region with the term and status this concept uses there,
// making "where is this banned or preferred" scannable. This module groups a
// concept's terms by market (named markets when the source has them, otherwise
// markets derived from validity tags), orders each locale's terms most-blessed →
// most-restricted, and annotates every market and locale with banned/preferred
// flags. No React here, so the ordering and the flag rules are checked directly.

import { isBannedStatus, isPreferredStatus } from "./concept-meta";
import { termsByMarket } from "./grouping";
import type { LocaleTerms, MarketTermGroup } from "./grouping";
import { TERM_STATUSES } from "./types";
import type { Market, Term } from "./types";

function statusRank(term: Term): number {
  const i = TERM_STATUSES.indexOf(term.status);
  return i === -1 ? TERM_STATUSES.length : i;
}

/**
 * Order a locale's terms most-blessed → most-restricted (preferred first, banned
 * last), stable for equal status. The wording to lead with rises to the top and
 * banned variants sink, so a market panel reads at a glance.
 */
export function orderLocaleTerms(terms: Term[]): Term[] {
  return terms
    .map((term, i) => ({ term, i }))
    .sort((a, b) => statusRank(a.term) - statusRank(b.term) || a.i - b.i)
    .map(({ term }) => term);
}

/** A locale within a market, with its ordered terms and banned/preferred flags. */
export interface MarketLocaleView extends LocaleTerms {
  /** Terms ordered most-blessed → most-restricted. */
  terms: Term[];
  /** The wording to lead with: the most-blessed non-banned term, else the first. */
  primary: Term;
  hasBanned: boolean;
  hasPreferred: boolean;
}

/** A market/region panel's worth of this concept's wording, fully annotated. */
export interface MarketView {
  /** The named market, or null for the trailing "Other locales" bucket. */
  market: Market | null;
  name: string;
  description?: string;
  locales: MarketLocaleView[];
  localeCount: number;
  termCount: number;
  /** A term in this market is forbidden or deprecated. */
  hasBanned: boolean;
  /** A term in this market is preferred. */
  hasPreferred: boolean;
}

function buildLocaleView(group: LocaleTerms): MarketLocaleView {
  const terms = orderLocaleTerms(group.terms);
  const hasBanned = terms.some((t) => isBannedStatus(t.status));
  const hasPreferred = terms.some((t) => isPreferredStatus(t.status));
  const primary = terms.find((t) => !isBannedStatus(t.status)) ?? terms[0];
  return { locale: group.locale, terms, primary, hasBanned, hasPreferred };
}

/**
 * Build the geography view-model: group a concept's terms by market (named or
 * derived — the caller decides which `markets` to pass) and annotate each market
 * and locale with banned/preferred flags. Stable and side-effect free.
 */
export function buildMarketView(terms: Term[], markets: Market[]): MarketView[] {
  return termsByMarket(terms, markets).map((group: MarketTermGroup) => {
    const locales = group.locales.map(buildLocaleView);
    return {
      market: group.market,
      name: group.name,
      description: group.market?.description,
      locales,
      localeCount: locales.length,
      termCount: locales.reduce((sum, l) => sum + l.terms.length, 0),
      hasBanned: locales.some((l) => l.hasBanned),
      hasPreferred: locales.some((l) => l.hasPreferred),
    };
  });
}
