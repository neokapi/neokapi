// Pure validity → lane transforms behind the ConstraintsPanel (Apache-2.0).
// CONSTRAINTS are the temporal + per-market truth of a concept: when a term or
// relation is valid (valid-from → valid-to), and where a term is banned or
// preferred. This module turns that into two tested shapes:
//
//   • buildConstraintModel — every dated term/relation becomes a LANE positioned
//     on one shared time scale, so the panel can draw a small Gantt-style chart.
//     Open-ended windows (no valid-to) and open-start windows (no valid-from) are
//     flagged so the bars can render an "ongoing / since forever" cap, and each
//     lane carries whether it is in force as-of a reference instant.
//   • constraintSummary — the "banned where / preferred where" read, derived from
//     per-market term status, independent of any dates.
//
// No React here so the scaling, clamping, open-end, and as-of rules are
// unit-tested directly.

import { RELATION_LABEL, isBannedStatus, isPreferredStatus } from "./concept-meta";
import type { Concept, Market, Relation, RelationType, Term, TermStatus } from "./types";

const DAY_MS = 86_400_000;

// ── Time helpers ─────────────────────────────────────────────────────────────

function toMs(iso?: string): number | undefined {
  if (!iso) return undefined;
  const ms = new Date(iso).getTime();
  return Number.isNaN(ms) ? undefined : ms;
}

function clamp01(n: number): number {
  if (n < 0) return 0;
  if (n > 1) return 1;
  return n;
}

// ── Lanes ────────────────────────────────────────────────────────────────────

/** A single dated term/relation drawn as one bar on the shared time scale. */
export interface ConstraintLane {
  id: string;
  kind: "term" | "relation";
  /** The term text, or the relation phrase + neighbour. */
  label: string;
  locale?: string;
  market?: string;
  status?: TermStatus;
  relationType?: RelationType;
  note?: string;
  validFrom?: string;
  validTo?: string;
  /** No valid-from: valid since before the chart begins. */
  openStart: boolean;
  /** No valid-to: still in force (the bar runs to the right edge). */
  openEnd: boolean;
  /** In force at the model's `asOf` instant. */
  active: boolean;
  /** Bar start position on the scale, 0..1. */
  start: number;
  /** Bar end position on the scale, 0..1. */
  end: number;
}

/** A labelled gridline on the time scale. */
export interface ConstraintTick {
  label: string;
  /** Position on the scale, 0..1. */
  pos: number;
}

export interface ConstraintScale {
  fromMs: number;
  toMs: number;
  fromLabel: string;
  toLabel: string;
  ticks: ConstraintTick[];
  /** Position of the as-of marker, 0..1. */
  nowPos: number;
}

export interface ConstraintModel {
  asOf: string;
  scale: ConstraintScale;
  lanes: ConstraintLane[];
  /** True when at least one lane carries a real time window. */
  hasWindows: boolean;
}

export interface BuildConstraintsOptions {
  /** Reference instant for "in force" / the now marker. Default: Date.now(). */
  asOf?: string;
  /** Named markets, for resolving a term's market label. */
  markets?: Market[];
  /** The concept's direct relations (dated ones become lanes). */
  relations?: Relation[];
  /** Resolve a neighbour concept id to a label for relation lanes. */
  labelFor?: (conceptId: string) => string;
  /** Padding around the domain as a fraction of its span. Default 0.06. */
  padFraction?: number;
}

/** A term's market: its explicit `market` tag, else a market that covers its locale. */
export function marketLabelFor(term: Term, markets: Market[]): string | undefined {
  const tag = term.validity?.tags?.market;
  if (tag) {
    const named = markets.find((m) => m.id === tag || m.name === tag);
    return named?.name ?? tag;
  }
  const cover = markets.find((m) => m.locales.includes(term.locale));
  return cover?.name;
}

/** Year/short-month label for a tick or scale bound. */
function boundLabel(ms: number): string {
  return new Date(ms).toLocaleDateString(undefined, { month: "short", year: "numeric" });
}

/**
 * Up to a handful of evenly-read gridlines across the domain. Year starts when
 * the span covers years; otherwise three even ticks. Always within [from, to].
 */
