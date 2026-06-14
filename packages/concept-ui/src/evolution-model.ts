// The pure builder behind the concept evolution timeline (Apache-2.0). It takes
// the raw view-model a data source provides — a concept, its terms with validity
// windows, its direct relations, an optional platform revision log, and named
// markets — and projects it into an `EvolutionModel`: language lanes of validity
// spans, structural branches (a sibling locale appearing, a `directory → folder`
// rename), point milestones, a shared context track, and the density CLUSTERS
// that reduce a busy history to clouds while the signal stays discrete.
//
// No React, no I/O, no `Date.now()` — `now` is passed in — so the lane, branch,
// importance, and clustering rules are deterministic and unit-tested directly.

import { RELATION_LABEL, isBannedStatus, isPreferredStatus, primaryName } from "./concept-meta";
import type {
  BuildEvolutionOptions,
  EvolutionBranch,
  EvolutionCluster,
  EvolutionContextMarker,
  EvolutionExtent,
  EvolutionLane,
  EvolutionMilestone,
  EvolutionModel,
  EvolutionSpan,
  EvolutionTick,
  EvolutionTone,
  ExtentUnit,
  MilestoneKind,
  SpanCap,
} from "./evolution-types";
import { SIGNAL_IMPORTANCE, TONE_IMPORTANCE } from "./evolution-types";
import type { Concept, Market, Relation, TermStatus, TimelineEvent } from "./types";

// ── Inputs ───────────────────────────────────────────────────────────────────

export interface EvolutionInput {
  concept: Concept;
  /** The concept's direct relations (both directions). */
  relations?: Relation[];
  /** Display labels for neighbour concept ids (relation/rename targets). */
  neighbourLabels?: Record<string, string>;
  /** The platform revision log, when the source supplies one. */
  timeline?: TimelineEvent[];
  /** Named markets, for nicer context labels. */
  markets?: Market[];
}

const DEFAULTS = {
  maxFocusLanes: 3,
  clusterWindow: 0.05,
  includeDiscussion: true,
};

// ── Date helpers (pure) ──────────────────────────────────────────────────────

const t = (iso?: string | null): number => (iso ? Date.parse(iso) : NaN);
const valid = (iso?: string | null): iso is string => !Number.isNaN(t(iso));
const DAY = 86_400_000;

function minIso(values: Array<string | undefined | null>): string | undefined {
  let best: string | undefined;
  for (const v of values) {
    if (!valid(v)) continue;
    if (best === undefined || t(v) < t(best)) best = v;
  }
  return best;
}

function maxIso(values: Array<string | undefined | null>): string | undefined {
  let best: string | undefined;
  for (const v of values) {
    if (!valid(v)) continue;
    if (best === undefined || t(v) > t(best)) best = v;
  }
  return best;
}

// ── Status → tone / span cap ─────────────────────────────────────────────────

function spanCap(status: TermStatus, end: string | null, now: string): SpanCap {
  if (isBannedStatus(status)) return "banned";
  if (end === null) return "open";
  return t(end) <= t(now) ? "expired" : "bounded";
}

/** A span's own importance — drives which span labels survive a tight lane. */
function spanImportance(status: TermStatus): number {
  if (isPreferredStatus(status)) return TONE_IMPORTANCE.promote;
  if (isBannedStatus(status)) return TONE_IMPORTANCE.ban;
  return 35;
}

// ── Lanes (one per locale) ───────────────────────────────────────────────────

interface LaneDraft {
  locale: string;
  spans: EvolutionSpan[];
  milestones: EvolutionMilestone[];
  earliest: string; // earliest span start (real or inferred)
  earliestReal?: string; // earliest REAL validFrom (a genuine introduction)
  latest: string; // latest activity instant
  hasPreferred: boolean;
  hasGenesisSpan: boolean; // a span inferred-to/at genesis → lane existed at the start
}

