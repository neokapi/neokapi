// Pure transforms behind the enriched concept story (AD-021): the timeline
// builder/sorter that merges the server's story entries into day-grouped,
// display-ready events; the per-market grouping of a concept's terms; and the
// relation grouping for the relations explorer. No React here so the ordering
// and bucketing rules are unit-tested directly.
import type { TermInfo } from "../../types/api";
import type {
  ConceptRelation,
  ConceptStoryEntry,
  ConceptStoryKind,
  Market,
  RelationType,
} from "../../types/brand-graph";
import { RELATION_TYPES } from "../../types/brand-graph";

// ── Story timeline ───────────────────────────────────────────────────────────

export type StoryTone = "create" | "revision" | "observation" | "comment" | "changeset";

export interface StoryDisplayEntry {
  id: string;
  kind: ConceptStoryKind;
  tone: StoryTone;
  at: string;
  actor?: string;
  /** A one-line headline for the event. */
  title: string;
  /** Optional longer body (e.g. a comment's text). */
  detail?: string;
}

export interface StoryDayGroup {
  /** Stable calendar-day key, YYYY-MM-DD. */
  key: string;
  /** A friendly date label for the day header. */
  label: string;
  entries: StoryDisplayEntry[];
}

function isCreation(entry: ConceptStoryEntry): boolean {
  if (entry.kind !== "revision") return false;
  if (entry.ref === "1") return true;
  return /^\s*created\b/i.test(entry.summary ?? "");
}

/** Map one raw story entry to its display shape. */
export function deriveStoryEntry(entry: ConceptStoryEntry, index: number): StoryDisplayEntry {
  const base = { id: `${entry.kind}-${entry.at}-${index}`, at: entry.at, actor: entry.actor };
  const summary = entry.summary?.trim();
  switch (entry.kind) {
    case "revision": {
      const created = isCreation(entry);
      return {
        ...base,
        kind: "revision",
        tone: created ? "create" : "revision",
        title: summary || (created ? "Created concept" : "Updated concept"),
      };
    }
    case "observation":
      return {
        ...base,
        kind: "observation",
        tone: "observation",
        title: summary || "Observation recorded",
      };
    case "comment":
      return {
        ...base,
        kind: "comment",
        tone: "comment",
        title: entry.actor ? `${entry.actor} commented` : "Comment added",
        detail: summary || undefined,
      };
    case "changeset":
      return {
        ...base,
        kind: "changeset",
        tone: "changeset",
        title: summary || "Change-set",
      };
    default:
      return { ...base, kind: entry.kind, tone: "revision", title: summary || "Event" };
  }
}

/** The YYYY-MM-DD calendar key for an instant (UTC-stable for ISO inputs). */
export function dayKey(iso: string): string {
  if (/^\d{4}-\d{2}-\d{2}/.test(iso)) return iso.slice(0, 10);
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "unknown";
  return d.toISOString().slice(0, 10);
}

function dayLabel(key: string): string {
  if (key === "unknown") return "Undated";
  const d = new Date(`${key}T12:00:00Z`);
  if (Number.isNaN(d.getTime())) return key;
  return d.toLocaleDateString(undefined, { day: "numeric", month: "long", year: "numeric" });
}

/**
 * Merge story entries into day-grouped, display-ready events. `order` controls
 * both the day order and the order within each day ("asc" tells the story from
 * the beginning; "desc" puts the latest activity first). Stable for equal
 * timestamps, so the output is deterministic.
 */
export function buildStoryTimeline(
  entries: ConceptStoryEntry[],
  order: "asc" | "desc" = "asc",
): StoryDayGroup[] {
  const display = entries.map(deriveStoryEntry);
  const dir = order === "asc" ? 1 : -1;
  const sorted = display
    .map((e, i) => ({ e, i }))
    .sort((a, b) => {
      if (a.e.at !== b.e.at) return a.e.at < b.e.at ? -dir : dir;
      return (a.i - b.i) * dir;
    })
    .map(({ e }) => e);

  const groups: StoryDayGroup[] = [];
  const byKey = new Map<string, StoryDayGroup>();
  for (const entry of sorted) {
    const key = dayKey(entry.at);
    let group = byKey.get(key);
    if (!group) {
      group = { key, label: dayLabel(key), entries: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.entries.push(entry);
  }
  return groups;
}

// ── Terms by market ──────────────────────────────────────────────────────────

export interface MarketTermGroup {
  /** The matching market, or null for the "other locales" bucket. */
  market: Market | null;
  name: string;
  locales: { locale: string; terms: TermInfo[] }[];
}

function groupByLocale(terms: TermInfo[]): Map<string, TermInfo[]> {
  const map = new Map<string, TermInfo[]>();
  for (const t of terms) {
    const arr = map.get(t.locale) ?? [];
    arr.push(t);
    map.set(t.locale, arr);
  }
  return map;
}

/**
 * Group a concept's terms by market: each market that covers at least one of the
 * term locales becomes a section listing its locales and their terms; any locale
 * covered by no market falls into a trailing "Other locales" section. A locale
 * shared by several markets appears under each, so the per-market truth reads
 * side by side.
 */
export function termsByMarket(terms: TermInfo[], markets: Market[]): MarketTermGroup[] {
  const byLocale = groupByLocale(terms);
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

  const otherLocales = [...byLocale.keys()].filter((loc) => !covered.has(loc));
  if (otherLocales.length > 0) {
    groups.push({
      market: null,
      name: "Other locales",
      locales: otherLocales.map((loc) => ({ locale: loc, terms: byLocale.get(loc)! })),
    });
  }
  return groups;
}

// ── Relation grouping ────────────────────────────────────────────────────────

export interface RelationGroupItem {
  relation: ConceptRelation;
  /** The concept on the far end of the relation, from the subject's view. */
  otherId: string;
  /** True when the stored relation points away from the subject. */
  outgoing: boolean;
}

export interface RelationGroup {
  type: RelationType;
  items: RelationGroupItem[];
}

/**
 * Group a concept's relations by type (in the canonical RELATION_TYPES order),
 * resolving each to the neighbour on the far end relative to `subjectId`. Only
 * non-empty groups are returned.
 */
export function groupRelations(relations: ConceptRelation[], subjectId: string): RelationGroup[] {
  const byType = new Map<RelationType, RelationGroupItem[]>();
  for (const r of relations) {
    const outgoing = r.source_id === subjectId;
    const otherId = outgoing ? r.target_id : r.source_id;
    const item: RelationGroupItem = { relation: r, otherId, outgoing };
    const arr = byType.get(r.relation_type) ?? [];
    arr.push(item);
    byType.set(r.relation_type, arr);
  }
  return RELATION_TYPES.filter((t) => byType.has(t)).map((type) => ({
    type,
    items: byType.get(type)!,
  }));
}
