// A portable investigation record: the query, what was pinned, the record in
// view and the counts the view showed, with the data it was read from named by
// format, version, project, span and generation time. A person or an agent can
// restore the query from it and check the counts against the same data.

import type { Atlas } from "./atlas";
import { type EvalContext, cohort, evalContext } from "./filter";
import { eventsInWindow } from "./histogram";
import type { AtlasQuery } from "./query";
import { NEEDS_PERSON } from "./state";
import { preparedStateAt } from "./atlas";
import { type Transition, transitions } from "./transitions";

export const INVESTIGATION_FORMAT = "kapi-context-investigation";
export const INVESTIGATION_VERSION = 1;

export interface InvestigationCounts {
  /** Records in the selection, and their summed weight. */
  records: number;
  weight: number;
  /** Records in the atlas, and their summed weight. */
  total_records: number;
  total_weight: number;
  /** Weight waiting for a person at the snapshot. */
  needs_person: number;
  /** Weight whose state differs between the window's start and the snapshot. */
  changed_state: number;
  /** Events in the window on the query's clock. */
  events_in_window: number;
}

export interface Investigation {
  format: typeof INVESTIGATION_FORMAT;
  version: typeof INVESTIGATION_VERSION;
  generated_at: string;
  data: {
    format: string;
    version: number;
    project?: string;
    generated_at: string;
    span: { from: string; to: string };
  };
  query: AtlasQuery;
  pinned: AtlasQuery | null;
  selected: { id: string; label: string; short?: string } | null;
  counts: InvestigationCounts;
  /** Every (start, snapshot) pair in the selection, largest first. */
  transitions: Transition[];
}

/** The counts the view shows for a query. */
export function investigationCounts(
  atlas: Atlas,
  q: AtlasQuery,
  ctx: EvalContext = evalContext(atlas, q),
): InvestigationCounts {
  const indices = cohort(atlas, q, ctx);
  let weight = 0;
  let needs = 0;
  for (const i of indices) {
    const p = atlas.records[i];
    weight += p.weight;
    if (NEEDS_PERSON.has(preparedStateAt(p, ctx.snapshotMs))) needs += p.weight;
  }
  const t = transitions(atlas, indices, ctx);
  return {
    records: indices.length,
    weight,
    total_records: atlas.records.length,
    total_weight: atlas.records.reduce((s, p) => s + p.weight, 0),
    needs_person: needs,
    changed_state: t.changedWeight,
    events_in_window: eventsInWindow(atlas, q.clock, ctx.startMs, ctx.endMs),
  };
}

/** Build the investigation record. `now` is injectable so tests are stable. */
export function exportInvestigation(
  atlas: Atlas,
  q: AtlasQuery,
  opts: { pinned?: AtlasQuery | null; selected?: string | null; now?: Date } = {},
): Investigation {
  const ctx = evalContext(atlas, q);
  const selected = opts.selected ? atlas.byId.get(opts.selected) : undefined;
  const d = atlas.data;
  return {
    format: INVESTIGATION_FORMAT,
    version: INVESTIGATION_VERSION,
    generated_at: (opts.now ?? new Date()).toISOString(),
    data: {
      format: d.format,
      version: d.version,
      ...(d.project ? { project: d.project } : {}),
      generated_at: d.generated_at,
      span: { from: d.span?.from ?? "", to: d.span?.to ?? "" },
    },
    query: q,
    pinned: opts.pinned ?? null,
    selected: selected
      ? {
          id: selected.record.id,
          label: selected.record.label,
          ...(selected.record.short ? { short: selected.record.short } : {}),
        }
      : null,
    counts: investigationCounts(atlas, q, ctx),
    transitions: transitions(atlas, cohort(atlas, q, ctx), ctx).transitions,
  };
}