function buildTicks(fromMs: number, toMs: number): ConstraintTick[] {
  const span = toMs - fromMs;
  if (span <= 0) return [];
  const pos = (ms: number) => clamp01((ms - fromMs) / span);
  const years = span / (DAY_MS * 365);

  if (years >= 1.2) {
    const ticks: ConstraintTick[] = [];
    const firstYear = new Date(fromMs).getUTCFullYear();
    const lastYear = new Date(toMs).getUTCFullYear();
    const step = lastYear - firstYear > 6 ? 2 : 1;
    for (let y = firstYear; y <= lastYear; y += step) {
      const ms = Date.UTC(y, 0, 1);
      if (ms < fromMs || ms > toMs) continue;
      ticks.push({ label: String(y), pos: pos(ms) });
    }
    if (ticks.length >= 2) return ticks;
  }

  // Short span: three even ticks labelled by month-year.
  return [0, 0.5, 1].map((f) => {
    const ms = fromMs + span * f;
    return { label: boundLabel(ms), pos: f };
  });
}

interface RawWindow {
  fromMs?: number;
  toMs?: number;
}

function collectInstants(windows: RawWindow[], asOfMs: number): number[] {
  const xs: number[] = [asOfMs];
  for (const w of windows) {
    if (w.fromMs !== undefined) xs.push(w.fromMs);
    if (w.toMs !== undefined) xs.push(w.toMs);
  }
  return xs;
}

/**
 * Build the lane model: every dated term and relation positioned on one shared
 * time scale, plus the scale (bounds, ticks, now marker). Items with no window
 * at all are excluded from the chart (they belong in constraintSummary).
 */
export function buildConstraintModel(
  concept: Concept,
  opts: BuildConstraintsOptions = {},
): ConstraintModel {
  const asOf = opts.asOf ?? new Date().toISOString();
  const asOfMs = toMs(asOf) ?? Date.now();
  const markets = opts.markets ?? [];
  const relations = opts.relations ?? [];
  const labelFor = opts.labelFor ?? ((id: string) => id);
  const pad = opts.padFraction ?? 0.06;

  interface Pending {
    lane: Omit<ConstraintLane, "start" | "end" | "active">;
    fromMs?: number;
    toMs?: number;
  }
  const pending: Pending[] = [];

  for (const term of concept.terms) {
    const fromMs = toMs(term.validity?.validFrom);
    const toMs_ = toMs(term.validity?.validTo);
    if (fromMs === undefined && toMs_ === undefined) continue; // no window
    pending.push({
      fromMs,
      toMs: toMs_,
      lane: {
        id: `term:${term.locale}:${term.text}`,
        kind: "term",
        label: term.text,
        locale: term.locale,
        market: marketLabelFor(term, markets),
        status: term.status,
        note: term.note,
        validFrom: term.validity?.validFrom,
        validTo: term.validity?.validTo,
        openStart: fromMs === undefined,
        openEnd: toMs_ === undefined,
      },
    });
  }

  for (const r of relations) {
    const touches = r.sourceId === concept.id || r.targetId === concept.id;
    if (!touches) continue;
    const fromMs = toMs(r.validity?.validFrom);
    const toMs_ = toMs(r.validity?.validTo);
    if (fromMs === undefined && toMs_ === undefined) continue;
    const otherId = r.sourceId === concept.id ? r.targetId : r.sourceId;
    if (otherId === concept.id) continue;
    const rel = RELATION_LABEL[r.type] ?? r.type.toLowerCase();
    pending.push({
      fromMs,
      toMs: toMs_,
      lane: {
        id: `rel:${r.id}`,
        kind: "relation",
        label: `${rel} ${labelFor(otherId)}`,
        relationType: r.type,
        note: r.note,
        validFrom: r.validity?.validFrom,
        validTo: r.validity?.validTo,
        openStart: fromMs === undefined,
        openEnd: toMs_ === undefined,
      },
    });
  }

  const hasWindows = pending.length > 0;

  // Domain: span every window instant + asOf, then pad. Guard a zero span.
  const instants = collectInstants(
    pending.map((p) => ({ fromMs: p.fromMs, toMs: p.toMs })),
    asOfMs,
  );
  let minMs = Math.min(...instants);
  let maxMs = Math.max(...instants);
  if (maxMs - minMs < DAY_MS) {
    minMs -= 30 * DAY_MS;
    maxMs += 30 * DAY_MS;
  }
  const padMs = (maxMs - minMs) * pad;
  const fromMs = minMs - padMs;
  const toMs_ = maxMs + padMs;
  const span = toMs_ - fromMs;
  const pos = (ms: number) => clamp01((ms - fromMs) / span);

  const lanes: ConstraintLane[] = pending
    .map((p) => {
      const startMs = p.fromMs ?? fromMs;
      const endMs = p.toMs ?? toMs_;
      const active =
        (p.fromMs === undefined || p.fromMs <= asOfMs) && (p.toMs === undefined || asOfMs < p.toMs);
      return {
        ...p.lane,
        start: pos(startMs),
        end: pos(endMs),
        active,
        _sort: p.fromMs ?? Number.NEGATIVE_INFINITY,
      };
    })
    .sort((a, b) => (a._sort !== b._sort ? a._sort - b._sort : a.label.localeCompare(b.label)))
    .map(({ _sort, ...lane }) => lane);

  return {
    asOf,
    scale: {
      fromMs,
      toMs: toMs_,
      fromLabel: boundLabel(fromMs),
      toLabel: boundLabel(toMs_),
      ticks: buildTicks(fromMs, toMs_),
      nowPos: pos(asOfMs),
    },
    lanes,
    hasWindows,
  };
}

