// The one query every view of the atlas reads: a window, a snapshot, the
// retained filters (OR within an axis, AND across axes) and how the view is
// arranged. It is plain JSON, so a link can carry it and an investigation
// record can embed it.

import { DERIVED_AXES, type Atlas, type Measure, geoAxis } from "./atlas";
import { NEEDS_PERSON } from "./state";
import { addDays, clampDay, isDay, maxDay, minDay } from "./time";

export const ATLAS_VIEWS = ["flow", "matrix", "time", "map", "records"] as const;
export type AtlasView = (typeof ATLAS_VIEWS)[number];

export const FLOW_LENSES = ["origin", "attention", "change"] as const;
/**
 * What colour says in the flow: the first column's category (`origin`), the
 * share waiting for a person (`attention`), or how that share moved between
 * the window's start and the snapshot (`change`).
 */
export type FlowLens = (typeof FLOW_LENSES)[number];

export const EVENT_CLOCKS = ["recorded", "effective"] as const;
/** Which instant positions an event: when it was recorded, or when it takes effect. */
export type EventClock = (typeof EVENT_CLOCKS)[number];

export const MEASURES = ["records", "units"] as const satisfies readonly Measure[];

export const MIN_FLOW_AXES = 2;
export const MAX_FLOW_AXES = 5;
export const HISTORY_LIMIT = 60;

export interface AtlasQuery {
  /** First day of the window, inclusive. */
  from: string;
  /** Last day of the window, inclusive. */
  to: string;
  /** The day whose end the snapshot reads. Equals `to` while `follow` holds. */
  asOf: string;
  /** Whether the snapshot follows the window's end. */
  follow: boolean;
  /** The clock events are placed on. States always resolve on the recorded clock. */
  clock: EventClock;
  /** Axis id to the values retained on it. Empty or missing means unrestricted. */
  filters: Record<string, string[]>;
  /** An explicit set of record ids; empty means unrestricted. */
  ids: string[];
  view: AtlasView;
  /** The flow's columns, left to right. */
  flowAxes: string[];
  lens: FlowLens;
  /** Whether the flow draws the surrounding context behind the selection. */
  surrounding: boolean;
  matrixRows: string;
  matrixCols: string;
  /** The rows of the over-time raster. */
  rasterAxis: string;
  /**
   * What a count counts: records, one each, or units, where a content cell
   * weighs its unit count.
   */
  measure: Measure;
}

/** The days a preset window covers, counted back from the data's last day. */
export const WINDOW_PRESETS = [7, 30, 90] as const;

const FLOW_CANDIDATES = ["family", "product", "channel", "brand", "market", "region", "locale"];

function usable(atlas: Atlas, id: string): boolean {
  const axis = atlas.axisById.get(id);
  return !!axis && axis.origin !== "view" && axis.values.length > 0;
}

function dataAxesBy(atlas: Atlas, candidates: string[]): string[] {
  const named = candidates.filter((id) => usable(atlas, id));
  const rest = atlas.axes
    .filter((a) => a.origin !== "view" && a.values.length > 1 && !named.includes(a.id))
    .map((a) => a.id);
  return [...named, ...rest];
}

/** The columns a flow opens with: up to three data axes, then the state at the snapshot. */
export function defaultFlowAxes(atlas: Atlas): string[] {
  const data = dataAxesBy(atlas, FLOW_CANDIDATES).slice(0, 3);
  const axes = [...data, DERIVED_AXES.statusSnapshot].filter((id) => atlas.axisById.has(id));
  if (axes.length < MIN_FLOW_AXES && atlas.axisById.has(DERIVED_AXES.statusStart)) {
    axes.unshift(DERIVED_AXES.statusStart);
  }
  return axes.slice(0, MAX_FLOW_AXES);
}

/** The query an atlas opens with: the last 30 days, everything selected, the flow. */
export function defaultQuery(atlas: Atlas): AtlasQuery {
  const to = atlas.lastDay;
  const from = maxDay(atlas.firstDay, addDays(to, -29));
  const data = dataAxesBy(atlas, FLOW_CANDIDATES);
  const raster = data.find((id) => id !== "family") ?? data[0] ?? DERIVED_AXES.statusSnapshot;
  return {
    from,
    to,
    asOf: to,
    follow: true,
    clock: "recorded",
    filters: {},
    ids: [],
    view: "flow",
    flowAxes: defaultFlowAxes(atlas),
    lens: "origin",
    surrounding: true,
    matrixRows: data[0] ?? DERIVED_AXES.statusSnapshot,
    matrixCols: data[1] ?? DERIVED_AXES.statusSnapshot,
    rasterAxis: raster,
    measure: "records",
  };
}

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function pick<T extends string>(v: unknown, allowed: readonly T[], fallback: T): T {
  return typeof v === "string" && (allowed as readonly string[]).includes(v) ? (v as T) : fallback;
}

