// @neokapi/status-views — framework-level, data-source-agnostic UI for the
// localization status surfaces both desktops and the web app rebuild across the
// Apache/AGPL boundary (Apache-2.0). Views are purely presentational: the caller
// folds its own event/SSE stream into the shared render model and passes it in.

export { ConvergenceRunView } from "./ConvergenceRunView";
export type { ConvergenceRunViewProps } from "./ConvergenceRunView";

export type {
  ConvergenceRunModel,
  ConvergencePassView,
  ConvergenceLocaleRow,
  LocaleRowState,
  ConvergenceOutcome,
  ConvergenceRunHeader,
  ParkedScope,
} from "./convergence-model";

// The convergence event protocol and the one fold over it. Every surface that
// renders a run — kapi-desktop's job feed, bowrain's SSE stream — folds with
// this rather than carrying its own copy of the state machine.
export { reduceRun, applyEvent, emptyRunModel } from "./convergence-fold";
export type { ConvergenceEvent, ConvergenceEventType } from "./convergence-fold";
