// Pure derivation of the brand activity timeline (AD-021). There is no single
// brand-event endpoint, so the feed is woven from the governed write paths the
// hub already exposes: change-set lifecycle (from the list), reviews and pilots
// (from change-set details), and concept revisions, observations, and comments
// (from per-concept stories). Kept React-free so the weaving + day grouping are
// unit-tested directly.
import type { ConceptInfo } from "../../types/api";
import type { ChangeSet, ChangeSetDetail, ConceptStoryEntry } from "../../types/brand-graph";

export type ActivityCategory = "experiment" | "concept" | "observation" | "comment";

export const ACTIVITY_CATEGORIES: ActivityCategory[] = [
  "experiment",
  "concept",
  "observation",
  "comment",
];

export type FeedKind =
  | "changeset.opened"
  | "changeset.submitted"
  | "changeset.merged"
  | "changeset.abandoned"
  | "changeset.reviewed"
  | "pilot.started"
  | "concept.revision"
  | "observation"
  | "comment";

/** One event on the woven brand timeline (pure data; the view adds icons + links). */
export interface FeedItem {
  id: string;
  at: string;
  category: ActivityCategory;
  kind: FeedKind;
  actor?: string;
  /** The concept or change-set the event is about. */
  title: string;
  detail?: string;
  conceptId?: string;
  changesetId?: string;
}

/** A concept's story, paired with the concept id it belongs to. */
export interface StorySource {
  conceptId: string;
  entries: ConceptStoryEntry[];
}

export interface BuildFeedArgs {
  changesets: ChangeSet[];
  /** Change-set details, used only for their reviews + pilots. */
  details?: ChangeSetDetail[];
  /** Per-concept stories, used for revisions, observations, and comments. */
  stories?: StorySource[];
  /** concept id → display name, for human-readable titles. */
  conceptNames?: Record<string, string>;
  limit?: number;
}

type RawItem = Omit<FeedItem, "id">;

/**
 * buildFeed merges every source into one chronological, de-duplicated list,
 * newest first. De-duplication collapses identical events (the same lifecycle
 * transition or review reported by overlapping sources) so the timeline reads
 * cleanly.
 */
export function buildFeed(args: BuildFeedArgs): FeedItem[] {
  const { changesets, details = [], stories = [], conceptNames = {}, limit = 80 } = args;
  const raw: RawItem[] = [];

  for (const cs of changesets) {
    raw.push({
      at: cs.created_at,
      category: "experiment",
      kind: "changeset.opened",
      actor: cs.created_by,
      title: cs.name,
      detail: "Experiment opened",
      changesetId: cs.id,
    });
    if (cs.submitted_at) {
      raw.push({
        at: cs.submitted_at,
        category: "experiment",
        kind: "changeset.submitted",
        actor: cs.created_by,
        title: cs.name,
        detail: "Submitted for review",
        changesetId: cs.id,
      });
    }
    if (cs.merged_at) {
      raw.push({
        at: cs.merged_at,
        category: "experiment",
        kind: "changeset.merged",
        actor: cs.merged_by || cs.created_by,
        title: cs.name,
        detail: "Merged into the live graph",
        changesetId: cs.id,
      });
    }
    if (cs.status === "abandoned") {
      raw.push({
        at: cs.updated_at,
        category: "experiment",
        kind: "changeset.abandoned",
        actor: cs.created_by,
        title: cs.name,
        detail: "Abandoned",
        changesetId: cs.id,
      });
    }
  }

  for (const detail of details) {
    for (const review of detail.reviews) {
      raw.push({
        at: review.created_at,
        category: "experiment",
        kind: "changeset.reviewed",
        actor: review.reviewer,
        title: detail.name,
        detail: review.verdict === "approve" ? "Approved the experiment" : "Requested changes",
        changesetId: detail.id,
      });
    }
    for (const pilot of detail.pilots) {
      raw.push({
        at: pilot.created_at,
        category: "experiment",
        kind: "pilot.started",
        actor: pilot.created_by,
        title: detail.name,
        detail: `Piloting on ${pilot.project_id} · ${pilot.stream}`,
        changesetId: detail.id,
      });
    }
  }

  for (const story of stories) {
    const name = conceptNames[story.conceptId] ?? story.conceptId;
    for (const entry of story.entries) {
      if (entry.kind === "changeset") continue; // covered by change-set events
      const mapped = mapStoryEntry(entry, name, story.conceptId);
      if (mapped) raw.push(mapped);
    }
  }

  const seen = new Set<string>();
  const deduped: RawItem[] = [];
  for (const item of raw) {
    if (!item.at || Number.isNaN(new Date(item.at).getTime())) continue;
    const key = `${item.category}|${item.kind}|${item.conceptId ?? item.changesetId ?? ""}|${item.at}|${item.detail ?? ""}`;
    if (seen.has(key)) continue;
    seen.add(key);
    deduped.push(item);
  }

  deduped.sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());

  return deduped.slice(0, limit).map((item, i) => ({
    ...item,
    id: `${item.kind}:${item.conceptId ?? item.changesetId ?? "x"}:${item.at}:${i}`,
  }));
}

function mapStoryEntry(entry: ConceptStoryEntry, name: string, conceptId: string): RawItem | null {
  const base = { at: entry.at, actor: entry.actor, title: name, conceptId } as const;
  switch (entry.kind) {
    case "revision":
      return {
        ...base,
        category: "concept",
        kind: "concept.revision",
        detail: entry.summary || "Concept updated",
      };
    case "observation":
      return {
        ...base,
        category: "observation",
        kind: "observation",
        detail: entry.summary || "Observation recorded",
      };
    case "comment":
      return {
        ...base,
        category: "comment",
        kind: "comment",
        detail: entry.summary || "Comment added",
      };
    default:
      return null;
  }
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
  if (Number.isNaN(d.getTime())) return "—";
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