function axisOr(atlas: Atlas, v: unknown, fallback: string): string {
  return typeof v === "string" && atlas.axisById.has(v) ? v : fallback;
}

/**
 * Normalise anything that claims to be a query against an atlas: days clamped
 * into the data, the window ordered, the snapshot following the end when it
 * says so, filters kept only for axes and values the atlas has, two to five
 * distinct flow columns, the map only where there is geography to draw.
 * Never throws; whatever cannot be read falls back to the default.
 */
export function validateQuery(raw: unknown, atlas: Atlas): AtlasQuery {
  const d = defaultQuery(atlas);
  if (!isRecord(raw)) return d;
  const lo = atlas.firstDay;
  const hi = atlas.lastDay;

  let from = isDay(raw.from) ? clampDay(raw.from, lo, hi) : d.from;
  let to = isDay(raw.to) ? clampDay(raw.to, lo, hi) : d.to;
  if (from > to) [from, to] = [to, from];
  const follow = typeof raw.follow === "boolean" ? raw.follow : true;
  const asOf = follow ? to : isDay(raw.asOf) ? clampDay(raw.asOf, lo, hi) : to;

  const filters: Record<string, string[]> = {};
  if (isRecord(raw.filters)) {
    for (const [axisId, values] of Object.entries(raw.filters)) {
      const axis = atlas.axisById.get(axisId);
      if (!axis || !Array.isArray(values)) continue;
      const known = new Set(axis.values.map((v) => v.id));
      const kept = [...new Set(values.filter((v): v is string => typeof v === "string"))].filter(
        (v) => known.has(v),
      );
      if (kept.length > 0) filters[axisId] = kept;
    }
  }

  const ids = Array.isArray(raw.ids)
    ? [...new Set(raw.ids.filter((v): v is string => typeof v === "string"))].filter((id) =>
        atlas.byId.has(id),
      )
    : [];

  let flowAxes = Array.isArray(raw.flowAxes)
    ? [...new Set(raw.flowAxes.filter((v): v is string => typeof v === "string"))].filter((id) =>
        atlas.axisById.has(id),
      )
    : [];
  if (flowAxes.length < MIN_FLOW_AXES) flowAxes = d.flowAxes;
  flowAxes = flowAxes.slice(0, MAX_FLOW_AXES);

  let view = pick(raw.view, ATLAS_VIEWS, d.view);
  if (view === "map" && !geoAxis(atlas)) view = "flow";

  return {
    from,
    to,
    asOf,
    follow,
    clock: pick(raw.clock, EVENT_CLOCKS, d.clock),
    filters,
    ids,
    view,
    flowAxes,
    lens: pick(raw.lens, FLOW_LENSES, d.lens),
    surrounding: typeof raw.surrounding === "boolean" ? raw.surrounding : d.surrounding,
    matrixRows: axisOr(atlas, raw.matrixRows, d.matrixRows),
    matrixCols: axisOr(atlas, raw.matrixCols, d.matrixCols),
    rasterAxis: axisOr(atlas, raw.rasterAxis, d.rasterAxis),
    measure: pick(raw.measure, MEASURES, d.measure),
  };
}

/** Whether the query restricts the records at all. */
export function hasFilters(q: AtlasQuery): boolean {
  return q.ids.length > 0 || Object.values(q.filters).some((v) => v.length > 0);
}

// ── Editing ──────────────────────────────────────────────────────────────

/** Set the window; the snapshot follows the end while `follow` holds. */
export function withWindow(q: AtlasQuery, from: string, to: string): AtlasQuery {
  const [a, b] = from <= to ? [from, to] : [to, from];
  return { ...q, from: a, to: b, asOf: q.follow ? b : q.asOf };
}

/** A window of `days` days ending on the atlas's last day; `all` covers the whole span. */
export function presetWindow(atlas: Atlas, days: number | "all"): { from: string; to: string } {
  const to = atlas.lastDay;
  if (days === "all") return { from: atlas.firstDay, to };
  return { from: maxDay(atlas.firstDay, addDays(to, -(days - 1))), to };
}

/** Move the window by `days`, keeping its width and staying inside the data. */
export function shiftWindow(q: AtlasQuery, atlas: Atlas, days: number): AtlasQuery {
  const width = Math.round((Date.parse(q.to) - Date.parse(q.from)) / 86_400_000);
  let from = addDays(q.from, days);
  let to = addDays(q.to, days);
  if (from < atlas.firstDay) {
    from = atlas.firstDay;
    to = minDay(addDays(from, width), atlas.lastDay);
  }
  if (to > atlas.lastDay) {
    to = atlas.lastDay;
    from = maxDay(addDays(to, -width), atlas.firstDay);
  }
  return withWindow(q, from, to);
}

