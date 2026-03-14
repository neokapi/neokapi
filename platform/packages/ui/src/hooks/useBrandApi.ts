import { useCallback } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  VoiceProfile,
  BrandComplianceScore,
  ScoreTrend,
  StoredScore,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
} from "../brand/types";

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
      void queryClient.invalidateQueries({ queryKey: ["brand-profile", ws, variables.id] });
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
