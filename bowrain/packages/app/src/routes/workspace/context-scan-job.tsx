import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import {
  ContextScanProgress,
  ContextScanReview,
  DashboardSkeleton,
  PageHeader,
  useApi,
  useContextScanJob,
} from "@neokapi/ui";
import type { ContextScanRequest, VoiceProfile } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";
import { contextScanRequestKey, rememberContextScanRequest } from "./context-scan";

export function ContextScanJobRoute() {
  const navigate = useNavigate();
  const { workspace, jobId } = useParams({ strict: false });
  const api = useApi();
  const { activeWorkspace } = useRouteContext({ strict: false }) as WorkspaceRouteContext;
  const ws = activeWorkspace?.slug ?? "";

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Context scan · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const { data: job, isLoading } = useContextScanJob(jobId);

  /** Re-enqueue with the request that produced this job; if it is no longer
   * in sessionStorage, fall back to the scan input page. */
  const handleRegenerate = useCallback(async () => {
    let request: ContextScanRequest | null = null;
    if (jobId) {
      try {
        const raw = sessionStorage.getItem(contextScanRequestKey(jobId));
        if (raw) request = JSON.parse(raw) as ContextScanRequest;
      } catch {
        request = null;
      }
    }
    if (!request) {
      void navigate({
        to: "/$workspace/context/scan",
        params: { workspace: workspace ?? "" },
      });
      return;
    }
    const { job_id } = await api.startContextScan(ws, request);
    rememberContextScanRequest(job_id, request);
    void navigate({
      to: "/$workspace/context/scan/$jobId",
      params: { workspace: workspace ?? "", jobId: job_id },
    });
  }, [api, ws, jobId, navigate, workspace]);

  const handleApproved = useCallback(
    (profile: VoiceProfile) => {
      void navigate({
        to: "/$workspace/context/voice/$profileId",
        params: { workspace: workspace ?? "", profileId: profile.id },
      });
    },
    [navigate, workspace],
  );

  if (isLoading || !job) {
    return <DashboardSkeleton />;
  }

  if (job.status === "completed" && job.draft) {
    return (
      <div className="max-w-6xl mx-auto space-y-6">
        <PageHeader
          title="Review your draft"
          subtitle="Every field shows the model's confidence and the evidence it rests on. Adjust anything, test it on sample copy, then approve to create the profile and the selected terms."
        />
        <ContextScanReview
          draft={job.draft}
          onApproved={handleApproved}
          onRegenerate={() => void handleRegenerate()}
        />
      </div>
    );
  }

  return (
    <div className="max-w-xl mx-auto pt-8">
      <ContextScanProgress
        job={job}
        onRetry={job.status === "failed" ? () => void handleRegenerate() : undefined}
      />
    </div>
  );
}
