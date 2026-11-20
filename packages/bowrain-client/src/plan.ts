// The review plan: what a project asks of its reviewers, ordered by the reach
// of each decision (GET /:ws/:id/review/plan). Coordinates first, then the
// context, then exceptions, then interfaces read in context, and last what
// the involvement in force leaves to the checks. The server counts; this file
// maps its answer.

import {
  planStepsFromServer,
  type PlanHeldReason,
  type PlanStep,
  type ServerPlanStep,
} from "./languages.ts";

/** How much a project's reviewers can be involved. */
export type ReviewInvolvement = "context" | "interfaces" | "every";

/** A level of the plan, in the order a reviewer works it. */
export type PlanLevel = "coordinates" | "context" | "exceptions" | "interfaces" | "policy";

/** One place in the project's context space: a collection's coordinates and its voice. */
export interface PlanPoint {
  collection: string;
  coordinates: Record<string, string>;
  voice?: string;
  /** The voice profile's id. */
  voiceId?: string;
  sourceOnly: boolean;
  priority?: string;
  /** The involvement at the point: the project's, or the override that applies there. */
  involvement?: ReviewInvolvement;
}

/**
 * The involvement a manager set at a point of the project's context space,
 * in place of the project's own. Of the overrides that apply to a
 * collection, the one naming the most axes wins; between two naming as many,
 * the deeper involvement wins.
 */
export interface InvolvementOverride {
  coordinates: Record<string, string>;
  involvement: ReviewInvolvement;
  /** The collections the override decides. */
  collections: number;
}

/** The project's involvement, its overrides, and the axes an override may name. */
export interface ReviewInvolvementSettings {
  involvement: ReviewInvolvement;
  overrides: InvolvementOverride[];
  /** Each axis the project's collections declare, with the values they hold. */
  axes: Record<string, string[]>;
}

/** A kind of change against the project's last confirmed coordinates. */
export type CoordinateChangeKind =
  | "point_added"
  | "point_removed"
  | "point_moved"
  | "voice_changed"
  | "source_only_changed"
  | "language_added"
  | "language_removed";

/**
 * One difference between the coordinates the team last confirmed and the
 * current ones. A point is named by its collection; a language change names
 * its locale.
 */
export interface CoordinateChange {
  kind: CoordinateChangeKind;
  collection?: string;
  /** The current coordinates, or the last confirmed ones for a removed point. */
  coordinates: Record<string, string>;
  previousCoordinates?: Record<string, string>;
  voice?: string;
  previousVoice?: string;
  sourceOnly: boolean;
  locale?: string;
}

/** The plan's coordinates: the points, who confirmed them, and what changed since. */
export interface PlanCoordinates {
  sourceLanguage: string;
  targetLanguages: string[];
  points: PlanPoint[];
  /** Set once a person has confirmed the coordinates. */
  confirmed: boolean;
  confirmedBy?: string;
  /** The display name of the person who confirmed. */
  confirmedByName?: string;
  confirmedAt?: string;
  /**
   * What differs from the coordinates the team last confirmed, whoever
   * confirmed them. While `changed` is above zero the step waits for the team
   * again; one confirmation settles it for everyone.
   */
  changes: CoordinateChange[];
  changed: number;
}

/**
 * One rule and the findings it raised. Most exceptions are one rule broken
 * many times, so a reviewer decides the rule before the blocks.
 */
export interface PlanRule {
  /** What the review deck's rule filter takes. */
  key: string;
  /** The check's rule id, such as `terms.vocabulary`. */
  rule: string;
  /** The term or the pattern the rule names. */
  subject?: string;
  /** One finding's wording, which names the rule to a person. */
  message?: string;
  /** The terms-store concept a word rule comes from. */
  conceptId?: string;
  fails: boolean;
  count: number;
}

/** The plan's part for one target language the caller may review. */
export interface PlanLanguage {
  locale: string;
  waiting: number;
  /** Translations held back by a failing check or a stale approval. */
  failing: number;
  /** Pages the involvement asks a person to read: the sample under Context. */
  pages: number;
  pagesMore: boolean;
  /** Every page with a translation waiting. */
  pagesTotal: number;
  /** Pages holding a translation with a failing finding. */
  pagesWithFindings: number;
  /** Translations that clear every bar and the involvement leaves to the checks. */
  settleable: number;
  /** Translations the deck holds back until the context is decided. */
  held: number;
}