function buildLaneDrafts(concept: Concept, genesis: string, now: string): Map<string, LaneDraft> {
  const lanes = new Map<string, LaneDraft>();
  for (const term of concept.terms) {
    const locale = term.locale;
    const realFrom = valid(term.validity?.validFrom) ? term.validity!.validFrom! : undefined;
    const start = realFrom ?? genesis;
    const end = valid(term.validity?.validTo) ? term.validity!.validTo! : null;
    const market = term.validity?.tags?.market;
    const span: EvolutionSpan = {
      id: `sp:${locale}:${market ?? ""}:${term.text}:${start}`,
      termText: term.text,
      locale,
      market,
      status: term.status,
      start,
      end,
      startInferred: realFrom === undefined,
      endInferred: false,
      cap: spanCap(term.status, end, now),
      importance: spanImportance(term.status),
    };
    let lane = lanes.get(locale);
    if (!lane) {
      lane = {
        locale,
        spans: [],
        milestones: [],
        earliest: start,
        earliestReal: realFrom,
        latest: maxIso([start, end ?? now]) ?? start,
        hasPreferred: false,
        hasGenesisSpan: false,
      };
      lanes.set(locale, lane);
    }
    lane.spans.push(span);
    if (t(start) < t(lane.earliest)) lane.earliest = start;
    // A span inferred (no real validFrom) and pinned at/near genesis means the
    // lane was already present when the concept began — it is an origin-era lane,
    // not a locale that branches in later.
    if (span.startInferred && t(start) <= t(genesis) + DAY) lane.hasGenesisSpan = true;
    if (realFrom && (!lane.earliestReal || t(realFrom) < t(lane.earliestReal))) {
      lane.earliestReal = realFrom;
    }
    const act = maxIso([lane.latest, start, end ?? undefined]);
    if (act) lane.latest = act;
    if (isPreferredStatus(term.status)) lane.hasPreferred = true;
  }
  for (const lane of lanes.values()) {
    lane.spans.sort((a, b) => t(a.start) - t(b.start) || a.termText.localeCompare(b.termText));
  }
  return lanes;
}

/** The lane present at genesis: earliest start, English preferred on ties. */
function originLocale(lanes: Map<string, LaneDraft>): string | undefined {
  let best: LaneDraft | undefined;
  for (const lane of lanes.values()) {
    if (!best) {
      best = lane;
      continue;
    }
    const dt = t(lane.earliest) - t(best.earliest);
    if (dt < 0) best = lane;
    else if (dt === 0) {
      const aEn = lane.locale.toLowerCase().startsWith("en");
      const bEn = best.locale.toLowerCase().startsWith("en");
      if (aEn && !bEn) best = lane;
      else if (aEn === bEn && lane.locale.localeCompare(best.locale) < 0) best = lane;
    }
  }
  return best?.locale;
}

// ── Within-lane renames (directory → folder) ─────────────────────────────────

/**
 * Detect a term supersession inside a lane: an older banned/expired term and a
 * newer blessed term of different text. Conservative — only the clearest case,
 * so a routine status tweak is not mis-read as a rename.
 */
function laneRename(lane: LaneDraft, now: string): EvolutionMilestone | undefined {
  const retired = [...lane.spans]
    .filter((s) => s.cap === "banned" || s.cap === "expired")
    .sort((a, b) => t(b.start) - t(a.start))[0];
  const current = lane.spans
    .filter((s) => !isBannedStatus(s.status) && (s.end === null || s.cap === "bounded"))
    .sort((a, b) => t(b.start) - t(a.start))[0];
  if (!retired || !current || retired.termText === current.termText) return undefined;
  // Require genuine temporal evidence of a handover: a real retirement date on
  // the old term, or a real start on the new one. Two undated terms (a legacy
  // forbidden label sitting beside the preferred one) are a constraints fact,
  // not a moment on the timeline.
  if (retired.end === null && current.startInferred) return undefined;
  // The successor must not pre-date the retired term, or the arrow reads
  // backwards (e.g. "folder → directory" when folder is the older wording).
  if (t(current.start) < t(retired.start)) return undefined;
  const at = maxIso([current.start, retired.end ?? retired.start]) ?? current.start;
  return {
    id: `ms:rename:${lane.locale}:${at}`,
    at: t(at) > t(now) ? now : at,
    tone: "rename",
    kind: "rename",
    importance: TONE_IMPORTANCE.rename,
    summary: `${retired.termText} → ${current.termText}`,
    detail: `Preferred wording in ${lane.locale} changed.`,
    laneKey: lane.locale,
  };
}