/** Pin the snapshot to a day; it stops following the window. */
export function withSnapshot(q: AtlasQuery, day: string): AtlasQuery {
  return { ...q, asOf: day, follow: false };
}

/** Let the snapshot follow the window's end again, or stop it. */
export function withFollow(q: AtlasQuery, follow: boolean): AtlasQuery {
  return { ...q, follow, asOf: follow ? q.to : q.asOf };
}

/**
 * Retain values on axes. Each entry replaces that axis's values, or with
 * `add` joins them (OR within the axis); a value already retained is removed
 * by a second `add`, so shift-clicking toggles.
 */
export function withRetained(
  q: AtlasQuery,
  picks: { axis: string; value: string }[],
  add = false,
): AtlasQuery {
  const filters = { ...q.filters };
  const byAxis = new Map<string, string[]>();
  for (const { axis, value } of picks) {
    byAxis.set(axis, [...(byAxis.get(axis) ?? []), value]);
  }
  for (const [axis, values] of byAxis) {
    if (add) {
      const current = new Set(filters[axis] ?? []);
      for (const v of values) {
        if (current.has(v)) current.delete(v);
        else current.add(v);
      }
      if (current.size === 0) delete filters[axis];
      else filters[axis] = [...current];
    } else {
      filters[axis] = [...new Set(values)];
    }
  }
  return { ...q, filters };
}

/** Drop one value (or, without `value`, every value) retained on an axis. */
export function withoutFilter(q: AtlasQuery, axis: string, value?: string): AtlasQuery {
  const filters = { ...q.filters };
  if (value === undefined) delete filters[axis];
  else {
    const kept = (filters[axis] ?? []).filter((v) => v !== value);
    if (kept.length === 0) delete filters[axis];
    else filters[axis] = kept;
  }
  return { ...q, filters };
}

/** Drop every filter and the explicit id set. */
export function withoutFilters(q: AtlasQuery): AtlasQuery {
  return { ...q, filters: {}, ids: [] };
}

// ── Flow columns ─────────────────────────────────────────────────────────

/**
 * Put `axis` in column `index`. An axis already shown elsewhere swaps places
 * with the one it replaces, so a column is never shown twice.
 */
export function setFlowAxis(q: AtlasQuery, index: number, axis: string): AtlasQuery {
  const axes = [...q.flowAxes];
  if (index < 0 || index >= axes.length) return q;
  const at = axes.indexOf(axis);
  if (at === index) return q;
  if (at !== -1) axes[at] = axes[index];
  axes[index] = axis;
  return { ...q, flowAxes: axes };
}

/** Move a column one place left (`-1`) or right (`1`). */
export function moveFlowAxis(q: AtlasQuery, index: number, dir: -1 | 1): AtlasQuery {
  const j = index + dir;
  if (index < 0 || j < 0 || index >= q.flowAxes.length || j >= q.flowAxes.length) return q;
  const axes = [...q.flowAxes];
  [axes[index], axes[j]] = [axes[j], axes[index]];
  return { ...q, flowAxes: axes };
}

/** Remove a column, keeping at least two. */
export function removeFlowAxis(q: AtlasQuery, index: number): AtlasQuery {
  if (q.flowAxes.length <= MIN_FLOW_AXES) return q;
  return { ...q, flowAxes: q.flowAxes.filter((_, i) => i !== index) };
}

/** Add a column on the right, up to five; the first axis not shown when none is named. */
export function addFlowAxis(q: AtlasQuery, atlas: Atlas, axis?: string): AtlasQuery {
  if (q.flowAxes.length >= MAX_FLOW_AXES) return q;
  const next = axis ?? atlas.axes.find((a) => !q.flowAxes.includes(a.id))?.id;
  if (!next || q.flowAxes.includes(next) || !atlas.axisById.has(next)) return q;
  return { ...q, flowAxes: [...q.flowAxes, next] };
}

// ── Questions to start from ──────────────────────────────────────────────

export const QUESTION_IDS = ["changed", "applies", "needs-person", "lifetime"] as const;
export type QuestionId = (typeof QUESTION_IDS)[number];

/**
 * Arrange the query to answer one of the starting questions. Axes the atlas
 * lacks are skipped; the window and snapshot are kept.
 *
 * - `changed`: what changed in this window (records an event touched, from
 *   their state at the start to their state at the snapshot).
 * - `applies`: where rules apply (family, channel, locale or region, state).
 * - `needs-person`: what waits for a person (state at start to state at snapshot).
 * - `lifetime`: rules with a lifetime (lifetime, family, channel).
 */