/**
 * Whether the plan's last step may settle now: first_pass settles on the
 * checks alone once, policy once the context is decided, held waits for
 * the context after the first pass, none under the involvement every.
 */
export type PlanSettle = "first_pass" | "policy" | "held" | "none";

/**
 * Who the settle pass the server runs after each run is credited to: the
 * person who set the involvement, else the person who confirmed the
 * coordinates. `skip` says why the pass does not run: nobody to credit
 * (`no_one`), or that person no longer holds the review permission
 * (`no_permission`).
 */
export interface PlanAutoSettle {
  by?: string;
  byName?: string;
  source?: "involvement" | "coordinates";
  skip?: "no_one" | "no_permission";
}

/** The project's last settle pass, by hand or after a run. */
export interface PlanSettleResult {
  at: string;
  approved: number;
  by?: string;
  byName?: string;
  automatic: boolean;
}

/** A project scan for candidate terms, started by a push or from the plan. */
export interface PlanScan {
  id: string;
  status: "queued" | "processing" | "completed" | "failed";
  progress: number;
  createdAt: string;
  updatedAt: string;
}

/**
 * Why the term scan waits instead of running, while the context has not
 * started. A push starts the scan on its own once nothing holds it back.
 *
 * - waiting_content: the project holds no source text yet.
 * - waiting_push: it holds text and has had no scan; the next push starts one.
 * - waiting_quota: the workspace used its monthly AI limit, which resets at
 *   `quotaResetsAt`.
 * - waiting_credits: the workspace has no AI credits left.
 * - no_ai: the platform has no AI provider configured.
 */
export type PlanScanWaiting =
  | "waiting_content"
  | "waiting_push"
  | "waiting_quota"
  | "waiting_credits"
  | "no_ai";

export const PLAN_SCAN_WAITING: readonly PlanScanWaiting[] = [
  "waiting_content",
  "waiting_push",
  "waiting_quota",
  "waiting_credits",
  "no_ai",
];

/** A scan waiting reason the server sent, or undefined for one this client does not know. */
export function planScanWaitingOf(v: string | undefined): PlanScanWaiting | undefined {
  return (PLAN_SCAN_WAITING as readonly string[]).includes(v ?? "")
    ? (v as PlanScanWaiting)
    : undefined;
}

/**
 * The state of the job proposing target terms, as the context step shows it:
 * queued or processing while it runs, or, while concepts wait for a proposed
 * term and no job runs, why none starts: waiting_quota (the monthly AI limit,
 * which resets at `quotaResetsAt`), waiting_credits or no_ai.
 */
export type PlanProposals = "queued" | "processing" | "waiting_quota" | "waiting_credits" | "no_ai";

const PLAN_PROPOSALS: readonly PlanProposals[] = [
  "queued",
  "processing",
  "waiting_quota",
  "waiting_credits",
  "no_ai",
];

/** A proposals state the server sent, or undefined for one this client does not know. */
export function planProposalsOf(v: string | undefined): PlanProposals | undefined {
  return (PLAN_PROPOSALS as readonly string[]).includes(v ?? "") ? (v as PlanProposals) : undefined;
}

/**
 * Why a level the plan skips for `next` only waits: the context's term scan
 * runs (queued, processing), or it cannot start (waiting_content,
 * waiting_quota, waiting_credits, no_ai).
 */
export type PlanWaitReason =
  | "queued"
  | "processing"
  | "waiting_content"
  | "waiting_quota"
  | "waiting_credits"
  | "no_ai";

const PLAN_WAIT_REASONS: readonly PlanWaitReason[] = [
  "queued",
  "processing",
  "waiting_content",
  "waiting_quota",
  "waiting_credits",
  "no_ai",
];

/** A level with work ahead and nothing to decide yet, which `next` skips. */
export interface PlanWait {
  level: PlanLevel;
  reason: PlanWaitReason;
  /** When the workspace's monthly AI limit resets, set with waiting_quota. */
  quotaResetsAt?: string;
}

export interface ServerPlanWait {
  level: string;
  reason: string;
  quota_resets_at?: string;
}

/** The levels the plan skips for `next`, leaving out a level or reason this client does not know. */
export function planWaitingFromServer(waiting: ServerPlanWait[] | null | undefined): PlanWait[] {
  const out: PlanWait[] = [];
  for (const w of waiting ?? []) {
    if (!(LEVELS as readonly string[]).includes(w.level)) continue;
    if (!(PLAN_WAIT_REASONS as readonly string[]).includes(w.reason)) continue;
    out.push({
      level: w.level as PlanLevel,
      reason: w.reason as PlanWaitReason,
      quotaResetsAt: w.quota_resets_at || undefined,
    });
  }
  return out;
}

