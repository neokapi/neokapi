import type { ConvergeEvent } from "../types/api";
import type { RunEvent } from "../context/JobFeedContext";

/** A locale's lifecycle within a pass, mirroring the CLI live renderer. */
export type LocaleRowState = "queued" | "running" | "done";

/** One locale's live cell in a pass: unit progress + the TM/AI split. */
export interface ConvergeLocaleRow {
  locale: string;
  units: number;
  done: number;
  viaTM: number;
  viaAI: number;
  state: LocaleRowState;
}

/** One pass of a run: its live locale rows plus the post-derivation summary. */
export interface ConvergePassView {
  pass: number;
  /** Cap on passes (0 when unknown, e.g. the compat path). */
  maxPasses: number;
  rows: ConvergeLocaleRow[];
  /** True once the pass's post-derivation (pass_done) has arrived. */
  settled: boolean;
  produced?: number;
  producedDelta?: number;
  failingChecks?: number;
  /** Locales still short of their gate after the pass. */
  pending?: string[];
}

/** The reduced run model the live view renders. */
export interface ConvergeRunModel {
  passes: ConvergePassView[];
  materializedFiles?: number;
  /** "converged" | "parked" once the run's done event arrived. */
  finalState?: string;
  /**
   * True when the model was built from the typed converge_event stream (live
   * locale rows). False for the compatibility path, built from the coarser
   * converge_pass summaries (no per-locale rows).
   */
  live: boolean;
}

function newRow(locale: string): ConvergeLocaleRow {
  return { locale, units: 0, done: 0, viaTM: 0, viaAI: 0, state: "queued" };
}

/** Find (or, defensively, create) a locale row in the current pass. */
function ensureRow(pass: ConvergePassView, locale: string): ConvergeLocaleRow {
  let row = pass.rows.find((r) => r.locale === locale);
  if (!row) {
    row = newRow(locale);
    pass.rows.push(row);
  }
  return row;
}

/**
 * reduceConvergeRun folds a convergence run's event log into the render model.
 *
 * The typed converge_event stream is authoritative when present — it carries
 * live per-locale progress. When a run only stored the coarser converge_pass
 * summaries (an older backend, or a reconnect that missed the live stream), it
 * degrades to a pass-summary-only model with no locale rows.
 */
export function reduceConvergeRun(events: RunEvent[]): ConvergeRunModel {
  const typed = events
    .filter((e) => e.type === "converge_event" && e.converge_event)
    .map((e) => e.converge_event!);
  if (typed.length > 0) return reduceFromEvents(typed);
  return reduceFromPasses(events);
}

function reduceFromEvents(events: ConvergeEvent[]): ConvergeRunModel {
  const passes: ConvergePassView[] = [];
  let materializedFiles: number | undefined;
  let finalState: string | undefined;

  // Fall back to the last pass if a locale event arrives without a pass_start.
  const currentPass = (): ConvergePassView | null =>
    passes.length > 0 ? passes[passes.length - 1] : null;

  for (const ev of events) {
    switch (ev.type) {
      case "pass_start":
        passes.push({
          pass: ev.pass ?? passes.length + 1,
          maxPasses: ev.maxPasses ?? 0,
          rows: (ev.pending ?? []).map(newRow),
          settled: false,
        });
        break;
      case "locale_start": {
        const pass = currentPass();
        if (pass && ev.locale) {
          const row = ensureRow(pass, ev.locale);
          row.state = "running";
          if (ev.units != null) row.units = ev.units;
        }
        break;
      }
      case "unit_progress": {
        const pass = currentPass();
        if (pass && ev.locale) {
          const row = ensureRow(pass, ev.locale);
          row.done = ev.done ?? 0;
          row.viaTM = ev.viaTM ?? 0;
          row.viaAI = ev.viaAI ?? 0;
        }
        break;
      }
      case "locale_done": {
        const pass = currentPass();
        if (pass && ev.locale) {
          const row = ensureRow(pass, ev.locale);
          row.state = "done";
          if (ev.units != null) row.units = ev.units;
          row.done = ev.done ?? row.done;
          row.viaTM = ev.viaTM ?? row.viaTM;
          row.viaAI = ev.viaAI ?? row.viaAI;
        }
        break;
      }
      case "pass_done": {
        const pass = currentPass();
        if (pass) {
          pass.settled = true;
          pass.produced = ev.produced;
          pass.producedDelta = ev.producedDelta;
          pass.failingChecks = ev.failingChecks;
          pass.pending = ev.pending;
        }
        break;
      }
      case "materialized":
        materializedFiles = ev.files;
        break;
      case "done":
        finalState = ev.state;
        break;
      // "log" carries no structured shape; the backend surfaces it as a
      // progress line, so the reducer ignores it here.
    }
  }

  return { passes, materializedFiles, finalState, live: true };
}

function reduceFromPasses(events: RunEvent[]): ConvergeRunModel {
  const passes = events
    .filter((e) => e.type === "converge_pass" && e.converge)
    .map((e) => e.converge!)
    .map<ConvergePassView>((p) => ({
      pass: p.pass,
      maxPasses: 0,
      rows: [],
      settled: true,
      produced: p.produced,
      producedDelta: p.producedDelta,
      failingChecks: p.failingChecks,
      pending: p.pendingLocales,
    }));
  return { passes, live: false };
}
