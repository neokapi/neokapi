import { useMutation, useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { BrandScanRequest, BrandScanStatus } from "../types/api";
import type { VoiceProfile } from "../brand/types";

/**
 * Poll cadence for a brand-scan job: refetch every 1.5 s while the job is
 * queued or processing, and stop once it reaches a terminal state.
 * Exported so the stop condition is unit-testable without a live query.
 */
export function brandScanPollInterval(status: BrandScanStatus | undefined): number | false {
  if (status === "completed" || status === "failed") return false;
  return 1500;
}

/** Upload brand-scan source files to the workspace blob store. */
export function useUploadBrandScanSources() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (files: File[]) => api.uploadBrandScanSources(ws, files),
  });
}

/** Enqueue a brand scan; resolves with the job id to poll. */
export function useStartBrandScan() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (req: BrandScanRequest) => api.startBrandScan(ws, req),
  });
}

/** Poll a brand-scan job until it completes or fails. */
export function useBrandScanJob(jobId: string | undefined) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-scan", ws, jobId],
    queryFn: () => api.getBrandScan(ws, jobId ?? ""),
    enabled: !!ws && !!jobId,
    refetchInterval: (query) => brandScanPollInterval(query.state.data?.status),
  });
}

/** Stateless draft check for the live tester (zero AI cost). */
export function useCheckBrandDraft() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: ({ profile, text }: { profile: VoiceProfile; text: string }) =>
      api.checkBrandDraft(ws, profile, text),
  });
}