export interface ReviewPlan {
  stream: string;
  involvement: ReviewInvolvement;
  /** The involvement at points of the project's context space. */
  overrides: InvolvementOverride[];
  /** The first level the caller can act on, or null when none can be. */
  next: PlanLevel | null;
  /** The levels skipped for `next` because they only wait, with the reason. */
  waiting: PlanWait[];
  coordinates: PlanCoordinates;
  context: {
    /** Context cards waiting for the caller, by kind. */
    waiting: number;
    byKind: Record<string, number>;
    /** Concepts with a term in the source language. */
    concepts: number;
    /** Per target language, concepts with no usable term there. */
    untranslated: Record<string, number>;
    /** The last project scan for candidate terms, if one was started. */
    scan?: PlanScan;
    /** Why no scan runs, while the context has not started. */
    scanWaiting?: PlanScanWaiting;
    /** When the workspace's monthly AI limit resets, set with waiting_quota. */
    quotaResetsAt?: string;
    /** Concept and language pairs waiting for a proposed target term. */
    proposable: number;
    /** The last job the server started to propose target terms, if any. */
    proposals?: PlanScan;
    /** Why no proposal job runs while pairs wait. */
    proposalsWaiting?: Extract<PlanProposals, "waiting_quota" | "waiting_credits" | "no_ai">;
  };
  exceptions: {
    /** Findings in the source at the last project-wide check; null before one. */
    sourceFindings: number | null;
    checkedAt?: string;
    /** The rules behind the most findings, in the source and per target language. */
    byRule: { source: PlanRule[]; targets: Record<string, PlanRule[]> };
  };
  languages: PlanLanguage[];
  policy: {
    settle: PlanSettle;
    /** The project's open context decisions, for any reviewer. */
    contextOpen: number;
    firstPassAt?: string;
    /** Who the settle pass after each run is credited to, or why it does not run. */
    auto?: PlanAutoSettle;
    /** The last settle pass. */
    last?: PlanSettleResult;
  };
  /** Each level with its state, in order. */
  steps: PlanStep[];
  /** Set while the language decks hold translations back, with the reason. */
  held: PlanHeldReason | null;
  /** When the server took the plan's counts; they trail a change by a few seconds. */
  asOf?: string;
}

interface ServerPlanScan {
  id: string;
  status: string;
  progress: number;
  created_at: string;
  updated_at: string;
}

interface ServerPlanRule {
  key: string;
  rule: string;
  subject?: string;
  message?: string;
  concept_id?: string;
  fails?: boolean;
  count: number;
}

interface ServerCoordinateChange {
  kind: string;
  collection?: string;
  coordinates?: Record<string, string> | null;
  previous_coordinates?: Record<string, string> | null;
  voice?: string;
  previous_voice?: string;
  source_only?: boolean;
  locale?: string;
}

export interface ServerPlanCoordinates {
  source_language: string;
  target_languages?: string[] | null;
  points?:
    | {
        collection: string;
        coordinates?: Record<string, string> | null;
        voice?: string;
        voice_id?: string;
        source_only?: boolean;
        priority?: string;
        involvement?: string;
      }[]
    | null;
  confirmed: boolean;
  confirmed_by?: string;
  confirmed_by_name?: string;
  confirmed_at?: string;
  changes?: ServerCoordinateChange[] | null;
  changed?: number;
}

export interface ServerInvolvementOverride {
  coordinates?: Record<string, string> | null;
  involvement: string;
  collections?: number;
}

export interface ServerReviewInvolvement {
  involvement: string;
  overrides?: ServerInvolvementOverride[] | null;
  axes?: Record<string, string[] | null> | null;
}

