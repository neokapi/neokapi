import { ConvergenceRunView } from "@neokapi/status-views";
import type { RunEvent } from "../context/JobFeedContext";
import type { ConvergeOutput } from "../types/api";
import { reduceConvergeRun } from "../lib/convergeRun";

export interface ConvergeRunViewProps {
  /** The run's event stream (from the job feed). */
  events: RunEvent[];
  /** Whether the run is still in flight (renders the working row). */
  running?: boolean;
  /** Whether the run was cancelled (renders the terminal cancelled row). */
  canceled?: boolean;
  /** Open the Review page filtered to a parked (collection, locale) scope. */
  onOpenReview?: (scope: { collection?: string; locale?: string }) => void;
}

/**
 * The desktop adapter over the shared <ConvergenceRunView />: it reduces the job
 * feed's typed convergence-event stream into the shared render model and lifts
 * the run's structured outcome (from the terminal `complete` event) into the
 * shared outcome shape. All presentation lives in @neokapi/status-views, shared
 * with bowrain's Runs surface.
 */
export function ConvergeRunView({ events, running, canceled, onOpenReview }: ConvergeRunViewProps) {
  const model = reduceConvergeRun(events);
  const result: ConvergeOutput | undefined = events.find(
    (e) => e.type === "complete" && e.converge_result,
  )?.converge_result;

  return (
    <ConvergenceRunView
      model={model}
      running={running}
      canceled={canceled}
      onOpenReview={onOpenReview}
      result={
        result
          ? {
              converged: result.converged,
              passes: result.passes,
              parkedScopes: result.parkedScopes,
              materializedFiles: result.materializedFiles,
              blockedOnSource: result.blockedOnSource,
              sourceGate: result.sourceGate,
              stallReason: result.stallReason,
            }
          : undefined
      }
    />
  );
}
