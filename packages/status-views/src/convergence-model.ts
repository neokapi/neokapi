// The normalized convergence-run render model shared across the Apache/AGPL
// boundary. Both sides fold their own event stream into this shape — kapi-desktop
// from the desktop job-feed's typed `converge_event`s, bowrain from the server's
// SSE `ConvergenceEvent`s — and pass it to <ConvergenceRunView />. The reducers
// stay app-side (they know their own wire types); only the render model and the
// presentational view are shared.

/** A locale's lifecycle within a pass, mirroring the CLI live renderer. */
export type LocaleRowState = "queued" | "running" | "done";

/** One locale's live cell in a pass: unit progress + the content memory/AI split. */
export interface ConvergenceLocaleRow {
  locale: string;
  /** Denominator a progress bar renders against (0 until locale_start). */
  units: number;
  /** Committed targets so far in this pass. */
  done: number;
  viaMemory: number;
  viaAI: number;
  state: LocaleRowState;
  /**
   * Authoritative per-locale ship verdict (shippable | parked | pending), when
   * the source carries one (bowrain's terminal standing / compat state events).
   * Undefined while a live stream is state-less — the badge falls back to
   * `state` (the coverage-derived lifecycle).
   */
  localeState?: string;
}

/** One pass of a run: its live locale rows plus the post-derivation summary. */
export interface ConvergencePassView {
  pass: number;
  /** Cap on passes (0 when unknown, e.g. a compat path). */
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
  /** Human-readable run log lines (auto-extract, transport notes). Optional. */
  logs?: string[];
  materializedFiles?: number;
  /** "converged" | "parked" once the run's done event arrived. */
  finalState?: string;
  /** True once the terminal `done` event has been folded in. */
  done?: boolean;
  /**
   * True when the model was built from a typed per-locale event stream (live
   * locale rows). Explicitly false for a compatibility path built from coarser
   * pass summaries (no per-locale rows); undefined when the distinction does not
   * apply.
   */
  live?: boolean;
}

/** A parked (locale, collection) scope — a deep-link target into review. */
export interface ParkedScope {
  locale: string;
  collection?: string;
}

/**
 * The structured terminal outcome of a run, when the caller has it out-of-band
 * from the reduced model (kapi-desktop reads it from the job feed's `complete`
 * event). Drives the converged/parked outcome block with parked-scope links.
 */
export interface ConvergenceOutcome {
  converged: boolean;
  passes: number;
  parkedScopes?: ParkedScope[];
  materializedFiles?: number;
  /** Translatable source blocks the run held below the project's source gate.
   *  Their translations were not produced, in any language: source content is
   *  shared, so this is one count for the whole fan-out rather than one per
   *  locale. */
  blockedOnSource?: number;
  /** The source gate the run applied (authored|checked|approved). */
  sourceGate?: string;
  /** Why the run did not converge. `source_not_ready` means every pending
   *  locale had nothing producible, because the source is unsettled. */
  stallReason?: string;
}

/** Run metadata for the optional header (trigger, id, timestamp). */
export interface ConvergenceRunHeader {
  id: string;
  trigger: string;
  created_at?: string;
}
