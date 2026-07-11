import type { RunEvent } from "../context/JobFeedContext";
// The convergence event protocol, the fold over it, and the render model are
// all shared across the Apache/AGPL boundary (@neokapi/status-views) — the
// desktop and bowrain's Runs surface reduce the same stream with the same state
// machine, and <ConvergenceRunView /> renders the result. All that lives here is
// the adapter from the desktop job feed's envelope to that stream.
import { reduceRun } from "@neokapi/status-views";
import type {
  ConvergenceLocaleRow as ConvergeLocaleRow,
  ConvergencePassView as ConvergePassView,
  ConvergenceRunModel as ConvergeRunModel,
  LocaleRowState,
} from "@neokapi/status-views";

export type { ConvergeLocaleRow, ConvergePassView, ConvergeRunModel, LocaleRowState };

/**
 * reduceConvergeRun folds a convergence run's job-feed events into the render
 * model: unwrap the typed `converge_event` envelopes the backend emits, then
 * hand the bare event stream to the shared fold.
 */
export function reduceConvergeRun(events: RunEvent[]): ConvergeRunModel {
  return reduceRun(
    events
      .filter((e) => e.type === "converge_event" && e.converge_event)
      .map((e) => e.converge_event!),
  );
}