export interface ServerReviewPlan {
  stream: string;
  involvement: string;
  overrides?: ServerInvolvementOverride[] | null;
  next: string;
  waiting?: ServerPlanWait[] | null;
  coordinates: ServerPlanCoordinates;
  context: {
    waiting: number;
    by_kind?: Record<string, number> | null;
    concepts: number;
    untranslated?: Record<string, number> | null;
    scan?: ServerPlanScan | null;
    scan_waiting?: string;
    quota_resets_at?: string;
    proposable?: number;
    proposals?: ServerPlanScan | null;
    proposals_waiting?: string;
  };
  exceptions: {
    source_findings?: number | null;
    checked_at?: string;
    by_rule?: {
      source?: ServerPlanRule[] | null;
      targets?: Record<string, ServerPlanRule[] | null> | null;
    } | null;
  };
  languages?:
    | {
        locale: string;
        waiting: number;
        failing: number;
        pages: number;
        pages_more?: boolean;
        pages_total?: number;
        pages_with_findings?: number;
        settleable: number;
        held?: number;
      }[]
    | null;
  policy?: {
    settle: string;
    context_open?: number;
    first_pass_at?: string;
    auto?: { by?: string; by_name?: string; source?: string; skip?: string } | null;
    last?: {
      at: string;
      approved: number;
      by?: string;
      by_name?: string;
      automatic?: boolean;
    } | null;
  } | null;
  steps?: ServerPlanStep[] | null;
  held?: string;
  as_of?: string;
}

const SETTLE: readonly PlanSettle[] = ["first_pass", "policy", "held", "none"];
const SCAN_STATUS: readonly PlanScan["status"][] = ["queued", "processing", "completed", "failed"];

export function planScanFromServer(s: ServerPlanScan): PlanScan {
  return {
    id: s.id,
    status: (SCAN_STATUS as readonly string[]).includes(s.status)
      ? (s.status as PlanScan["status"])
      : "failed",
    progress: s.progress,
    createdAt: s.created_at,
    updatedAt: s.updated_at,
  };
}

const LEVELS: readonly PlanLevel[] = [
  "coordinates",
  "context",
  "exceptions",
  "interfaces",
  "policy",
];

function involvementOf(v: string): ReviewInvolvement {
  return v === "interfaces" || v === "every" ? v : "context";
}

function overrideFromServer(o: ServerInvolvementOverride): InvolvementOverride {
  return {
    coordinates: o.coordinates ?? {},
    involvement: involvementOf(o.involvement),
    collections: o.collections ?? 0,
  };
}

export function reviewInvolvementFromServer(r: ServerReviewInvolvement): ReviewInvolvementSettings {
  return {
    involvement: involvementOf(r.involvement),
    overrides: (r.overrides ?? []).map(overrideFromServer),
    axes: Object.fromEntries(Object.entries(r.axes ?? {}).map(([axis, vs]) => [axis, vs ?? []])),
  };
}

/** The axes a point is read by, in order; the rest follow by name. */
const POINT_AXES = ["product", "channel", "brand"];

/** A point's axes in reading order: product, channel, brand, then the rest by name. */
function pointAxes(coords: Record<string, string>): string[] {
  const rank = (a: string) => {
    const i = POINT_AXES.indexOf(a);
    return i < 0 ? POINT_AXES.length : i;
  };
  return Object.keys(coords).sort((a, b) => rank(a) - rank(b) || a.localeCompare(b));
}

/** A point as a person reads it: its values in reading order, "app · ui". */
export function pointLabel(coords: Record<string, string>): string {
  return pointAxes(coords)
    .map((a) => coords[a])
    .join(" · ");
}

/** A key naming a point, the same for the same axes and values in any order. */
export function pointKey(coords: Record<string, string>): string {
  return Object.keys(coords)
    .sort()
    .map((a) => `${encodeURIComponent(a)}=${encodeURIComponent(coords[a])}`)
    .join(",");
}

/**
 * The points an override may be set at, from the plan's points: each
 * product, and each product with a channel, leaving out the ones that hold
 * an override and the source-only points.
 */
export function overridePoints(
  points: PlanPoint[],
  overrides: Pick<InvolvementOverride, "coordinates">[],
): Record<string, string>[] {
  const taken = new Set(overrides.map((o) => pointKey(o.coordinates)));
  const out: Record<string, string>[] = [];
  const add = (c: Record<string, string>) => {
    const key = pointKey(c);
    if (taken.has(key)) return;
    taken.add(key);
    out.push(c);
  };
  for (const pt of points) {
    if (pt.sourceOnly) continue;
    const { product, channel } = pt.coordinates;
    if (product) add({ product });
    if (product && channel) add({ product, channel });
  }
  return out.sort((a, b) => pointLabel(a).localeCompare(pointLabel(b)));
}

function ruleFromServer(r: ServerPlanRule): PlanRule {
  return {
    key: r.key,
    rule: r.rule,
    subject: r.subject || undefined,
    message: r.message || undefined,
    conceptId: r.concept_id || undefined,
    fails: r.fails ?? false,
    count: r.count,
  };
}

