// Change-set (experiment) hooks for the Brand knowledge graph (AD-021): the
// draft → in_review → approved → merged lifecycle, its ordered ops, reviews,
// pilots, and the blast-radius preview over stored content.
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import { invalidateConceptAggregates } from "./useConceptsApi";
import type {
  ChangeSetStatus,
  CreateChangeSetRequest,
  UpdateChangeSetRequest,
  AddChangeSetOpRequest,
  ReviewRequest,
  StartPilotRequest,
} from "../types/brand-graph";

function useWs() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return { api, ws };
}

// ── List + single change-set ────────────────────────────────────────────────

export function useChangesets(status?: ChangeSetStatus) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["changesets", ws, status ?? "all"],
    queryFn: () => api.listChangesets(ws, status),
    enabled: !!ws,
    staleTime: 10_000,
  });
}

/**
 * The workspace's change-sets bucketed by lifecycle status, counted over every
 * change-set rather than over the fetched page — buckets an active filter
 * excludes would otherwise read as empty rather than unknown.
 */
export function useChangesetCounts() {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["changeset-counts", ws],
    queryFn: () => api.getChangesetCounts(ws),
    enabled: !!ws,
    staleTime: 10_000,
  });
}

export function useChangeset(changesetId: string) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["changeset", ws, changesetId],
    queryFn: () => api.getChangeset(ws, changesetId),
    enabled: !!ws && !!changesetId,
    staleTime: 5_000,
  });
}

export function useCreateChangeset() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: CreateChangeSetRequest) => api.createChangeset(ws, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["changesets", ws] });
      void qc.invalidateQueries({ queryKey: ["changeset-counts", ws] });
    },
  });
}

export function usePatchChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: UpdateChangeSetRequest) => api.patchChangeset(ws, changesetId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["changeset", ws, changesetId] });
      void qc.invalidateQueries({ queryKey: ["changesets", ws] });
      void qc.invalidateQueries({ queryKey: ["changesetCounts", ws] });
    },
  });
}

// ── Ops ──────────────────────────────────────────────────────────────────────

function invalidateAfterOpEdit(
  qc: ReturnType<typeof useQueryClient>,
  ws: string,
  changesetId: string,
) {
  void qc.invalidateQueries({ queryKey: ["changeset", ws, changesetId] });
  void qc.invalidateQueries({ queryKey: ["changeset-blast-radius", ws, changesetId] });
}

export function useAppendChangesetOp(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddChangeSetOpRequest) => api.appendChangesetOp(ws, changesetId, req),
    onSuccess: () => invalidateAfterOpEdit(qc, ws, changesetId),
  });
}

export function useRemoveChangesetOp(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (seq: number) => api.removeChangesetOp(ws, changesetId, seq),
    onSuccess: () => invalidateAfterOpEdit(qc, ws, changesetId),
  });
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

function invalidateLifecycle(
  qc: ReturnType<typeof useQueryClient>,
  ws: string,
  changesetId: string,
) {
  void qc.invalidateQueries({ queryKey: ["changeset", ws, changesetId] });
  void qc.invalidateQueries({ queryKey: ["changesets", ws] });
  // A lifecycle move changes which bucket the change-set counts under.
  void qc.invalidateQueries({ queryKey: ["changeset-counts", ws] });
}

export function useSubmitChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.submitChangeset(ws, changesetId),
    onSuccess: () => invalidateLifecycle(qc, ws, changesetId),
  });
}

export function useApproveChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req?: ReviewRequest) => api.approveChangeset(ws, changesetId, req),
    onSuccess: () => invalidateLifecycle(qc, ws, changesetId),
  });
}

export function useRejectChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req?: ReviewRequest) => api.rejectChangeset(ws, changesetId, req),
    onSuccess: () => invalidateLifecycle(qc, ws, changesetId),
  });
}

export function useMergeChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.mergeChangeset(ws, changesetId),
    onSuccess: () => {
      invalidateLifecycle(qc, ws, changesetId);
      // A merge applies ops to the live graph + voice profiles.
      invalidateConceptAggregates(qc, ws);
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
      void qc.invalidateQueries({ queryKey: ["brand-profiles", ws] });
    },
  });
}

export function useAbandonChangeset(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => api.abandonChangeset(ws, changesetId),
    onSuccess: () => invalidateLifecycle(qc, ws, changesetId),
  });
}

// ── Blast radius + pilots ────────────────────────────────────────────────────

export function useChangesetBlastRadius(changesetId: string, enabled = true) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["changeset-blast-radius", ws, changesetId],
    queryFn: () => api.getChangesetBlastRadius(ws, changesetId),
    enabled: enabled && !!ws && !!changesetId,
    staleTime: 30_000,
    // This is the slowest read in the platform — a walk over every stored block
    // in the workspace. The default retry schedule turns one slow request into
    // four, which is how a panel ends up loading for minutes and reads as never
    // resolving. One retry, then say so.
    retry: 1,
    refetchOnWindowFocus: false,
  });
}

/**
 * The findings diff for one stream. Scoped to a single project and stream, so
 * unlike the blast radius it is a small read — but it is still a walk, so it
 * follows the same one-retry rule rather than turning a slow stream into four
 * slow requests.
 */
export function useTrialFindings(
  changesetId: string,
  projectId: string,
  stream: string,
  enabled = true,
) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["changeset-trial", ws, changesetId, projectId, stream],
    queryFn: () => api.trialFindings(ws, changesetId, projectId, stream),
    enabled: enabled && !!ws && !!changesetId && !!projectId && !!stream,
    staleTime: 30_000,
    retry: 1,
    refetchOnWindowFocus: false,
  });
}

export function useAddPilot(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: StartPilotRequest) => api.addPilot(ws, changesetId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["changeset", ws, changesetId] });
      void qc.invalidateQueries({ queryKey: ["changeset-blast-radius", ws, changesetId] });
    },
  });
}

export function useRemovePilot(changesetId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { projectId: string; stream: string }) =>
      api.removePilot(ws, changesetId, vars.projectId, vars.stream),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["changeset", ws, changesetId] });
      void qc.invalidateQueries({ queryKey: ["changeset-blast-radius", ws, changesetId] });
    },
  });
}