// ── Relations → branches + milestones ────────────────────────────────────────

function relationEvents(
  concept: Concept,
  relations: Relation[],
  label: (id: string) => string,
  now: string,
): { branches: EvolutionBranch[]; milestones: EvolutionMilestone[] } {
  const branches: EvolutionBranch[] = [];
  const milestones: EvolutionMilestone[] = [];
  for (const r of relations) {
    const touches = r.sourceId === concept.id || r.targetId === concept.id;
    if (!touches) continue;
    const outgoing = r.sourceId === concept.id;
    const otherId = outgoing ? r.targetId : r.sourceId;
    if (otherId === concept.id) continue;
    const other = label(otherId);
    const from = valid(r.validity?.validFrom) ? r.validity!.validFrom! : undefined;
    const to = valid(r.validity?.validTo) ? r.validity!.validTo! : undefined;
    const rel = RELATION_LABEL[r.type] ?? r.type.toLowerCase();

    if (r.type === "REPLACED_BY" && outgoing) {
      const at = from ?? concept.updatedAt ?? now;
      branches.push({
        id: `br:rename:${r.id}`,
        at: t(at) > t(now) ? now : at,
        kind: "rename",
        relationType: r.type,
        label: `renamed to ${other}`,
        navigateId: otherId,
        importance: TONE_IMPORTANCE.rename,
      });
      milestones.push({
        id: `ms:rename:${r.id}`,
        at: t(at) > t(now) ? now : at,
        tone: "rename",
        kind: "rename",
        importance: TONE_IMPORTANCE.rename,
        summary: `Renamed to ${other}`,
        ref: r.id,
        navigateId: otherId,
      });
      continue;
    }

    // Other typed relations only land on the timeline when they carry a date
    // (an undated relation is "always true" — it lives in the Relations panel),
    // and only once that date has arrived — a future validFrom/validTo has not
    // happened yet, so it is neither history nor allowed to stretch the axis.
    if (from && t(from) <= t(now)) {
      const tone: EvolutionTone = "relation";
      milestones.push({
        id: `ms:rel-on:${r.id}`,
        at: from,
        tone,
        kind: "relation",
        importance:
          r.type === "COMPETITOR" ? TONE_IMPORTANCE.relation + 10 : TONE_IMPORTANCE.relation,
        summary: `Linked ${rel} ${other}`,
        ref: r.id,
        navigateId: otherId,
      });
      branches.push({
        id: `br:rel-on:${r.id}`,
        at: from,
        kind: "relation-on",
        relationType: r.type,
        label: `${rel} ${other}`,
        navigateId: otherId,
        importance: TONE_IMPORTANCE.relation,
      });
    }
    if (to && t(to) <= t(now)) {
      milestones.push({
        id: `ms:rel-off:${r.id}`,
        at: to,
        tone: "relation",
        kind: "relation",
        importance: TONE_IMPORTANCE.relation,
        summary: `Unlinked ${rel} ${other}`,
        ref: r.id,
        navigateId: otherId,
      });
    }
  }
  return { branches, milestones };
}

// ── Rich revision log → milestones ───────────────────────────────────────────