/** A rule's name for a person: its message, else its subject, else its id. */
export function planRuleLabel(r: PlanRule): string {
  return r.message || r.subject || r.rule;
}

const CHANGE_KINDS: readonly CoordinateChangeKind[] = [
  "point_added",
  "point_removed",
  "point_moved",
  "voice_changed",
  "source_only_changed",
  "language_added",
  "language_removed",
];

function isChangeKind(k: string): k is CoordinateChangeKind {
  return (CHANGE_KINDS as readonly string[]).includes(k);
}

export function planCoordinatesFromServer(c: ServerPlanCoordinates): PlanCoordinates {
  // A kind this client does not know is left out rather than drawn wrong;
  // the count stays the server's, so the step still reads as changed.
  const changes = (c.changes ?? []).flatMap((ch): CoordinateChange[] =>
    isChangeKind(ch.kind)
      ? [
          {
            kind: ch.kind,
            collection: ch.collection || undefined,
            coordinates: ch.coordinates ?? {},
            previousCoordinates: ch.previous_coordinates ?? undefined,
            voice: ch.voice || undefined,
            previousVoice: ch.previous_voice || undefined,
            sourceOnly: ch.source_only ?? false,
            locale: ch.locale || undefined,
          },
        ]
      : [],
  );
  return {
    sourceLanguage: c.source_language,
    targetLanguages: c.target_languages ?? [],
    points: (c.points ?? []).map((pt) => ({
      collection: pt.collection,
      coordinates: pt.coordinates ?? {},
      voice: pt.voice || undefined,
      voiceId: pt.voice_id || undefined,
      sourceOnly: pt.source_only ?? false,
      priority: pt.priority || undefined,
      involvement: pt.involvement ? involvementOf(pt.involvement) : undefined,
    })),
    confirmed: c.confirmed,
    confirmedBy: c.confirmed_by || undefined,
    confirmedByName: c.confirmed_by_name || undefined,
    confirmedAt: c.confirmed_at || undefined,
    changes,
    changed: c.changed ?? (c.changes ?? []).length,
  };
}

/** Whether the coordinates step waits for someone: never confirmed, or changed since. */
export function coordinatesOpen(c: PlanCoordinates): boolean {
  return !c.confirmed || c.changed > 0;
}

function autoSettleFromServer(a: {
  by?: string;
  by_name?: string;
  source?: string;
  skip?: string;
}): PlanAutoSettle {
  return {
    by: a.by || undefined,
    byName: a.by_name || undefined,
    source: a.source === "involvement" || a.source === "coordinates" ? a.source : undefined,
    skip: a.skip === "no_one" || a.skip === "no_permission" ? a.skip : undefined,
  };
}

function proposalsWaitingOf(v: string | undefined): ReviewPlan["context"]["proposalsWaiting"] {
  return v === "waiting_quota" || v === "waiting_credits" || v === "no_ai" ? v : undefined;
}

export function planFromServer(p: ServerReviewPlan): ReviewPlan {
  return {
    stream: p.stream,
    involvement: involvementOf(p.involvement),
    overrides: (p.overrides ?? []).map(overrideFromServer),
    next: (LEVELS as readonly string[]).includes(p.next) ? (p.next as PlanLevel) : null,
    waiting: planWaitingFromServer(p.waiting),
    coordinates: planCoordinatesFromServer(p.coordinates),
    context: {
      waiting: p.context.waiting,
      byKind: p.context.by_kind ?? {},
      concepts: p.context.concepts,
      untranslated: p.context.untranslated ?? {},
      scan: p.context.scan ? planScanFromServer(p.context.scan) : undefined,
      scanWaiting: planScanWaitingOf(p.context.scan_waiting),
      quotaResetsAt: p.context.quota_resets_at || undefined,
      proposable: p.context.proposable ?? 0,
      proposals: p.context.proposals ? planScanFromServer(p.context.proposals) : undefined,
      proposalsWaiting: proposalsWaitingOf(p.context.proposals_waiting),
    },
    exceptions: {
      sourceFindings: p.exceptions.source_findings ?? null,
      checkedAt: p.exceptions.checked_at || undefined,
      byRule: {
        source: (p.exceptions.by_rule?.source ?? []).map(ruleFromServer),
        targets: Object.fromEntries(
          Object.entries(p.exceptions.by_rule?.targets ?? {}).map(([locale, rules]) => [
            locale,
            (rules ?? []).map(ruleFromServer),
          ]),
        ),
      },
    },
    languages: (p.languages ?? []).map((l) => ({
      locale: l.locale,
      waiting: l.waiting,
      failing: l.failing,
      pages: l.pages,
      pagesMore: l.pages_more ?? false,
      pagesTotal: l.pages_total ?? l.pages,
      pagesWithFindings: l.pages_with_findings ?? 0,
      settleable: l.settleable,
      held: l.held ?? 0,
    })),
    policy: {
      settle: (SETTLE as readonly string[]).includes(p.policy?.settle ?? "")
        ? (p.policy!.settle as PlanSettle)
        : p.involvement === "every"
          ? "none"
          : "first_pass",
      contextOpen: p.policy?.context_open ?? 0,
      firstPassAt: p.policy?.first_pass_at || undefined,
      auto: p.policy?.auto ? autoSettleFromServer(p.policy.auto) : undefined,
      last: p.policy?.last
        ? {
            at: p.policy.last.at,
            approved: p.policy.last.approved,
            by: p.policy.last.by || undefined,
            byName: p.policy.last.by_name || undefined,
            automatic: p.policy.last.automatic ?? false,
          }
        : undefined,
    },
    steps: planStepsFromServer(p.steps),
    held: p.held === "context" ? "context" : null,
    asOf: p.as_of || undefined,
  };
}

