import type { ConvergeEvent } from "../types/api";
import type { RunEvent } from "../context/JobFeedContext";
// The render model is shared across the Apache/AGPL boundary — this reducer
// (which knows the desktop job-feed's typed events) folds into it, and the
// shared <ConvergenceRunView /> renders it. See @neokapi/status-views.
import type {
  ConvergenceLocaleRow as ConvergeLocaleRow,
  ConvergencePassView as ConvergePassView,
  ConvergenceRunModel as ConvergeRunModel,
  LocaleRowState,
} from "@neokapi/status-views";

export type { ConvergeLocaleRow, ConvergePassView, ConvergeRunModel, LocaleRowState };

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
