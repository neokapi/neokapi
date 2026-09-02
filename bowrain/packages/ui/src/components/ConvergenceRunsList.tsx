import {
  Badge,
  Button,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
  cn,
} from "@neokapi/ui-primitives";
import type { ConvergenceRun, ConvergenceRunState } from "../types/api";

const runStateColor: Record<ConvergenceRunState, string> = {
  running: "bg-info/10 text-info dark:bg-info/20 dark:text-info",
  converged: "bg-success/10 text-success dark:bg-success/20 dark:text-success",
  parked: "bg-warning/10 text-warning dark:bg-warning/20 dark:text-warning",
  canceled: "bg-muted text-muted-foreground",
  failed: "bg-destructive/10 text-destructive dark:bg-destructive/20 dark:text-destructive",
};

// Display labels for the machine run states — the wire value `converged` reads
// as the product's "up to date"; the rest keep their plain casing.
const runStateLabel: Record<ConvergenceRunState, string> = {
  running: "Running",
  converged: "Up to date",
  parked: "Parked",
  canceled: "Canceled",
  failed: "Failed",
};

function triggerLabel(trigger: string): string {
  const labels: Record<string, string> = {
    manual: "Manual",
    cli: "kapi up",
    push: "On push",
  };
  return labels[trigger] || trigger;
}

/** Compact "3 shippable · 1 parked" summary from a run's locale standing. */
function localeSummary(run: ConvergenceRun): string {
  const locales = run.locales ?? [];
  if (locales.length === 0) return "·";
  const counts: Record<string, number> = {};
  for (const l of locales) counts[l.state] = (counts[l.state] ?? 0) + 1;
  const order = ["shippable", "pending", "parked"];
  const parts = order.filter((s) => counts[s]).map((s) => `${counts[s]} ${s}`);
  return parts.length > 0 ? parts.join(" · ") : `${locales.length} locales`;
}

function ts(value?: string): string {
  if (!value) return "·";
  return new Date(value).toLocaleString();
}

export interface ConvergenceRunsListProps {
  runs: ConvergenceRun[];
  loading?: boolean;
  selectedRunId?: string | null;
  starting?: boolean;
  cancelingRunId?: string | null;
  onSelect: (runId: string) => void;
  onRunNow: () => void;
  onCancel: (runId: string) => void;
}

/**
 * ConvergenceRunsList shows a project's recent server-side convergence runs
 * (newest first) with state, trigger, pass count, per-locale summary, and
 * start/finish times. A "Run now" button starts a run; each in-flight run gets
 * a Cancel. Presentational — data + mutations are wired by the caller.
 */
export function ConvergenceRunsList({
  runs,
  loading,
  selectedRunId,
  starting,
  cancelingRunId,
  onSelect,
  onRunNow,
  onCancel,
}: ConvergenceRunsListProps) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">Runs</h2>
          <p className="text-[11px] text-muted-foreground">
            Runs on the server: the team's <code>kapi up</code> for this project.
          </p>
        </div>
        <Button size="sm" onClick={onRunNow} disabled={starting}>
          {starting ? "Starting…" : "Run now"}
        </Button>
      </div>

      {loading ? (
        <div className="space-y-2">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-10 w-full" />
          ))}
        </div>
      ) : runs.length === 0 ? (
        <div className="text-sm text-muted-foreground border border-dashed border-border rounded-md p-6 text-center">
          No runs yet. Start one with “Run now”, or run <code>kapi up</code> locally.
        </div>
      ) : (
        <div className="border border-border rounded-md overflow-hidden">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>State</TableHead>
                <TableHead>Trigger</TableHead>
                <TableHead className="text-right">Passes</TableHead>
                <TableHead>Locales</TableHead>
                <TableHead>Started</TableHead>
                <TableHead>Finished</TableHead>
                <TableHead className="text-right" />
              </TableRow>
            </TableHeader>
            <TableBody>
              {runs.map((run) => (
                <TableRow
                  key={run.id}
                  onClick={() => onSelect(run.id)}
                  className={cn("cursor-pointer", selectedRunId === run.id && "bg-accent/40")}
                >
                  <TableCell>
                    <Badge className={cn("text-[10px]", runStateColor[run.state])}>
                      {runStateLabel[run.state] ?? run.state}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{triggerLabel(run.trigger)}</TableCell>
                  <TableCell className="text-xs text-right tabular-nums">{run.passes}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {localeSummary(run)}
                    {run.failing_checks ? (
                      <span className="text-destructive"> · {run.failing_checks} failing</span>
                    ) : null}
                  </TableCell>
                  <TableCell className="text-[11px] text-muted-foreground">
                    {ts(run.created_at)}
                  </TableCell>
                  <TableCell className="text-[11px] text-muted-foreground">
                    {ts(run.finished_at)}
                  </TableCell>
                  <TableCell className="text-right">
                    {run.state === "running" && (
                      <Button
                        size="sm"
                        variant="ghost"
                        className="h-7 text-destructive hover:text-destructive"
                        disabled={cancelingRunId === run.id}
                        onClick={(e) => {
                          e.stopPropagation();
                          onCancel(run.id);
                        }}
                      >
                        {cancelingRunId === run.id ? "Canceling…" : "Cancel"}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
