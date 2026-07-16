import { useEffect, useState } from "react";
import { useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Card, ConvergenceRunsList, ConvergenceRunView, useApi, useWorkspace } from "@neokapi/ui";
import { convergenceRunsQueryOptions } from "../../queries";
import { useConvergenceRunEvents } from "../../hooks/useConvergenceRunEvents";

/**
 * Project-scoped Runs surface: the recent server-side convergence runs
 * ("the server runs `kapi up` for the team"), a "Run now" trigger, per-run
 * cancel, and a live detail pane driven by the selected run's SSE stream.
 */
export function RunsRoute() {
  const { projectId } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const queryClient = useQueryClient();
  const ws = activeWorkspace?.slug ?? "";

  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Runs — ${activeWorkspace.name} — Bowrain`;
    }
  }, [activeWorkspace]);

  const runsQuery = useQuery({
    ...convergenceRunsQueryOptions(api, ws, projectId ?? ""),
    enabled: !!ws && !!projectId,
  });
  const runs = runsQuery.data ?? [];

  // Default the detail pane to the newest run once the list loads.
  useEffect(() => {
    if (!selectedRunId && runs.length > 0) {
      setSelectedRunId(runs[0].id);
    }
  }, [runs, selectedRunId]);

  const startRun = useMutation({
    mutationFn: () => api.startConvergenceRun(ws, projectId ?? "", { trigger: "manual" }),
    onSuccess: (run) => {
      setSelectedRunId(run.id);
      void queryClient.invalidateQueries({ queryKey: ["convergenceRuns", ws, projectId] });
    },
  });

  const cancelRun = useMutation({
    mutationFn: (runId: string) => api.cancelConvergenceRun(ws, projectId ?? "", runId),
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: ["convergenceRuns", ws, projectId] }),
  });

  // As a live run reaches its terminal event, refresh the list so its row
  // shows the settled state/timestamps.
  const { model, connecting } = useConvergenceRunEvents(ws, projectId ?? undefined, selectedRunId);
  const runDone = model.done;
  useEffect(() => {
    if (runDone) {
      void queryClient.invalidateQueries({ queryKey: ["convergenceRuns", ws, projectId] });
    }
  }, [runDone, ws, projectId, queryClient]);

  if (!activeWorkspace || !projectId) {
    return (
      <Card className="mt-8 max-w-md mx-auto p-8 text-center text-muted-foreground text-sm">
        Select a project to view runs
      </Card>
    );
  }

  const selectedRun = runs.find((r) => r.id === selectedRunId);

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6 space-y-6">
      <ConvergenceRunsList
        runs={runs}
        loading={runsQuery.isLoading}
        selectedRunId={selectedRunId}
        starting={startRun.isPending}
        cancelingRunId={cancelRun.isPending ? cancelRun.variables : null}
        onSelect={setSelectedRunId}
        onRunNow={() => startRun.mutate()}
        onCancel={(runId) => cancelRun.mutate(runId)}
      />

      {selectedRunId && (
        <Card className="p-4">
          <ConvergenceRunView
            model={model}
            run={selectedRun}
            connecting={connecting}
            running={!model.done && !connecting}
          />
        </Card>
      )}
    </div>
  );
}