export function applyQuestion(q: AtlasQuery, atlas: Atlas, id: QuestionId): AtlasQuery {
  const has = (axis: string) => atlas.axisById.has(axis);
  const firstOf = (...ids: string[]) => ids.find((id) => usable(atlas, id));
  const columns = (...ids: (string | undefined)[]) => {
    const axes = [...new Set(ids.filter((id): id is string => !!id && has(id)))];
    return axes.length >= MIN_FLOW_AXES ? axes.slice(0, MAX_FLOW_AXES) : q.flowAxes;
  };
  const base: AtlasQuery = { ...q, view: "flow", filters: {}, ids: [] };
  const familyHas = (v: string) =>
    atlas.axisById.get("family")?.values.some((x) => x.id === v) ?? false;

  switch (id) {
    case "changed":
      return {
        ...base,
        lens: "change",
        filters: { [DERIVED_AXES.activity]: ["changed"] },
        flowAxes: columns(
          "family",
          DERIVED_AXES.statusStart,
          DERIVED_AXES.statusSnapshot,
          firstOf("channel", "product", "locale"),
        ),
      };
    case "applies":
      return {
        ...base,
        lens: "origin",
        filters: familyHas("rule") ? { family: ["rule"] } : {},
        flowAxes: columns(
          "family",
          firstOf("channel", "product"),
          firstOf("locale", "region", "market"),
          DERIVED_AXES.statusSnapshot,
        ),
      };
    case "needs-person": {
      const waiting = [...NEEDS_PERSON].filter((s) =>
        atlas.records.some((p) => p.historyState.includes(s)),
      );
      return {
        ...base,
        lens: "attention",
        filters: waiting.length > 0 ? { [DERIVED_AXES.statusSnapshot]: waiting } : {},
        flowAxes: columns(
          DERIVED_AXES.statusStart,
          DERIVED_AXES.statusSnapshot,
          "family",
          firstOf("product", "channel"),
        ),
      };
    }
    case "lifetime":
      return {
        ...base,
        lens: "origin",
        filters: { [DERIVED_AXES.lifetime]: ["upcoming", "in-force", "expired"] },
        flowAxes: columns(
          DERIVED_AXES.lifetime,
          "family",
          firstOf("channel", "product"),
          DERIVED_AXES.statusSnapshot,
        ),
      };
  }
}

// ── Links ────────────────────────────────────────────────────────────────

/** The fragment that restores a query: `q=<URI-encoded JSON>`. */
export function serialiseQuery(q: AtlasQuery): string {
  return `q=${encodeURIComponent(JSON.stringify(q))}`;
}

/**
 * Read a query back out of a location hash (with or without the `#`, alone or
 * among other `&`-separated parameters). Returns the raw value for
 * `validateQuery`, or null when the hash carries none or it does not parse.
 */
export function parseQueryHash(hash: string): unknown {
  const body = hash.startsWith("#") ? hash.slice(1) : hash;
  for (const part of body.split("&")) {
    if (!part.startsWith("q=")) continue;
    try {
      return JSON.parse(decodeURIComponent(part.slice(2)));
    } catch {
      return null;
    }
  }
  return null;
}

// ── Back and forward ─────────────────────────────────────────────────────

/** The investigation's own back and forward stacks. */
export interface QueryHistory {
  past: AtlasQuery[];
  present: AtlasQuery;
  future: AtlasQuery[];
}

export function startHistory(present: AtlasQuery): QueryHistory {
  return { past: [], present, future: [] };
}

function sameQuery(a: AtlasQuery, b: AtlasQuery): boolean {
  return JSON.stringify(a) === JSON.stringify(b);
}

/** Move to a new query: the old one joins the past (60 at most) and the future clears. */
export function pushQuery(h: QueryHistory, next: AtlasQuery): QueryHistory {
  if (sameQuery(h.present, next)) return h;
  const past = [...h.past, h.present].slice(-HISTORY_LIMIT);
  return { past, present: next, future: [] };
}

/** Replace the present without a history entry (playback, dragging). */
export function replaceQuery(h: QueryHistory, next: AtlasQuery): QueryHistory {
  return { ...h, present: next };
}

export function canGoBack(h: QueryHistory): boolean {
  return h.past.length > 0;
}

export function canGoForward(h: QueryHistory): boolean {
  return h.future.length > 0;
}

export function goBack(h: QueryHistory): QueryHistory {
  if (h.past.length === 0) return h;
  const present = h.past[h.past.length - 1];
  return { past: h.past.slice(0, -1), present, future: [h.present, ...h.future] };
}

export function goForward(h: QueryHistory): QueryHistory {
  if (h.future.length === 0) return h;
  const [present, ...future] = h.future;
  return { past: [...h.past, h.present].slice(-HISTORY_LIMIT), present, future };
}
