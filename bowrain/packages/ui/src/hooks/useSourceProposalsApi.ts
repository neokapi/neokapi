import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEditorApi } from "./useEditorApi";
import { useWorkspace } from "../context/WorkspaceContext";
import type { CreateSourceProposalRequest } from "../types/api";

// react-query hooks for back-to-source review (RV-F). The list drives the source
// owner's review surface; the mutations create and decide proposals, invalidating
// the surface (and the review session, whose blocks change when a source is
// approved and every locale re-drafts).

const proposalsKey = (ws: string, projectId: string) => ["source-proposals", ws, projectId];

/** Open source-change proposals for a project. */
export function useSourceProposals(projectId: string, enabled = true) {
  const api = useEditorApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return useQuery({
    queryKey: proposalsKey(ws, projectId),
    queryFn: () => api.listSourceProposals(projectId),
    enabled: enabled && !!ws && !!projectId,
    staleTime: 5_000,
  });
}

/** Create a source-change proposal from the review session. */
export function useCreateSourceProposal(projectId: string) {
  const api = useEditorApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (req: Omit<CreateSourceProposalRequest, "stream">) =>
      api.createSourceProposal(projectId, req),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: proposalsKey(ws, projectId) });
    },
  });
}

/** Approve or reject an open source proposal (approval re-drafts every locale). */
export function useDecideSourceProposal(projectId: string) {
  const api = useEditorApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { proposalId: string; decision: "approve" | "reject"; reason?: string }) =>
      api.decideSourceProposal(projectId, v.proposalId, v.decision, v.reason),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: proposalsKey(ws, projectId) });
      // Approving a source change rewrites the source and demotes every
      // locale's translation, so the review session's blocks change. Refresh it.
      void qc.invalidateQueries({ queryKey: ["review-session"] });
    },
  });
}
