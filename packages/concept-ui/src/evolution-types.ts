// The derived model behind the concept EVOLUTION timeline (Apache-2.0). Where
// `./types` holds the raw view-model a data source provides (terms, relations,
// validity, an optional revision log), this module is the SHAPE those inputs are
// projected into for the timeline: language lanes of validity spans, structural
// branches (a sibling locale appearing, a `directory → folder` rename), point
// milestones, a shared context track, and the density CLUSTERS that let a busy
// history reduce to "clouds" of the unimportant while the signal stays discrete.
//
// It is the contract the pure builder (`./evolution-model`) writes and the two
// renderers (`EvolutionRoadmap`, `EvolutionGraph`) read — locked here so they
// can be developed and tested independently.

import type { RelationType, TermStatus, TimelineKind } from "./types";

// ── Tone & importance ────────────────────────────────────────────────────────

/**
 * The coarse family an evolution event belongs to. Tone drives BOTH the icon /
 * accent a renderer uses AND the default importance — the high-signal tones
 * (genesis, rename, promote, ban, sibling, governed) survive decluttering; the
 * low-signal ones (edit, evidence, discussion) are the first to fold into a
 * cluster cloud when the timeline gets dense.
 */
export type EvolutionTone =
  | "genesis" // the concept came into being
  | "rename" // a term superseded another / a REPLACED_BY landed (highest signal)
  | "promote" // a term became preferred
  | "ban" // a term became forbidden/deprecated — "stop using after X"
  | "sibling" // a new locale/market term first appeared (a branch)
  | "relation" // a typed relation was gained or lost (competitor, use-instead…)
  | "governed" // a governed change-set merged
  | "edit" // a routine revision / minor change
  | "evidence" // an external observation
  | "discussion"; // a comment

/** The canonical importance weight for each tone (higher = more signal). */
export const TONE_IMPORTANCE: Record<EvolutionTone, number> = {
  rename: 100,
  genesis: 90,
  ban: 80,
  promote: 70,
  sibling: 65,
  governed: 60,
  relation: 40,
  edit: 20,
  evidence: 15,
  discussion: 10,
};

/** Default cut: events at or above this importance never fold into a cluster. */
export const SIGNAL_IMPORTANCE = 50;

// ── Validity spans (the bars in a lane) ──────────────────────────────────────

/** How a validity span ends — drives the bar's terminal cap. */
export type SpanCap =
  | "open" // no validTo: the term is still current → arrow to now
  | "bounded" // a plain validTo in the future-of-its-start, not a ban
  | "expired" // validTo in the past, term not banned
  | "banned"; // the term is forbidden/deprecated — a hard stop

/** One term's life as a bar within its language lane. */
export interface EvolutionSpan {
  id: string;
  termText: string;
  locale: string;
  market?: string;
  status: TermStatus;
  /** ISO start — the term's validFrom, else an inferred lane/concept start. */
  start: string;
  /** ISO end — the term's validTo, or null when open-ended (runs to now). */
  end: string | null;
  /** True when `start` was inferred (no real validFrom on the term). */
  startInferred: boolean;
  /** True when `end` was inferred. */
  endInferred: boolean;
  cap: SpanCap;
  importance: number;
}

// ── Point milestones ─────────────────────────────────────────────────────────

/** A discriminated milestone kind (extends the raw TimelineKind with derived). */
export type MilestoneKind = TimelineKind | "sibling" | "rename" | "promote" | "ban";

/** A single dated point on the timeline — global, or scoped to one lane. */
export interface EvolutionMilestone {
  id: string;
  at: string;
  tone: EvolutionTone;
  kind: MilestoneKind;
  importance: number;
  summary: string;
  detail?: string;
  actor?: string;
  /** An opaque reference (revision number, change-set id, relation id…). */
  ref?: string;
  /** The lane (locale key) this belongs to; undefined = global spine. */
  laneKey?: string;
  /** A concept to re-centre on when activated (rename / relation target). */
  navigateId?: string;
}

// ── Structural branches ──────────────────────────────────────────────────────