/** The products a plan's points name, with their channels, in order. */
export function planProducts(points: PlanPoint[]): { product: string; channels: string[] }[] {
  const byProduct = new Map<string, Set<string>>();
  for (const pt of points) {
    const product = pt.coordinates.product;
    if (!product) continue;
    const channels = byProduct.get(product) ?? new Set<string>();
    if (pt.coordinates.channel) channels.add(pt.coordinates.channel);
    byProduct.set(product, channels);
  }
  return [...byProduct.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([product, channels]) => ({ product, channels: [...channels].sort() }));
}

/** One collection of a coordinates cell. */
export interface CoordinateCellCollection {
  name: string;
  coordinates: Record<string, string>;
  voice?: string;
  sourceOnly: boolean;
  blocks: number;
  /** Per target language, the share of the blocks established there (0 to 1). */
  coverage: Record<string, number>;
}

/** One product and channel of the coordinates map. */
export interface CoordinateCell {
  product: string;
  channel: string;
  collections: CoordinateCellCollection[];
  /** Translatable blocks of the cell's collections. */
  blocks: number;
  /** The voices bound at the cell, each once. */
  voices: string[];
  /** Every collection of the cell is source only. */
  sourceOnly: boolean;
  /** Per target language, the share of the cell's blocks established there (0 to 1). */
  coverage: Record<string, number>;
  /** The kinds of change touching the cell since the last confirmation. */
  changes: CoordinateChangeKind[];
}

/** The coordinates map (GET /:ws/:id/review/coordinates). */
export interface ReviewCoordinates {
  stream: string;
  coordinates: PlanCoordinates;
  cells: CoordinateCell[];
}

interface ServerCoordinateCollection {
  name: string;
  coordinates?: Record<string, string> | null;
  voice?: string;
  source_only?: boolean;
  blocks: number;
  coverage?: Record<string, number> | null;
}

export interface ServerReviewCoordinates {
  stream: string;
  coordinates: ServerPlanCoordinates;
  cells?:
    | {
        product: string;
        channel: string;
        collections?: ServerCoordinateCollection[] | null;
        blocks: number;
        voices?: string[] | null;
        source_only?: boolean;
        coverage?: Record<string, number> | null;
        changes?: string[] | null;
      }[]
    | null;
}

export function coordinatesFromServer(r: ServerReviewCoordinates): ReviewCoordinates {
  return {
    stream: r.stream,
    coordinates: planCoordinatesFromServer(r.coordinates),
    cells: (r.cells ?? []).map((c) => ({
      product: c.product,
      channel: c.channel,
      collections: (c.collections ?? []).map((col) => ({
        name: col.name,
        coordinates: col.coordinates ?? {},
        voice: col.voice || undefined,
        sourceOnly: col.source_only ?? false,
        blocks: col.blocks,
        coverage: col.coverage ?? {},
      })),
      blocks: c.blocks,
      voices: c.voices ?? [],
      sourceOnly: c.source_only ?? false,
      coverage: c.coverage ?? {},
      changes: (c.changes ?? []).filter(isChangeKind),
    })),
  };
}
