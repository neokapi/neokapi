// The atlas prepared for querying: records with parsed instants and sorted
// history, the axis catalogue (the data's axes plus the ones the view
// derives), and the span in whole days.

import type { AtlasAxis, AtlasData, AtlasRecord, AtlasValue } from "../types";
import { ABSENT, STATE_ORDER, UNKNOWN } from "./state";
import { DAY_MS, dayOf, instantMs, isUnknownInstant } from "./time";

/** What a record weighs: one each, or its unit count. */
export type Measure = "records" | "units";

/** The value a record takes on an axis it carries no coordinate for. */
export const NOT_SET = "";

/** Axes the view derives from time; never on the wire. */
export const DERIVED_AXES = {
  /** The state at the window's start. */
  statusStart: "status-start",
  /** The state at the snapshot. */
  statusSnapshot: "status-snapshot",
  /** Where the snapshot falls in the record's lifetime window. */
  lifetime: "lifetime",
  /** Time since the record was recorded, at the snapshot. */
  age: "record-age",
  /** Whether an event touched the record inside the window. */
  activity: "activity",
} as const;

export type DerivedAxisId = (typeof DERIVED_AXES)[keyof typeof DERIVED_AXES];

export const LIFETIME_VALUES = ["upcoming", "in-force", "expired", "open"] as const;
export const AGE_VALUES = ["lt-7d", "7-30d", "30-90d", "gt-90d", "unknown"] as const;
export const ACTIVITY_VALUES = ["changed", "quiet"] as const;

/** Where an axis in the catalogue comes from. */
export type AxisOrigin = "structural" | "declared" | "derived" | "view";

/** One axis as the view knows it. */
export interface AxisInfo {
  id: string;
  /** The data's label, when it carried one. */
  label?: string;
  kind: string;
  origin: AxisOrigin;
  /** Every value, in display order. Status axes list every known state. */
  values: AtlasValue[];
  /** Whether the value depends on the query's dates. */
  temporal: boolean;
}

/** A record with its instants parsed once. */
export interface PreparedRecord {
  index: number;
  record: AtlasRecord;
  weight: number;
  recordedMs: number;
  validFromMs: number;
  validToMs: number;
  historyAt: number[];
  historyState: string[];
}

/** A prepared event. */
export interface PreparedEvent {
  index: number;
  recordedMs: number;
  effectiveMs: number;
  kind: string;
  record?: string;
}

/** The atlas, ready for queries. */
export interface Atlas {
  data: AtlasData;
  measure: Measure;
  /** Whether any record weighs other than one in the data. */
  weighted: boolean;
  records: PreparedRecord[];
  byId: Map<string, PreparedRecord>;
  events: PreparedEvent[];
  axes: AxisInfo[];
  axisById: Map<string, AxisInfo>;
  /** First and last day the data covers. */
  firstDay: string;
  lastDay: string;
}

function weightOf(r: AtlasRecord): number {
  const w = Number(r.weight);
  return Number.isFinite(w) && w > 0 ? w : 1;
}

function prepareRecord(record: AtlasRecord, index: number, measure: Measure): PreparedRecord {
  const entries = (record.history ?? [])
    .map((h) => ({ at: instantMs(h.at), state: h.state }))
    .filter((h) => !Number.isNaN(h.at))
    .sort((a, b) => a.at - b.at);
  return {
    index,
    record,
    weight: measure === "units" ? weightOf(record) : 1,
    recordedMs: isUnknownInstant(record.recorded) ? Number.NaN : instantMs(record.recorded),
    validFromMs: record.valid_from ? instantMs(record.valid_from) : Number.NaN,
    validToMs: record.valid_to ? instantMs(record.valid_to) : Number.NaN,
    historyAt: entries.map((e) => e.at),
    historyState: entries.map((e) => e.state),
  };
}

/** The state a prepared record held at `t`; see `stateAt`. */
export function preparedStateAt(p: PreparedRecord, t: number): string {
  if (p.historyAt.length === 0) {
    if (Number.isNaN(p.recordedMs)) return UNKNOWN;
    return p.recordedMs <= t ? UNKNOWN : ABSENT;
  }
  // Binary search for the last entry at or before t.
  let lo = 0;
  let hi = p.historyAt.length - 1;
  let found = -1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (p.historyAt[mid] <= t) {
      found = mid;
      lo = mid + 1;
    } else {
      hi = mid - 1;
    }
  }
  return found === -1 ? ABSENT : p.historyState[found];
}

/** The raw coordinate a record carries on a data axis. */
export function coordinateOf(p: PreparedRecord, axis: string): string {
  const v = p.record.coordinates?.[axis];
  if (v !== undefined && v !== null && v !== "") return v;
  if (axis === "family") return p.record.family || NOT_SET;
  return NOT_SET;
}

