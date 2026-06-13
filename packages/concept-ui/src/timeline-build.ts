// Higher-level timeline transforms behind the ConceptTimeline panel
// (Apache-2.0). Where `./timeline` carries the primitives (sort, day-bucket, the
// minimal synthesise), this module adds what the panel needs to tell a real
// product story:
//
//   • synthesizeCoreTimeline — the RICHER framework-only fallback. A local
//     termbase has no revision log, but it does know term validity windows and
//     relation validity windows, so we can still surface status transitions
//     (term valid / retired), relations gained / lost, plus create / update.
//   • resolveTimelineEvents — pick the platform's revision log when present, else
//     fall back to the synthesised core (the documented degradation rule).
//   • buildDisplayTimeline — sort, bucket by day, and attach per-event display
//     metadata (a tone + a stable id) so the panel renders icons and keys
//     without re-deriving anything.
//
// No React here so the merge, ordering, and tone rules are unit-tested directly.

import { RELATION_LABEL, isBannedStatus } from "./concept-meta";
import { buildTimeline } from "./timeline";
import type { Concept, Relation, TimelineEvent, TimelineKind } from "./types";

// ── Tone (drives the per-event icon + accent in the panel) ───────────────────

/** A coarse visual family for a timeline event — maps to an icon and accent. */
export type TimelineTone =
  | "genesis"
  | "edit"
  | "status"
  | "relation"
  | "evidence"
  | "discussion"
  | "governed";

export interface TimelineKindMeta {
  tone: TimelineTone;
  /** A short noun for the kind, e.g. for an aria-label. */
  label: string;
}

/** The tone + label each timeline kind renders with. */
export const TIMELINE_KIND_META: Record<TimelineKind, TimelineKindMeta> = {
  create: { tone: "genesis", label: "Created" },
  revision: { tone: "edit", label: "Revised" },
  status: { tone: "status", label: "Status change" },
  relation: { tone: "relation", label: "Relation" },
  observation: { tone: "evidence", label: "Observation" },
  comment: { tone: "discussion", label: "Comment" },
  changeset: { tone: "governed", label: "Change-set" },
};

/** Tone + label for a kind, with a safe fallback for unknown kinds. */
export function timelineKindMeta(kind: TimelineKind): TimelineKindMeta {
  return TIMELINE_KIND_META[kind] ?? { tone: "edit", label: "Event" };
}

// ── Core synthesis (the framework-only fallback) ─────────────────────────────

export interface CoreTimelineOptions {
  /** Resolve a neighbour concept id to a display label for relation events. */
  labelFor?: (conceptId: string) => string;
}

/**
 * Synthesise a richer CORE timeline from a concept and its direct relations, for
 * sources with no revision log. Beyond create/update it derives, from validity
 * windows the local termbase already stores:
 *
 *   • a term becoming valid (`validFrom`) and a term being retired/expiring
 *     (`validTo`, phrased by whether the term is banned);
 *   • a relation being gained (`validFrom`) and lost (`validTo`).
 *
 * Returned UNSORTED — pass through buildDisplayTimeline. Deterministic and
 * deduplicated. Empty when the concept tracks no timestamps or windows.
 */