const KIND_TONE: Record<string, EvolutionTone> = {
  create: "genesis",
  revision: "edit",
  relation: "relation",
  observation: "evidence",
  comment: "discussion",
  changeset: "governed",
};

/** Classify a `status`-kind revision into promote / ban / edit by its wording. */
function statusTone(summary: string): EvolutionTone {
  const s = summary.toLowerCase();
  if (/forbid|deprecat|ban|retire|prohibit/.test(s)) return "ban";
  if (/prefer|approv|bless|promot/.test(s)) return "promote";
  return "edit";
}

function timelineMilestones(events: TimelineEvent[], now: string): EvolutionMilestone[] {
  return events
    .filter((e) => valid(e.at))
    .map((e, i) => {
      const tone: EvolutionTone =
        e.kind === "status" ? statusTone(e.summary) : (KIND_TONE[e.kind] ?? "edit");
      const kind: MilestoneKind =
        e.kind === "status" && tone !== "edit" ? (tone as MilestoneKind) : e.kind;
      return {
        id: e.id ?? `ms:${e.kind}:${e.at}:${e.ref ?? i}`,
        at: t(e.at) > t(now) ? now : e.at,
        tone,
        kind,
        importance: TONE_IMPORTANCE[tone],
        summary: e.summary,
        detail: e.detail,
        actor: e.actor,
        ref: e.ref,
      } satisfies EvolutionMilestone;
    });
}

// ── Context track ────────────────────────────────────────────────────────────

function contextMarkers(
  lanes: Map<string, LaneDraft>,
  origin: string | undefined,
  genesis: string,
  markets: Market[],
  timeline: TimelineEvent[],
  now: string,
): EvolutionContextMarker[] {
  const out: EvolutionContextMarker[] = [];
  const localeName = (locale: string): string => {
    const m = markets.find((mk) =>
      mk.locales.some((l) => l.toLowerCase().startsWith(locale.toLowerCase())),
    );
    return m ? m.name : locale;
  };
  // A locale "opening": a non-origin lane whose first REAL validFrom is after
  // genesis — a genuine "the brand reached this market" moment.
  for (const lane of lanes.values()) {
    if (lane.locale === origin) continue;
    if (lane.hasGenesisSpan) continue;
    if (!lane.earliestReal) continue;
    if (t(lane.earliestReal) <= t(genesis) + DAY) continue;
    out.push({
      id: `cx:market:${lane.locale}:${lane.earliestReal}`,
      at: lane.earliestReal,
      kind: "market",
      label: `${localeName(lane.locale)} introduced`,
    });
  }
  // Governed change-sets that touched this concept.
  for (const e of timeline) {
    if (e.kind !== "changeset" || !valid(e.at)) continue;
    out.push({
      id: `cx:cs:${e.ref ?? e.at}`,
      at: t(e.at) > t(now) ? now : e.at,
      kind: "changeset",
      label: e.summary,
      ref: e.ref,
    });
  }
  return out.sort((a, b) => t(a.at) - t(b.at));
}

// ── Clustering (the clouds) ──────────────────────────────────────────────────

/**
 * Fold runs of BELOW-SIGNAL, time-close milestones into clusters. High-signal
 * milestones (≥ SIGNAL_IMPORTANCE) always pass through discrete. A run of one is
 * never clustered. `windowMs` is the proximity budget (a fraction of the extent).
 */
