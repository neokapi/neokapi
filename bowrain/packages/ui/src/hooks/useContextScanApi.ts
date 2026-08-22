import { useMutation, useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ContextScanRequest, ContextScanStatus } from "../types/api";
import type { VoiceProfile } from "../voice/types";

/**
 * Poll cadence for a brand-scan job: refetch every 1.5 s while the job is
 * queued or processing, and stop once it reaches a terminal state.
 * Exported so the stop condition is unit-testable without a live query.
 */
export function contextScanPollInterval(status: ContextScanStatus | undefined): number | false {
  if (status === "completed" || status === "failed") return false;
  return 1500;
}

/** Upload brand-scan source files to the workspace blob store. */
export function useUploadContextScanSources() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (files: File[]) => api.uploadContextScanSources(ws, files),
  });
}

/** Enqueue a context scan; resolves with the job id to poll. */
export function useStartContextScan() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (req: ContextScanRequest) => api.startContextScan(ws, req),
  });
}

/** Poll a brand-scan job until it completes or fails. */
export function useContextScanJob(jobId: string | undefined) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-scan", ws, jobId],
    queryFn: () => api.getContextScan(ws, jobId ?? ""),
    enabled: !!ws && !!jobId,
    refetchInterval: (query) => contextScanPollInterval(query.state.data?.status),
  });
}

/** Stateless draft check for the live tester (zero AI cost). */
export function useCheckVoiceDraft() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: ({ profile, text }: { profile: VoiceProfile; text: string }) =>
      api.checkVoiceDraft(ws, profile, text),
  });
}
