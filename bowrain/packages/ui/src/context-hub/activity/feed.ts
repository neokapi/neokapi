// Pure presentation of the brand activity timeline (AD-021). The workspace
// activity feed is one server read — every governed and ordinary write to the
// graph is recorded there — so this file no longer weaves anything: it maps a
// recorded activity to a row, names the type families the category filter asks
// the server for, and groups a page by day. Kept React-free so all three are
// unit-tested directly.
import type { ActivityInfo, ConceptInfo } from "../../types/api";

export type ActivityCategory = "experiment" | "concept" | "observation" | "comment";

export const ACTIVITY_CATEGORIES: ActivityCategory[] = [
  "experiment",
  "concept",
  "observation",
  "comment",
];

/**
 * The activity type families each category covers. `?types=` matches a prefix
 * server-side, so a category is expressed as prefixes rather than as a list of
 * types — a family gains a member without this map changing. An experiment
 * spans three families: the change-set's own lifecycle, the review verdicts on
 * it, and the pilots run from it.
 */
export const CATEGORY_TYPE_PREFIXES: Record<ActivityCategory, string[]> = {
  experiment: ["changeset.", "review.", "pilot."],
  concept: ["concept."],
  observation: ["observation."],
  comment: ["comment."],
};

/**
 * The `types` filter for a set of categories, deduplicated and ordered so the
 * query key is stable. An empty set yields an empty list — the caller decides
 * whether that means "everything" or "nothing"; here it means the filter
 * excludes every family, so nothing should be fetched.
 */
export function typePrefixesFor(categories: ActivityCategory[]): string[] {
  const seen = new Set<string>();
  for (const category of ACTIVITY_CATEGORIES) {
    if (!categories.includes(category)) continue;
    for (const prefix of CATEGORY_TYPE_PREFIXES[category]) seen.add(prefix);
  }
  return [...seen];
}

/** The category an activity type belongs to, or null when no family claims it. */
export function categoryForType(type: string): ActivityCategory | null {
  for (const category of ACTIVITY_CATEGORIES) {
    if (CATEGORY_TYPE_PREFIXES[category].some((prefix) => type.startsWith(prefix))) {
      return category;
    }
  }
  return null;
}

/** One row of the brand timeline (pure data; the view adds icons and links). */
export interface FeedItem {
  id: string;
  at: string;
  category: ActivityCategory | null;
  type: string;
  actor?: string;
  /** The concept or change-set the event is about, named when it can be. */
  title: string;
  detail?: string;
  conceptId?: string;
  changesetId?: string;
}

/** Display names for the entities a feed page refers to, keyed by id. */
export type EntityNames = Record<string, string>;

/**
 * toFeedItem renders a recorded activity as a row. The server's summary is the
 * sentence — it is always present and always describes what happened — so it
 * leads whenever the entity's name is not known, and becomes the second line
 * when it is. An id is never shown: an unnamed entity reads as its sentence
 * rather than as a UUID.
 */
export function toFeedItem(activity: ActivityInfo, names: EntityNames = {}): FeedItem {
  const conceptId =
    activity.entity_type === "concept" ? activity.entity_id : activity.data?.concept_id;
  const changesetId =
    activity.entity_type === "changeset" ? activity.entity_id : activity.data?.changeset_id;
  const subject = changesetId ?? conceptId;
  const name = subject ? names[subject] : undefined;

  return {
    id: activity.id,
    at: activity.created_at,
    category: categoryForType(activity.type),
    type: activity.type,
    actor: activity.actor_id,
    title: name ?? activity.summary,
    detail: name ? activity.summary : undefined,
    conceptId,
    changesetId,
  };
}

/** A human-readable concept label: a preferred term, else an English term, else the first. */
export function conceptDisplayName(concept: ConceptInfo): string {
  if (concept.terms.length === 0) return concept.domain || concept.id;
  const preferred = concept.terms.find((t) => t.status === "preferred");
  const english = concept.terms.find((t) => t.locale.startsWith("en"));
  return (preferred ?? english ?? concept.terms[0]).text;
}

// ── Day grouping ──────────────────────────────────────────────────────────────

export interface DayGroup {
  key: string;
  label: string;
  items: FeedItem[];
}

function startOfDay(ms: number): number {
  const d = new Date(ms);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

/** dayLabel renders a day relative to `now`: "Today", "Yesterday", or an absolute date. */
export function dayLabel(iso: string, now: number = Date.now()): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "·";
  const diffDays = Math.round((startOfDay(now) - startOfDay(d.getTime())) / 86_400_000);
  if (diffDays <= 0) return "Today";
  if (diffDays === 1) return "Yesterday";
  return d.toLocaleDateString(undefined, { day: "numeric", month: "short", year: "numeric" });
}

function dayKey(iso: string): string {
  const d = new Date(iso);
  return `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;
}

/**
 * groupByDay buckets an already-sorted (newest-first) feed into day groups,
 * preserving order. Each group carries a relative label.
 */
export function groupByDay(items: FeedItem[], now: number = Date.now()): DayGroup[] {
  const groups: DayGroup[] = [];
  const byKey = new Map<string, DayGroup>();
  for (const item of items) {
    const key = dayKey(item.at);
    let group = byKey.get(key);
    if (!group) {
      group = { key, label: dayLabel(item.at, now), items: [] };
      byKey.set(key, group);
      groups.push(group);
    }
    group.items.push(item);
  }
  return groups;
}