/** A fork drawn between lanes, or out to another concept. */
export interface EvolutionBranch {
  id: string;
  at: string;
  kind: "sibling" | "rename" | "relation-on" | "relation-off";
  /** Lane the branch leaves from (the parent), or undefined = the spine. */
  fromLaneKey?: string;
  /** Lane the branch lands in (a sibling birth), or undefined. */
  toLaneKey?: string;
  relationType?: RelationType;
  label: string;
  /** A concept to re-centre on (rename / relation target). */
  navigateId?: string;
  importance: number;
}

// ── Clusters (the "clouds") ──────────────────────────────────────────────────

/**
 * A collapsed run of low-importance, time-close events. The renderers draw it as
 * one "N changes" cloud that expands to reveal its items, so a dense stretch of
 * routine edits never drowns the milestones that matter.
 */
export interface EvolutionCluster {
  id: string;
  /** Lane the cluster belongs to; undefined = global spine. */
  laneKey?: string;
  /** ISO span the folded events cover. */
  start: string;
  end: string;
  /** The folded milestones, ascending by time. */
  items: EvolutionMilestone[];
  count: number;
}

// ── Lanes ────────────────────────────────────────────────────────────────────

/** A language/market lane — one term lineage's life over time. */
export interface EvolutionLane {
  /** Stable key: `locale` or `locale|market`. */
  key: string;
  locale: string;
  market?: string;
  /** Display label, e.g. `en` or `nb · nordics`. */
  label: string;
  /** Whether this lane is shown expanded vs folded into the "N more" band. */
  focused: boolean;
  /** Ordering / focus-selection weight (recency + status blessing). */
  importance: number;
  spans: EvolutionSpan[];
  /** Lane-scoped milestones AFTER clustering (signal points only). */
  milestones: EvolutionMilestone[];
  clusters: EvolutionCluster[];
}

// ── Shared context track ─────────────────────────────────────────────────────

/** A reference marker on the shared context track behind the lanes. */
export interface EvolutionContextMarker {
  id: string;
  at: string;
  kind: "market" | "changeset";
  label: string;
  ref?: string;
}

// ── Time axis ────────────────────────────────────────────────────────────────

export type ExtentUnit = "year" | "quarter" | "month" | "week";

/** One axis tick. */
export interface EvolutionTick {
  at: string;
  label: string;
  major: boolean;
}

/** The overall time window the timeline spans, plus its axis ticks. */
export interface EvolutionExtent {
  start: string;
  end: string;
  unit: ExtentUnit;
  ticks: EvolutionTick[];
}

// ── The model ────────────────────────────────────────────────────────────────

/** The fully-derived evolution model both renderers consume. */
export interface EvolutionModel {
  extent: EvolutionExtent;
  /** Every lane, focused ones first. */
  lanes: EvolutionLane[];
  /** Convenience partitions of `lanes`. */
  focusedLanes: EvolutionLane[];
  /** Lanes folded into the "N more languages" band (awareness without noise). */
  moreLanes: EvolutionLane[];
  branches: EvolutionBranch[];
  /** Global (spine) milestones after clustering. */
  milestones: EvolutionMilestone[];
  /** Global clusters (folded low-signal spine events). */
  globalClusters: EvolutionCluster[];
  context: EvolutionContextMarker[];
  /** True when synthesised from validity windows (no platform revision log). */
  derived: boolean;
  /** Total events considered — drives the empty / sparse-degradation decision. */
  eventCount: number;
}

// ── Builder options ──────────────────────────────────────────────────────────

export interface BuildEvolutionOptions {
  /** ISO "now" — the open end of open-ended spans and the axis end. */
  now: string;
  /** Locales the host wants kept in focus (e.g. the user's working languages). */
  focusLocales?: string[];
  /** Max lanes shown expanded before the rest fold into "N more". Default 3. */
  maxFocusLanes?: number;
  /**
   * Declutter aggressiveness as a FRACTION of the total extent: low-importance
   * events whose neighbours fall within this window fold together. Larger =
   * more clustering (use for narrow widths). Default 0.05 (5% of the span).
   */
  clusterWindow?: number;
  /** Include observations/comments as low-signal events. Default true. */
  includeDiscussion?: boolean;
}