function dataAxis(axis: AtlasAxis, records: PreparedRecord[]): AxisInfo {
  const values: AtlasValue[] = [];
  const seen = new Set<string>();
  for (const v of axis.values ?? []) {
    if (v && typeof v.id === "string" && v.id !== "" && !seen.has(v.id)) {
      seen.add(v.id);
      values.push(v);
    }
  }
  // A value a record uses that the axis did not list still gets a place, after
  // the listed ones, so no record falls out of the view.
  const extra = new Set<string>();
  let unset = false;
  for (const p of records) {
    const v = coordinateOf(p, axis.id);
    if (v === NOT_SET) unset = true;
    else if (!seen.has(v)) extra.add(v);
  }
  for (const v of [...extra].sort()) values.push({ id: v });
  if (unset) values.push({ id: NOT_SET });
  const source = axis.source as string;
  const origin: AxisOrigin =
    source === "structural" || source === "declared" || source === "derived" ? source : "declared";
  return {
    id: axis.id,
    label: axis.label || undefined,
    kind: axis.kind || "category",
    origin,
    values,
    temporal: false,
  };
}

function viewAxis(id: DerivedAxisId, values: readonly string[]): AxisInfo {
  return {
    id,
    kind: "ordered",
    origin: "view",
    values: values.map((v) => ({ id: v })),
    temporal: true,
  };
}

/**
 * Prepare atlas data for querying. Tolerates missing arrays. With `measure:
 * "records"` every record weighs one; with `"units"` (the default) a content
 * cell weighs its unit count, as the contract carries it.
 */
export function prepareAtlas(data: AtlasData, opts: { measure?: Measure } = {}): Atlas {
  const measure = opts.measure ?? "units";
  const records = (data.records ?? []).map((r, i) => prepareRecord(r, i, measure));
  const byId = new Map(records.map((r) => [r.record.id, r]));
  const events: PreparedEvent[] = (data.events ?? []).map((e, index) => {
    const recordedMs = instantMs(e.at);
    const eff = e.effective ? instantMs(e.effective) : Number.NaN;
    return {
      index,
      recordedMs,
      effectiveMs: Number.isNaN(eff) ? recordedMs : eff,
      kind: e.kind,
      record: e.record,
    };
  });

  const axes: AxisInfo[] = [];
  const dataIds = new Set<string>();
  for (const a of data.axes ?? []) {
    if (!a || typeof a.id !== "string" || a.id === "" || dataIds.has(a.id)) continue;
    dataIds.add(a.id);
    axes.push(dataAxis(a, records));
  }
  // `family` is on every record; give it an axis when the data did not.
  if (!dataIds.has("family") && records.length > 0) {
    dataIds.add("family");
    axes.unshift(
      dataAxis({ id: "family", kind: "category", source: "derived", values: [] }, records),
    );
  }
  // A state the contract does not name still gets a place on the status axes.
  const known = new Set<string>(STATE_ORDER);
  const extraStates = new Set<string>();
  for (const p of records) for (const s of p.historyState) if (!known.has(s)) extraStates.add(s);
  const states = [
    ...STATE_ORDER.slice(0, -2),
    ...[...extraStates].sort(),
    ...STATE_ORDER.slice(-2),
  ];
  const derived: [DerivedAxisId, readonly string[]][] = [
    [DERIVED_AXES.statusStart, states],
    [DERIVED_AXES.statusSnapshot, states],
    [DERIVED_AXES.lifetime, LIFETIME_VALUES],
    [DERIVED_AXES.age, AGE_VALUES],
    [DERIVED_AXES.activity, ACTIVITY_VALUES],
  ];
  // A data axis that happens to share a derived axis's id wins; the derived
  // one is left out rather than shadowing what the project declared.
  for (const [id, values] of derived) if (!dataIds.has(id)) axes.push(viewAxis(id, values));

  let lo = Number.POSITIVE_INFINITY;
  let hi = Number.NEGATIVE_INFINITY;
  const widen = (ms: number) => {
    if (Number.isNaN(ms)) return;
    if (ms < lo) lo = ms;
    if (ms > hi) hi = ms;
  };
  // The span the host reports reaches the bounds of windows that open or close
  // later; the timeline stops at the moment the atlas was read, so a window
  // ending next month does not leave the default view looking at the future.
  const generated = instantMs(data.generated_at);
  const spanTo = instantMs(data.span?.to);
  widen(instantMs(data.span?.from));
  widen(Number.isNaN(generated) ? spanTo : Math.min(spanTo, generated));
  for (const p of records) {
    widen(p.recordedMs);
    for (const at of p.historyAt) widen(at);
  }
  // The span runs on the recorded clock. An event that takes effect later (a
  // window that opens next month) stays outside it; the histogram counts it as
  // outside rather than stretching the timeline into the future.
  for (const e of events) widen(e.recordedMs);
  if (!Number.isFinite(lo)) {
    const gen = instantMs(data.generated_at);
    lo = hi = Number.isNaN(gen) ? Math.floor(Date.now() / DAY_MS) * DAY_MS : gen;
  }

  return {
    data,
    measure,
    weighted: (data.records ?? []).some((r) => weightOf(r) !== 1),
    records,
    byId,
    events,
    axes,
    axisById: new Map(axes.map((a) => [a.id, a])),
    firstDay: dayOf(lo),
    lastDay: dayOf(hi),
  };
}

/** The data axis that carries geography with at least one placed value, if any. */
export function geoAxis(atlas: Atlas): AxisInfo | undefined {
  return atlas.axes.find(
    (a) =>
      a.kind === "geo" &&
      a.origin !== "view" &&
      a.values.some((v) => v.centroid || (v.members?.length ?? 0) > 0),
  );
}
