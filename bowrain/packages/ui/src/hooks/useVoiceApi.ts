import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type {
  VoiceRollupOptions,
  VoiceCorrectionRequest,
  CreateVoiceProfileRequest,
  UpdateVoiceProfileRequest,
} from "../voice/types";

export function useVoiceProfiles() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-profiles", ws],
    queryFn: () => api.listVoiceProfiles(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

export function useVoiceProfile(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice profile", ws, profileId],
    queryFn: () => api.getVoiceProfile(ws, profileId),
    enabled: !!ws && !!profileId,
    staleTime: 30_000,
  });
}

export function useCreateVoiceProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: CreateVoiceProfileRequest) => api.createVoiceProfile(ws, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["voice-profiles", ws] });
    },
  });
}

export function useUpdateVoiceProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (data: UpdateVoiceProfileRequest) => api.updateVoiceProfile(ws, data),
    onSuccess: (_result, variables) => {
      void queryClient.invalidateQueries({ queryKey: ["voice-profiles", ws] });
      void queryClient.invalidateQueries({
        queryKey: ["voice profile", ws, variables.id],
      });
    },
  });
}

export function useDeleteVoiceProfile() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (profileId: string) => api.deleteVoiceProfile(ws, profileId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["voice-profiles", ws] });
    },
  });
}

export function useVoiceScores(projectId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-scores", ws, projectId],
    queryFn: () => api.getVoiceScores(ws, projectId),
    enabled: !!ws && !!projectId,
    staleTime: 30_000,
  });
}

export function useVoiceRollup(opts?: VoiceRollupOptions) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-rollup", ws, opts ?? {}],
    queryFn: () => api.getVoiceRollup(ws, opts),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

export function useVoiceTrends(projectId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-trends", ws, projectId],
    queryFn: () => api.getVoiceTrends(ws, projectId),
    enabled: !!ws && !!projectId,
    staleTime: 60_000,
  });
}

// ── Correction-learning loop (AD-019) ──────────────────────────────────────

/**
 * Record a reviewer's in-place correction (original → corrected) into the
 * correction-learning loop for a project's bound voice profile. Repeated
 * corrections surface as candidate rules and auto-promote past the profile's
 * threshold, so a reviewer's fix becomes a check on every future generation.
 */
export function useRecordVoiceCorrection(projectId: string, stream?: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (req: VoiceCorrectionRequest) =>
      api.recordVoiceCorrection(ws, projectId, req, stream),
    onSuccess: (_result, req) => {
      void queryClient.invalidateQueries({ queryKey: ["voice-candidates", ws, req.profile_id] });
      void queryClient.invalidateQueries({ queryKey: ["voice profile", ws, req.profile_id] });
    },
  });
}

export function useVoiceCandidates(profileId: string, opts?: { minCount?: number; all?: boolean }) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-candidates", ws, profileId, opts?.minCount ?? 3, opts?.all ?? false],
    queryFn: () => api.listVoiceCandidates(ws, profileId, opts),
    enabled: !!ws && !!profileId,
    staleTime: 15_000,
  });
}

export function usePromoteVoiceRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (rule: { term: string; replacement?: string; correction_count?: number }) =>
      api.promoteVoiceRule(ws, profileId, rule),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["voice-candidates", ws, profileId] });
      void queryClient.invalidateQueries({ queryKey: ["voice profile", ws, profileId] });
    },
  });
}

export function useRejectVoiceRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (rule: { term: string; replacement?: string }) =>
      api.rejectVoiceRule(ws, profileId, rule),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["voice-candidates", ws, profileId] });
    },
  });
}

export function useEvaluateVoiceRule(profileId: string) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (req: {
      term: string;
      replacement?: string;
      project_id: string;
      stream?: string;
    }) => api.evaluateVoiceRule(ws, profileId, req),
  });
}

export function useVoiceDrift(
  projectId: string,
  opts?: { recentDays?: number; minScore?: number; dropPoints?: number },
) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery({
    queryKey: ["voice-drift", ws, projectId, opts ?? {}],
    queryFn: () => api.getVoiceDrift(ws, projectId, opts),
    enabled: !!ws && !!projectId,
    staleTime: 60_000,
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
      void queryClient.invalidateQueries({ queryKey: ["voice-profiles", ws] });
    },
  });
}