// ── Banned-where / preferred-where summary ───────────────────────────────────

/** One term placement in the banned/preferred summary. */
export interface ConstraintPlacement {
  locale: string;
  text: string;
  status: TermStatus;
  market?: string;
  note?: string;
  validFrom?: string;
  validTo?: string;
  /** In force at `asOf`. */
  active: boolean;
}

export interface ConstraintSummary {
  /** Forbidden or deprecated terms — never to be used (where). */
  banned: ConstraintPlacement[];
  /** Preferred terms — the one to reach for (where). */
  preferred: ConstraintPlacement[];
}

export interface SummaryOptions {
  asOf?: string;
  markets?: Market[];
}

function placement(term: Term, markets: Market[], asOfMs: number): ConstraintPlacement {
  const fromMs = toMs(term.validity?.validFrom);
  const toMs_ = toMs(term.validity?.validTo);
  return {
    locale: term.locale,
    text: term.text,
    status: term.status,
    market: marketLabelFor(term, markets),
    note: term.note,
    validFrom: term.validity?.validFrom,
    validTo: term.validity?.validTo,
    active: (fromMs === undefined || fromMs <= asOfMs) && (toMs_ === undefined || asOfMs < toMs_),
  };
}

function byMarketThenLocale(a: ConstraintPlacement, b: ConstraintPlacement): number {
  const am = a.market ?? "~";
  const bm = b.market ?? "~";
  if (am !== bm) return am.localeCompare(bm);
  return a.locale.localeCompare(b.locale);
}

/**
 * The "banned where / preferred where" read: every forbidden/deprecated term and
 * every preferred term, each placed by market + locale and flagged with whether
 * it is in force as-of the reference instant. Independent of any dates, so it
 * works for a flat termbase with no validity windows at all.
 */
export function constraintSummary(concept: Concept, opts: SummaryOptions = {}): ConstraintSummary {
  const markets = opts.markets ?? [];
  const asOfMs = toMs(opts.asOf ?? new Date().toISOString()) ?? Date.now();
  const banned: ConstraintPlacement[] = [];
  const preferred: ConstraintPlacement[] = [];
  for (const term of concept.terms) {
    if (isBannedStatus(term.status)) banned.push(placement(term, markets, asOfMs));
    else if (isPreferredStatus(term.status)) preferred.push(placement(term, markets, asOfMs));
  }
  banned.sort(byMarketThenLocale);
  preferred.sort(byMarketThenLocale);
  return { banned, preferred };
}

// ── Window phrasing (status-agnostic) ────────────────────────────────────────

/**
 * A short, status-agnostic phrase for a validity window: "Jan 2026 → Jun 2026",
 * "since Jan 2026" (open end), "until Jun 2026" (open start), or "always" (no
 * window). The panel composes the status verb ("preferred since …") around it.
 */
export function windowPhrase(validFrom?: string, validTo?: string): string {
  const from = toMs(validFrom);
  const to = toMs(validTo);
  if (from !== undefined && to !== undefined) return `${boundLabel(from)} → ${boundLabel(to)}`;
  if (from !== undefined) return `since ${boundLabel(from)}`;
  if (to !== undefined) return `until ${boundLabel(to)}`;
  return "always";
}
