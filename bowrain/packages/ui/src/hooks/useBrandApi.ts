import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { CreateVoiceProfileRequest, UpdateVoiceProfileRequest } from "../brand/types";

export function useBrandProfiles() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-profiles", ws],
    queryFn: () => api.listBrandProfiles(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

export function useBrandProfile(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-profile", ws, profileId],
    queryFn: () => api.getBrandProfile(ws, profileId),
    enabled: !!ws && !!profileId,
    staleTime: 30_000,
  });
}

export function useCreateBrandProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateVoiceProfileRequest) => api.createBrandProfile(ws, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["brand-profiles", ws] });
    },
  });
}

export function useUpdateBrandProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: UpdateVoiceProfileRequest) => api.updateBrandProfile(ws, data),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["brand-profiles", ws] });
      void queryClient.invalidateQueries({
        queryKey: ["brand-profile", ws, variables.id],
      });
    },
  });
}

export function useDeleteBrandProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (profileId: string) => api.deleteBrandProfile(ws, profileId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["brand-profiles", ws] });
    },
  });
}

export function useBrandScores(projectId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-scores", ws, projectId],
    queryFn: () => api.getBrandScores(ws, projectId),
    enabled: !!ws && !!projectId,
    staleTime: 30_000,
  });
}

export function useBrandTrends(projectId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-trends", ws, projectId],
    queryFn: () => api.getBrandTrends(ws, projectId),
    enabled: !!ws && !!projectId,
    staleTime: 60_000,
  });
}

// ── Correction-learning loop (AD-019) ──────────────────────────────────────

export function useBrandCandidates(profileId: string, opts?: { minCount?: number; all?: boolean }) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-candidates", ws, profileId, opts?.minCount ?? 3, opts?.all ?? false],
    queryFn: () => api.listBrandCandidates(ws, profileId, opts),
    enabled: !!ws && !!profileId,
    staleTime: 15_000,
  });
}

export function usePromoteBrandRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (rule: { term: string; replacement?: string; correction_count?: number }) =>
      api.promoteBrandRule(ws, profileId, rule),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["brand-candidates", ws, profileId] });
      void queryClient.invalidateQueries({ queryKey: ["brand-profile", ws, profileId] });
    },
  });
}

export function useRejectBrandRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (rule: { term: string; replacement?: string }) =>
      api.rejectBrandRule(ws, profileId, rule),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["brand-candidates", ws, profileId] });
    },
  });
}

export function useEvaluateBrandRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (req: {
      term: string;
      replacement?: string;
      project_id: string;
      stream?: string;
    }) => api.evaluateBrandRule(ws, profileId, req),
  });
}

export function useBrandDrift(
  projectId: string,
  opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["brand-drift", ws, projectId, opts ?? {}],
    queryFn: () => api.getBrandDrift(ws, projectId, opts),
    enabled: !!ws && !!projectId,
    staleTime: 60_000,
  });
}

export function useStarterPacks() {
  const api = useApi();

  return useQuery({
    queryKey: ["brand-starter-packs"],
    queryFn: () => api.listStarterPacks(),
    staleTime: 300_000,
  });
}

export function useCreateFromStarter() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (params: { pack: string; name?: string }) =>
      api.createProfileFromStarter(ws, params.pack, params.name),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["brand-profiles", ws] });
    },
  });
}
