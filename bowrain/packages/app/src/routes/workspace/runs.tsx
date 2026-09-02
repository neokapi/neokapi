import { useEffect, useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AnalyticsEvents,
  Card,
  ConvergenceRunContext,
  ConvergenceRunNowDialog,
  ConvergenceRunsList,
  ConvergenceRunView,
  countBucket,
  creditBucket,
  sharePercentBucket,
  useAnalytics,
  useApi,
  useWorkspace,
} from "@neokapi/ui";
import type { ConvergenceRunScope } from "@neokapi/ui";
import {
  convergenceEstimateQueryOptions,
  convergenceRunsQueryOptions,
  projectQueryOptions,
} from "../../queries";
import { useConvergenceRunEvents } from "../../hooks/useConvergenceRunEvents";
import { usePlatform } from "../../platform";

/**
 * Project-scoped Runs surface: the recent server-side convergence runs
 * ("the server runs `kapi up` for the team"), a source-readiness-first Run-now
 * consent gate (epic 019), per-run cancel, and a live detail pane driven by the
 * selected run's SSE stream. The detail pane renders the run's loop position and
 * a labeled stall/hold banner with a next action — never a silent spinner.
 */
export function RunsRoute() {
  const { projectId, stream } = useParams({ strict: false });
  const { activeWorkspace } = useWorkspace();
  const api = useApi();
  const platform = usePlatform();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { capture } = useAnalytics();
  const ws = activeWorkspace?.slug ?? "";

  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const [consentOpen, setConsentOpen] = useState(false);

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Runs · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const runsQuery = useQuery({
    ...convergenceRunsQueryOptions(api, ws, projectId ?? ""),
    enabled: !!ws && !!projectId,
  });
  const runs = runsQuery.data ?? [];

  // The project's review-workflow setting gates the awaiting-review element on
  // completed runs. Governed review is opt-out: only an explicit "false" hides
  // it (including while the project is still loading).
  const projectQuery = useQuery({
    ...projectQueryOptions(api, ws, projectId ?? "", stream),
    enabled: !!ws && !!projectId,
  });
  const reviewEnabled = projectQuery.data?.properties?.workflow_enabled !== "false";

  // The pre-flight estimate — fetched only while the consent dialog is open.
  const estimateQuery = useQuery({
    ...convergenceEstimateQueryOptions(api, ws, projectId ?? ""),
    enabled: consentOpen && !!ws && !!projectId,
  });
  const estimate = estimateQuery.data;

  // Default the detail pane to the newest run once the list loads.
  useEffect(() => {
    if (!selectedRunId && runs.length > 0) {
      setSelectedRunId(runs[0].id);
    }
  }, [runs, selectedRunId]);

  // Fire the estimate-viewed event once the estimate lands in the open dialog.
  // Beyond the yes/no coverage flag it carries the BUCKETED magnitude of the
  // work the run would do — what it would cost, how many units are left for AI
  // after TM recycling, and TM's share — which is what sizes the one-time
  // new-workspace credit grant. Bucketed only: never an exact credit amount,
  // token count, or anything content-derived.
  useEffect(() => {
    if (consentOpen && estimate) {
      const viaTm = estimate.totals?.via_tm ?? 0;
      const viaAi = estimate.totals?.via_ai ?? 0;
      // Where billing is unconfigured (self-hosted) the estimate carries no
      // credit section; the demand is still the token estimate (1 token ≈ 1
      // credit), so fall back to it rather than reporting a false zero.
      const credits = estimate.credits?.estimated_credits ?? estimate.totals?.token_estimate ?? 0;
      capture(AnalyticsEvents.convergenceEstimateViewed, {
        source_held: (estimate.source.held ?? 0) > 0,
        covers_all_ai: estimate.credits?.covers_all_ai ?? true,
        estimated_credits_bucket: creditBucket(credits),
        ai_units_bucket: countBucket(viaAi),
        tm_leverage_pct_bucket: sharePercentBucket(viaTm, viaTm + viaAi),
      });
    }
  }, [consentOpen, estimate, capture]);

  const startRun = useMutation({
    mutationFn: (scope: ConvergenceRunScope) =>
      api.startConvergenceRun(ws, projectId ?? "", { trigger: "manual", scope, confirmed: true }),
    onSuccess: (run, scope) => {
      capture(AnalyticsEvents.convergenceRunStarted, {
        scope,
        source_held: (estimate?.source.held ?? 0) > 0,
      });
      setConsentOpen(false);
      // Transport-only (scope "none") starts no run — the server answered 204 and
      // the mutation resolves null; just refresh the list.
      if (run) setSelectedRunId(run.id);
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
  // The run stream is a cookie-authenticated same-origin EventSource (web only);
  // the desktop reaches the server over Wails, so keep it off there and let the
  // list settle via the terminal-state invalidation below.
  const { model, connecting } = useConvergenceRunEvents(
    platform.kind === "web" ? ws : undefined,
    projectId ?? undefined,
    selectedRunId,
  );
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

  // Next-action navigation for the stall/hold banner + consent dialog. Source
  // review lands in the workspace task queue (AD-014); credits in settings.
  const goToTasks = () => void navigate({ to: "/$workspace/tasks", params: { workspace: ws } });
  const goToBilling = () =>
    void navigate({ to: "/$workspace/settings/billing", params: { workspace: ws } });
  // The awaiting-review hand-off deep-links into the dedicated review session
  // (all pending blocks across the project's items + locales), not the moded
  // translation editor.
  const goToReviewSurface = () =>
    void navigate({
      to: "/$workspace/p/$projectId/s/$stream/review",
      params: { workspace: ws, projectId: projectId ?? "", stream: stream ?? "main" },
    });

  return (
    <div className="mx-auto w-full max-w-5xl p-4 md:p-6 space-y-6">
      <ConvergenceRunsList
        runs={runs}
        loading={runsQuery.isLoading}
        selectedRunId={selectedRunId}
        starting={startRun.isPending}
        cancelingRunId={cancelRun.isPending ? cancelRun.variables : null}
        onSelect={setSelectedRunId}
        onRunNow={() => setConsentOpen(true)}
        onCancel={(runId) => cancelRun.mutate(runId)}
      />

      {selectedRunId && selectedRun && (
        <Card className="p-4 space-y-4">
          <ConvergenceRunContext
            run={selectedRun}
            live={platform.kind === "web" && !model.done}
            reviewEnabled={reviewEnabled}
            onSettleSource={goToTasks}
            onBuyCredits={goToBilling}
            onOpenReview={goToReviewSurface}
            onOpenReviewSurface={goToReviewSurface}
          />
          <ConvergenceRunView
            model={model}
            run={selectedRun}
            connecting={connecting}
            running={!model.done && !connecting}
          />
        </Card>
      )}

      <ConvergenceRunNowDialog
        open={consentOpen}
        onOpenChange={setConsentOpen}
        estimate={estimate}
        loading={estimateQuery.isLoading}
        error={estimateQuery.error ? String(estimateQuery.error) : undefined}
        starting={startRun.isPending}
        onStart={(scope) => startRun.mutate(scope)}
        onSettleSource={goToTasks}
        onBuyCredits={goToBilling}
      />
    </div>
  );
}