export function clusterMilestones(
  milestones: EvolutionMilestone[],
  windowMs: number,
  laneKey: string | undefined,
): { kept: EvolutionMilestone[]; clusters: EvolutionCluster[] } {
  const sorted = [...milestones].sort((a, b) => t(a.at) - t(b.at));
  const kept: EvolutionMilestone[] = [];
  const clusters: EvolutionCluster[] = [];
  let run: EvolutionMilestone[] = [];

  const flush = () => {
    if (run.length === 0) return;
    if (run.length === 1) {
      kept.push(run[0]);
    } else {
      const items = run;
      clusters.push({
        id: `cl:${laneKey ?? "global"}:${items[0].at}:${items.length}`,
        laneKey,
        start: items[0].at,
        end: items[items.length - 1].at,
        items,
        count: items.length,
      });
    }
    run = [];
  };

  for (const m of sorted) {
    if (m.importance >= SIGNAL_IMPORTANCE) {
      flush();
      kept.push(m);
      continue;
    }
    if (run.length === 0) {
      run.push(m);
      continue;
    }
    const near = t(m.at) - t(run[run.length - 1].at) <= windowMs;
    if (near) run.push(m);
    else {
      flush();
      run.push(m);
    }
  }
  flush();
  return { kept, clusters };
}

// ── Axis ticks ───────────────────────────────────────────────────────────────

function pickUnit(spanMs: number): ExtentUnit {
  const days = spanMs / DAY;
  if (days > 900) return "year";
  if (days > 270) return "quarter";
  if (days > 70) return "month";
  return "week";
}

function buildTicks(startMs: number, endMs: number, unit: ExtentUnit): EvolutionTick[] {
  const ticks: EvolutionTick[] = [];
  const start = new Date(startMs);
  const cur = new Date(
    Date.UTC(start.getUTCFullYear(), unit === "year" ? 0 : start.getUTCMonth(), 1),
  );
  // Snap to a unit boundary at or before start.
  if (unit === "quarter") cur.setUTCMonth(Math.floor(start.getUTCMonth() / 3) * 3, 1);
  if (unit === "week") {
    cur.setUTCFullYear(start.getUTCFullYear(), start.getUTCMonth(), start.getUTCDate());
    cur.setUTCDate(cur.getUTCDate() - cur.getUTCDay());
  }
  let guard = 0;
  while (cur.getTime() <= endMs && guard < 400) {
    guard += 1;
    const year = cur.getUTCFullYear();
    const month = cur.getUTCMonth();
    let label = "";
    let major = false;
    if (unit === "year") {
      label = String(year);
      major = true;
    } else if (unit === "quarter") {
      label = month === 0 ? String(year) : `Q${Math.floor(month / 3) + 1}`;
      major = month === 0;
    } else if (unit === "month") {
      label = cur.toLocaleDateString(undefined, { month: "short", timeZone: "UTC" });
      major = month === 0;
      if (major) label = String(year);
    } else {
      label = cur.toLocaleDateString(undefined, {
        month: "short",
        day: "numeric",
        timeZone: "UTC",
      });
      major = cur.getUTCDate() <= 7;
    }
    if (cur.getTime() >= startMs - 1) ticks.push({ at: cur.toISOString(), label, major });
    if (unit === "year") cur.setUTCFullYear(year + 1);
    else if (unit === "quarter") cur.setUTCMonth(month + 3);
    else if (unit === "month") cur.setUTCMonth(month + 1);
    else cur.setUTCDate(cur.getUTCDate() + 7);
  }
  return ticks;
}

// ── Focus selection ──────────────────────────────────────────────────────────

function selectFocus(
  lanes: EvolutionLane[],
  origin: string | undefined,
  focusLocales: string[] | undefined,
  maxFocus: number,
): void {
  if (lanes.length <= maxFocus) {
    for (const l of lanes) l.focused = true;
    return;
  }
  if (focusLocales && focusLocales.length > 0) {
    const want = new Set(focusLocales.map((l) => l.toLowerCase()));
    let any = false;
    for (const l of lanes) {
      l.focused = want.has(l.locale.toLowerCase());
      any = any || l.focused;
    }
    if (any) return;
  }
  // Default: the origin lane plus the highest-importance lanes, to the cap.
  const order = [...lanes].sort((a, b) => {
    if (a.locale === origin) return -1;
    if (b.locale === origin) return 1;
    return b.importance - a.importance;
  });
  const focus = new Set(order.slice(0, maxFocus).map((l) => l.key));
  for (const l of lanes) l.focused = focus.has(l.key);
}

