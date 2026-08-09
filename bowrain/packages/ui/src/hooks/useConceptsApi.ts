// Concept hooks for the Brand knowledge graph (AD-021): the concept list,
// a single concept, its story, relations, where-used (blast radius),
// observations, and comments — with mutations that invalidate the right keys.
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { AddConceptRequest, UpdateConceptRequest } from "../types/api";
import type {
  ListConceptsParams,
  RelationScope,
  AddConceptRelationRequest,
  AddObservationRequest,
  AddCommentRequest,
} from "../types/brand-graph";

function useWs() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return { api, ws };
}

/**
 * Every read whose answer a concept write can change: the lists, and the
 * server-side status counts and locale coverage the dashboard reads instead of
 * folding a page of concepts itself.
 */
export function invalidateConceptAggregates(
  qc: ReturnType<typeof useQueryClient>,
  ws: string,
): void {
  void qc.invalidateQueries({ queryKey: ["concepts", ws] });
  void qc.invalidateQueries({ queryKey: ["concept-status-counts", ws] });
  void qc.invalidateQueries({ queryKey: ["concept-locale-coverage", ws] });
}

// ── Concept list + single concept ──────────────────────────────────────────

export function useConcepts(params?: ListConceptsParams) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concepts", ws, params ?? {}],
    queryFn: () => api.listConcepts(ws, params),
    enabled: !!ws,
    staleTime: 15_000,
  });
}

/**
 * The workspace vocabulary counted by term lifecycle status, over every concept
 * rather than a page of them. The buckets overlap — a concept counts under each
 * status one of its terms carries — so they do not sum to `total`.
 */
export function useConceptStatusCounts() {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-status-counts", ws],
    queryFn: () => api.getConceptStatusCounts(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

/** Per-locale concept coverage over the whole workspace, most complete first. */
export function useConceptLocaleCoverage() {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-locale-coverage", ws],
    queryFn: () => api.getConceptLocaleCoverage(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

export function useConcept(conceptId: string) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept", ws, conceptId],
    queryFn: () => api.getConcept(ws, conceptId),
    enabled: !!ws && !!conceptId,
    staleTime: 15_000,
  });
}

export function useCreateConcept() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddConceptRequest) => api.createConcept(ws, req),
    onSuccess: () => {
      invalidateConceptAggregates(qc, ws);
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
    },
  });
}

export function useUpdateConcept() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: UpdateConceptRequest) => api.updateConcept(ws, req),
    onSuccess: (_r, req) => {
      invalidateConceptAggregates(qc, ws);
      void qc.invalidateQueries({ queryKey: ["concept", ws, req.concept_id] });
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
    },
  });
}

export function useDeleteConcept() {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (conceptId: string) => api.deleteConcept(ws, conceptId),
    onSuccess: () => {
      invalidateConceptAggregates(qc, ws);
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
    },
  });
}

// ── Story ───────────────────────────────────────────────────────────────────

export function useConceptStory(conceptId: string) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-story", ws, conceptId],
    queryFn: () => api.getConceptStory(ws, conceptId),
    enabled: !!ws && !!conceptId,
    staleTime: 15_000,
  });
}

// ── Relations ─────────────────────────────────────────────────────────────

export function useConceptRelations(conceptId: string, scope?: RelationScope) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-relations", ws, conceptId, scope ?? {}],
    queryFn: () => api.listConceptRelations(ws, conceptId, scope),
    enabled: !!ws && !!conceptId,
    staleTime: 15_000,
  });
}

export function useAddConceptRelation(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddConceptRelationRequest) => api.addConceptRelation(ws, conceptId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-relations", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
    },
  });
}

export function useDeleteConceptRelation(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (relationId: string) => api.deleteConceptRelation(ws, conceptId, relationId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-relations", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["graph", ws] });
    },
  });
}

// ── Where-used (concept blast radius) ───────────────────────────────────────

export function useConceptBlastRadius(conceptId: string, enabled = true) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-blast-radius", ws, conceptId],
    queryFn: () => api.getConceptBlastRadius(ws, conceptId),
    enabled: enabled && !!ws && !!conceptId,
    staleTime: 30_000,
  });
}

// ── Observations ────────────────────────────────────────────────────────────

export function useObservations(conceptId: string) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-observations", ws, conceptId],
    queryFn: () => api.listObservations(ws, conceptId),
    enabled: !!ws && !!conceptId,
    staleTime: 15_000,
  });
}

export function useAddObservation(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddObservationRequest) => api.addObservation(ws, conceptId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-observations", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
    },
  });
}

export function useDeleteObservation(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (observationId: string) => api.deleteObservation(ws, conceptId, observationId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-observations", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
    },
  });
}

// ── Comments ─────────────────────────────────────────────────────────────────

export function useConceptComments(conceptId: string) {
  const { api, ws } = useWs();
  return useQuery({
    queryKey: ["concept-comments", ws, conceptId],
    queryFn: () => api.listConceptComments(ws, conceptId),
    enabled: !!ws && !!conceptId,
    staleTime: 10_000,
  });
}

export function useAddConceptComment(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: AddCommentRequest) => api.addConceptComment(ws, conceptId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-comments", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
    },
  });
}

export function useResolveConceptComment(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (vars: { commentId: string; resolved?: boolean }) =>
      api.resolveConceptComment(ws, conceptId, vars.commentId, vars.resolved),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-comments", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
    },
  });
}

export function useDeleteConceptComment(conceptId: string) {
  const { api, ws } = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (commentId: string) => api.deleteConceptComment(ws, conceptId, commentId),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["concept-comments", ws, conceptId] });
      void qc.invalidateQueries({ queryKey: ["concept-story", ws, conceptId] });
    },
  });
}
