// The convergence event protocol and the one fold over it, shared across the
// Apache/AGPL boundary.
//
// core/convergence.Event is the single protocol every venue speaks — the CLI's
// live renderer, `kapi up --json`, the desktop's job feed, the server's SSE
// stream. Every surface therefore needs the same fold: events in, render model
// out. It lives here, once. (It used to live three times — kapi-desktop,
// bowrain's UI package, and Go — and a locale that read "pending" on one
// surface and "shippable" on another was the predictable result.)
//
// The fold is pure: no network, no DOM. Callers adapt their own transport
// (Wails job feed, EventSource) into ConvergenceEvent and fold it here.

import type {
  ConvergenceLocaleRow,
  ConvergencePassView,
  ConvergenceRunModel,
  LocaleRowState,
} from "./convergence-model";

/** Discriminator for a convergence run's progress events. */
export type ConvergenceEventType =
  | "pass_start"
  | "locale_start"
  | "unit_progress"
  | "locale_done"
  | "pass_done"
  | "materialized"
  | "log"
  | "done";

/**
 * One progress event of a convergence run — the flat, type-discriminated
 * protocol (core/convergence.Event). Fields are populated per `type` and
 * omitted when zero.
 *
 * A run is a sequence of passes; within a pass every pending locale runs the
 * default flow (concurrently), and after the pass coverage is re-derived with
 * the project's bound checks. The stream ends with exactly one `done`.
 */
export interface ConvergenceEvent {
  type: ConvergenceEventType;

  // Pass-scoped (pass_start, pass_done; `pass` stamps locale-scoped events too).
  pass?: number;
  maxPasses?: number;
  pending?: string[];

  // Pre-pass auto-extract on drift (pass_start).
  extractedFiles?: number;
  extractedBlocks?: number;

  // Locale-scoped (locale_start, unit_progress, locale_done).
  locale?: string;
  units?: number;
  done?: number;
  viaTM?: number;
  viaAI?: number;

  // Post-derivation (pass_done).
  produced?: number;
  producedDelta?: number;
  failingChecks?: number;

  // locale_done → shippable|parked|pending; done → converged|parked|failed|canceled.
  state?: string;

  // Materialized file count (materialized).
  files?: number;

  // Log line (log).
  message?: string;
}

/** A fresh, empty run model — the fold's identity. */
export function emptyRunModel(): ConvergenceRunModel {
  return { passes: [], logs: [], done: false, live: true };
}

function newRow(locale: string): ConvergenceLocaleRow {
  return { locale, units: 0, done: 0, viaTM: 0, viaAI: 0, state: "queued" };
}

/**
 * deriveRowState derives a locale's lifecycle from coverage alone. The live
 * stream does not carry a per-locale ship verdict, so the streaming badge is
 * driven by progress: all units covered → done, some work started → running,
 * else queued. The authoritative shippable/parked verdict arrives separately
 * (a run's terminal standing), never inferred here.
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
 * model is the accumulator; a caller that needs immutability (React) clones the
 * snapshot it hands to render.
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
        model.logs?.push(
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
        // The live stream is state-less; only adopt a ship verdict when the
        // event actually carries one. Never clobber a prior verdict with
        // undefined.
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
      if (ev.message) model.logs?.push(ev.message);
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