// ── Build ────────────────────────────────────────────────────────────────────

export function buildEvolutionModel(
  input: EvolutionInput,
  opts: BuildEvolutionOptions,
): EvolutionModel {
  const { concept } = input;
  const relations = input.relations ?? [];
  const timeline = input.timeline ?? [];
  const markets = input.markets ?? [];
  const maxFocus = opts.maxFocusLanes ?? DEFAULTS.maxFocusLanes;
  const clusterWindow = opts.clusterWindow ?? DEFAULTS.clusterWindow;
  const includeDiscussion = opts.includeDiscussion ?? DEFAULTS.includeDiscussion;
  const labelFor = (id: string): string => input.neighbourLabels?.[id] ?? id;

  // Genesis: explicit createdAt, else the earliest dated thing we can see.
  // `realGenesis` is undefined when the concept carries NO temporal data at all
  // (no createdAt, no term validity, no revision log) — an undated concept has
  // terms but no *history*, so the timeline reads as empty rather than charting
  // flat bars pinned at "now". `genesis` still gives spans an inferred start.
  const earliestTermStart = minIso(concept.terms.map((t2) => t2.validity?.validFrom));
  const realGenesis = concept.createdAt ?? earliestTermStart ?? minIso(timeline.map((e) => e.at));
  // Sanitise `now`: an unparseable value would make the extent math throw a
  // RangeError, so fall back deterministically (latest known, else genesis,
  // else the epoch). Computed after realGenesis since the fallback uses it.
  const now = valid(opts.now)
    ? opts.now
    : (maxIso([realGenesis, earliestTermStart, ...timeline.map((e) => e.at)]) ??
      realGenesis ??
      "1970-01-01T00:00:00.000Z");
  const genesis = realGenesis ?? now;

  const laneDrafts = buildLaneDrafts(concept, genesis, now);
  const origin = originLocale(laneDrafts);

  // Lane-scoped milestones: sibling births + within-lane renames.
  for (const lane of laneDrafts.values()) {
    if (
      lane.locale !== origin &&
      !lane.hasGenesisSpan &&
      lane.earliestReal &&
      t(lane.earliestReal) > t(genesis) + DAY
    ) {
      lane.milestones.push({
        id: `ms:sibling:${lane.locale}:${lane.earliestReal}`,
        at: lane.earliestReal,
        tone: "sibling",
        kind: "sibling",
        importance: TONE_IMPORTANCE.sibling,
        summary: `${lane.locale} added`,
        detail: `A ${lane.locale} term joined the concept.`,
        laneKey: lane.locale,
      });
    }
    const rename = laneRename(lane, now);
    if (rename) lane.milestones.push(rename);
  }

  // Relations → branches + milestones.
  const relOut = relationEvents(concept, relations, labelFor, now);

  // Sibling branches (origin lane → new lane).
  const siblingBranches: EvolutionBranch[] = [];
  for (const lane of laneDrafts.values()) {
    if (
      lane.locale !== origin &&
      !lane.hasGenesisSpan &&
      lane.earliestReal &&
      t(lane.earliestReal) > t(genesis) + DAY
    ) {
      siblingBranches.push({
        id: `br:sibling:${lane.locale}:${lane.earliestReal}`,
        at: lane.earliestReal,
        kind: "sibling",
        fromLaneKey: origin,
        toLaneKey: lane.locale,
        label: `${lane.locale} sibling`,
        importance: TONE_IMPORTANCE.sibling,
      });
    }
  }

  // Global spine milestones: the rich revision log if present, else a synthesised
  // genesis. Discussion (observation/comment) only when asked for.
  const rich = timelineMilestones(timeline, now).filter(
    (m) => includeDiscussion || (m.tone !== "evidence" && m.tone !== "discussion"),
  );
  const globalRaw: EvolutionMilestone[] = [];
  if (rich.length > 0) {
    globalRaw.push(...rich);
  } else if (realGenesis) {
    globalRaw.push({
      id: `ms:create:${realGenesis}`,
      at: realGenesis,
      tone: "genesis",
      kind: "create",
      importance: TONE_IMPORTANCE.genesis,
      summary: "Concept created",
    });
  }
  globalRaw.push(...relOut.milestones);

  // Extent.
  const allDates = [
    genesis,
    now,
    ...[...laneDrafts.values()].flatMap((l) => [l.earliest, l.latest]),
    ...globalRaw.map((m) => m.at),
    ...relOut.branches.map((b) => b.at),
    ...siblingBranches.map((b) => b.at),
  ];
  const startMs = t(minIso(allDates) ?? genesis);
  const endMs = Math.max(t(now), t(maxIso(allDates) ?? now));
  const unit = pickUnit(Math.max(endMs - startMs, DAY));
  const extent: EvolutionExtent = {
    start: new Date(startMs).toISOString(),
    end: new Date(endMs).toISOString(),
    unit,
    ticks: buildTicks(startMs, endMs, unit),
  };
  const windowMs = Math.max((endMs - startMs) * clusterWindow, DAY);

  // Cluster lane + global milestones.
  const lanes: EvolutionLane[] = [...laneDrafts.values()].map((draft) => {
    const { kept, clusters } = clusterMilestones(draft.milestones, windowMs, draft.locale);
    const market = draft.spans.find((s) => s.market)?.market;
    return {
      key: draft.locale,
      locale: draft.locale,
      market,
      label: draft.locale,
      focused: false,
      importance:
        (t(draft.latest) - startMs) / Math.max(endMs - startMs, 1) +
        (draft.hasPreferred ? 1 : 0) +
        (draft.locale === origin ? 0.5 : 0),
      spans: draft.spans,
      milestones: kept,
      clusters,
    } satisfies EvolutionLane;
  });
  lanes.sort((a, b) => b.importance - a.importance || a.locale.localeCompare(b.locale));

  const { kept: globalKept, clusters: globalClusters } = clusterMilestones(
    globalRaw,
    windowMs,
    undefined,
  );

  selectFocus(lanes, origin, opts.focusLocales, maxFocus);
  // Stable display order: focused first (by importance), then the rest.
  lanes.sort((a, b) => {
    if (a.focused !== b.focused) return a.focused ? -1 : 1;
    return b.importance - a.importance || a.locale.localeCompare(b.locale);
  });

  const context = contextMarkers(laneDrafts, origin, genesis, markets, timeline, now);

  // "History" counts only DATED signal: real milestones, branches, context, and
  // spans with a genuine start/end. Flat inferred-at-genesis bars (an undated
  // concept's terms) are not history and must not defeat the empty state.
  const datedSpans = lanes.reduce(
    (n, l) => n + l.spans.filter((s) => !s.startInferred || s.end !== null).length,
    0,
  );
  const eventCount =
    globalKept.length +
    globalClusters.reduce((n, c) => n + c.count, 0) +
    lanes.reduce(
      (n, l) => n + l.milestones.length + l.clusters.reduce((m, c) => m + c.count, 0),
      0,
    ) +
    datedSpans +
    relOut.branches.length +
    siblingBranches.length +
    context.length;

  return {
    extent,
    lanes,
    focusedLanes: lanes.filter((l) => l.focused),
    moreLanes: lanes.filter((l) => !l.focused),
    branches: [...siblingBranches, ...relOut.branches].sort((a, b) => t(a.at) - t(b.at)),
    milestones: globalKept,
    globalClusters,
    context,
    derived: rich.length === 0,
    eventCount,
  };
}

/** Convenience: a display name for the concept (re-exported for renderers). */
export function conceptDisplayName(concept: Concept): string {
  return primaryName(concept);
}
