// Change-set (experiment) hooks for the Brand knowledge graph (AD-021): the
// draft → in_review → approved → merged lifecycle, its ordered ops, reviews,
// pilots, and the blast-radius preview over stored content.
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
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
      void qc.invalidateQueries({ queryKey: ["concepts", ws] });
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