export function synthesizeCoreTimeline(
  concept: Concept,
  relations: Relation[] = [],
  opts: CoreTimelineOptions = {},
): TimelineEvent[] {
  const label = opts.labelFor ?? ((id: string) => id);
  const events: TimelineEvent[] = [];

  if (concept.createdAt) {
    events.push({ kind: "create", at: concept.createdAt, summary: "Concept created" });
  }
  if (concept.updatedAt && concept.updatedAt !== concept.createdAt) {
    events.push({ kind: "revision", at: concept.updatedAt, summary: "Concept updated" });
  }

  const seen = new Set<string>();
  const once = (key: string): boolean => {
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  };

  // Term lifecycle from validity windows.
  for (const term of concept.terms) {
    const from = term.validity?.validFrom;
    const to = term.validity?.validTo;
    const market = term.validity?.tags?.market;
    const scope = market ? ` · ${market}` : "";
    if (from && once(`from:${from}:${term.locale}:${term.text}`)) {
      events.push({
        kind: "status",
        at: from,
        summary: `${term.text} valid in ${term.locale}${scope}`,
        ref: term.locale,
      });
    }
    if (to && once(`to:${to}:${term.locale}:${term.text}`)) {
      const verb = isBannedStatus(term.status) ? "retired" : "expired";
      events.push({
        kind: "status",
        at: to,
        summary: `${term.text} ${verb} in ${term.locale}${scope}`,
        ref: term.locale,
      });
    }
  }

  // Relations gained / lost from relation validity windows.
  for (const r of relations) {
    const touches = r.sourceId === concept.id || r.targetId === concept.id;
    if (!touches) continue;
    const otherId = r.sourceId === concept.id ? r.targetId : r.sourceId;
    if (otherId === concept.id) continue;
    const rel = RELATION_LABEL[r.type] ?? r.type.toLowerCase();
    const from = r.validity?.validFrom;
    const to = r.validity?.validTo;
    if (from && once(`rfrom:${from}:${r.id}`)) {
      events.push({
        kind: "relation",
        at: from,
        summary: `Linked ${rel} ${label(otherId)}`,
        ref: r.id,
      });
    }
    if (to && once(`rto:${to}:${r.id}`)) {
      events.push({
        kind: "relation",
        at: to,
        summary: `Unlinked ${rel} ${label(otherId)}`,
        ref: r.id,
      });
    }
  }

  return events;
}

// ── Resolve (rich wins, else core) ───────────────────────────────────────────

export interface ResolveTimelineOptions extends CoreTimelineOptions {
  /** The platform's revision log, when the source supplies one. */
  remote?: TimelineEvent[];
  /** The concept's direct relations, for the core relation events. */
  relations?: Relation[];
}

/**
 * The events the panel should render: the platform revision log when it has any
 * entries, otherwise the synthesised framework-only core. Mirrors the documented
 * degradation rule in one tested place.
 */
export function resolveTimelineEvents(
  concept: Concept,
  opts: ResolveTimelineOptions = {},
): TimelineEvent[] {
  const remote = opts.remote ?? [];
  if (remote.length > 0) return remote;
  return synthesizeCoreTimeline(concept, opts.relations ?? [], { labelFor: opts.labelFor });
}

// ── Display building (sort + day-bucket + tone + stable id) ───────────────────

export interface TimelineDisplayEvent {
  /** Stable, unique key for React lists. */
  id: string;
  kind: TimelineKind;
  tone: TimelineTone;
  at: string;
  actor?: string;
  summary: string;
  detail?: string;
  ref?: string;
}

export interface TimelineDisplayDay {
  key: string;
  label: string;
  events: TimelineDisplayEvent[];
}

/**
 * Sort events, bucket them by calendar day, and attach display metadata: a tone
 * (for icon + accent) and a stable id (the event's own id, or a deterministic
 * composite). `order` controls direction ("desc" = latest first).
 */
export function buildDisplayTimeline(
  events: TimelineEvent[],
  order: "asc" | "desc" = "desc",
): TimelineDisplayDay[] {
  const days = buildTimeline(events, order);
  let n = 0;
  return days.map((day) => ({
    key: day.key,
    label: day.label,
    events: day.events.map((e) => {
      const id = e.id ?? `${e.kind}-${e.at}-${e.ref ?? ""}-${n}`;
      n += 1;
      return {
        id,
        kind: e.kind,
        tone: timelineKindMeta(e.kind).tone,
        at: e.at,
        actor: e.actor,
        summary: e.summary,
        detail: e.detail,
        ref: e.ref,
      };
    }),
  }));
}
