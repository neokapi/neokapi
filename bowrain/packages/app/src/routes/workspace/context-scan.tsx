import { useCallback, useEffect } from "react";
import { useNavigate, useParams, useRouteContext } from "@tanstack/react-router";
import { ContextScanInput, ContextScanLocalLaneCard } from "@neokapi/ui";
import type { ContextScanRequest } from "@neokapi/ui";
import type { WorkspaceRouteContext } from "..";

/**
 * SessionStorage key holding the request that produced a scan job, so the
 * review page can regenerate with the same request. The server stores the
 * request too, but its job GET intentionally returns only status + draft.
 */
export function contextScanRequestKey(jobId: string): string {
  return `brand-scan-request:${jobId}`;
}

/** Best-effort persistence — a full sessionStorage must not break the flow. */
export function rememberContextScanRequest(jobId: string, request: ContextScanRequest): void {
  try {
    sessionStorage.setItem(contextScanRequestKey(jobId), JSON.stringify(request));
  } catch {
    // Regeneration falls back to the scan input page.
  }
}

export function ContextScanRoute() {
  const navigate = useNavigate();
  const { workspace } = useParams({ strict: false });
  const { activeWorkspace, contextScanAvailable } = useRouteContext({
    strict: false,
  }) as WorkspaceRouteContext;

  useEffect(() => {
    if (activeWorkspace) {
      document.title = `Scan your brand · ${activeWorkspace.name} · Bowrain`;
    }
  }, [activeWorkspace]);

  const handleStarted = useCallback(
    (jobId: string, request: ContextScanRequest) => {
      rememberContextScanRequest(jobId, request);
      void navigate({
        to: "/$workspace/context/scan/$jobId",
        params: { workspace: workspace ?? "", jobId },
      });
    },
    [navigate, workspace],
  );

  const handleLocalLane = useCallback(() => {
    void navigate({
      to: "/$workspace/context/voice/mcp-guide",
      params: { workspace: workspace ?? "" },
    });
  }, [navigate, workspace]);

  // The entry-point CTAs are hidden when the capability is absent, but the
  // page stays reachable by URL — explain instead of failing with a raw 503.
  if (!contextScanAvailable) {
    return (
      <div className="max-w-3xl mx-auto space-y-6">
        <div>
          <h1 className="text-lg font-semibold">Scan your brand</h1>
          <p className="text-sm text-muted-foreground mt-1">
            The hosted context scan is not available on this server: it requires the background job
            system (PostgreSQL and a job queue), which this deployment does not run. You can still
            create a voice profile by hand, or draft one locally.
          </p>
        </div>
        <ContextScanLocalLaneCard onLearnMore={handleLocalLane} />
      </div>
    );
  }

  return (
    <div className="max-w-3xl mx-auto space-y-6">
      <div>
        <h1 className="text-lg font-semibold">Scan your brand</h1>
        <p className="text-sm text-muted-foreground mt-1">
          Give the scan anything that carries your voice: pasted text, web pages, marketing files,
          or a repository. It drafts a voice profile and candidate terms, with the evidence behind
          each field; nothing is saved until you review and approve.
        </p>
      </div>
      <ContextScanInput onStarted={handleStarted} />
    </div>
  );
}
