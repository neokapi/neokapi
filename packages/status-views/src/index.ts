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
