import type { ConvergenceEvent } from "../types/api";

/**
 * runReducer folds a server-side convergence run's SSE event stream into a
 * render model. The stream (bowrain/server/handlers_convergence.go) replays a
 * run's persisted events from seq 0, then follows live until the terminal
 * `done` event — so the same fold serves a finished run and a live one.
 *
 * The reducer is pure: it never touches the network or the DOM. The web Runs
 * surface reduces the events an EventSource delivers; tests reduce fixtures.
 */

/** A locale's lifecycle within a pass, mirroring the CLI live renderer. */
export type LocaleRowState = "queued" | "running" | "done";

/** One locale's live cell in a pass: unit progress + the TM/AI split. */
export interface ConvergenceLocaleRow {
  locale: string;
  /** Denominator a progress bar renders against (0 until locale_start). */
  units: number;
  /** Committed targets so far in this pass. */
  done: number;
  viaTM: number;
  viaAI: number;
  state: LocaleRowState;
  /**
   * Authoritative per-locale ship verdict (shippable | parked | pending). The
   * live stream is state-less, so this stays undefined while streaming — the
   * badge falls back to `state` (the coverage-derived lifecycle) — and is only
   * set from the run's terminal standing (or a compat state-carrying event).
   */
  localeState?: string;
}

/** One pass of a run: its live locale rows plus the post-derivation summary. */
export interface ConvergencePassView {
  pass: number;
  /** Cap on passes (0 when unknown). */
  maxPasses: number;
  rows: ConvergenceLocaleRow[];
  /** True once the pass's post-derivation (pass_done) has arrived. */
  settled: boolean;
  produced?: number;
  producedDelta?: number;
  failingChecks?: number;
  /** Locales still short of their gate after the pass. */
  pending?: string[];
}

/** The reduced run model the live view renders. */
export interface ConvergenceRunModel {
  passes: ConvergencePassView[];
  /** Human-readable run log lines (auto-extract, transport notes). */
  logs: string[];
  materializedFiles?: number;
  /** "converged" | "parked" once the run's done event arrived. */
  finalState?: string;
  /** True once the terminal `done` event has been folded in. */
  done: boolean;
}

/** A fresh, empty run model — the fold's identity. */
export function emptyRunModel(): ConvergenceRunModel {
  return { passes: [], logs: [], done: false };
}

function newRow(locale: string): ConvergenceLocaleRow {
  return { locale, units: 0, done: 0, viaTM: 0, viaAI: 0, state: "queued" };
}

/**
 * deriveRowState derives a locale's lifecycle from coverage alone. The live
 * stream does not carry a per-locale ship state, so the streaming badge is
 * driven by progress: all units covered → done, some work started → running,
 * else queued. The authoritative shippable/parked verdict comes from the run's
 * terminal standing (REST), never inferred here.
 */
function deriveRowState(row: ConvergenceLocaleRow): LocaleRowState {
  if (row.units > 0 && row.done >= row.units) return "done";
  if (row.done > 0 || row.units > 0) return "running";
  return "queued";
}

/** Find (or, defensively, create) a locale row in a pass. */
function ensureRow(pass: ConvergencePassView, locale: string): ConvergenceLocaleRow {
  let row = pass.rows.find((r) => r.locale === locale);
  if (!row) {
    row = newRow(locale);
    pass.rows.push(row);
  }
  return row;
}

/**
 * applyEvent folds one event into the model, mutating and returning it. The
 * model is the accumulator; callers that need immutability should clone first
 * (see reduceRun, which rebuilds from scratch).
 */
export function applyEvent(model: ConvergenceRunModel, ev: ConvergenceEvent): ConvergenceRunModel {
  // The pass a locale-scoped event belongs to: prefer the event's own pass
  // stamp, else the most recent pass.
  const currentPass = (): ConvergencePassView | null => {
    if (ev.pass != null) {
      const p = model.passes.find((x) => x.pass === ev.pass);
      if (p) return p;
    }
    return model.passes.length > 0 ? model.passes[model.passes.length - 1] : null;
  };

  switch (ev.type) {
    case "pass_start": {
      model.passes.push({
        pass: ev.pass ?? model.passes.length + 1,
        maxPasses: ev.maxPasses ?? 0,
        rows: (ev.pending ?? []).map(newRow),
        settled: false,
      });
      if (ev.extractedFiles || ev.extractedBlocks) {
        model.logs.push(
          `Extracted ${ev.extractedBlocks ?? 0} blocks from ${ev.extractedFiles ?? 0} files`,
        );
      }
      break;
    }
    case "locale_start": {
      const pass = currentPass();
      if (pass && ev.locale) {
        const row = ensureRow(pass, ev.locale);
        if (ev.units != null) row.units = ev.units;
        row.state = deriveRowState(row);
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
        row.state = deriveRowState(row);
      }
      break;
    }
    case "locale_done": {
      const pass = currentPass();
      if (pass && ev.locale) {
        const row = ensureRow(pass, ev.locale);
        if (ev.units != null) row.units = ev.units;
        row.done = ev.done ?? row.done;
        row.viaTM = ev.viaTM ?? row.viaTM;
        row.viaAI = ev.viaAI ?? row.viaAI;
        // locale_done is terminal for the row within this pass.
        row.state = "done";
        // The live stream is state-less; only adopt a per-locale ship verdict
        // when the event actually carries one (compat streams / final standing
        // replayed as events). Never clobber a prior verdict with undefined.
        if (ev.state) row.localeState = ev.state;
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
      model.materializedFiles = ev.files;
      break;
    case "log":
      if (ev.message) model.logs.push(ev.message);
      break;
    case "done":
      model.finalState = ev.state;
      model.done = true;
      break;
  }
  return model;
}

/** reduceRun folds an event log into a fresh render model. */
export function reduceRun(events: ConvergenceEvent[]): ConvergenceRunModel {
  const model = emptyRunModel();
  for (const ev of events) applyEvent(model, ev);
  return model;
}
