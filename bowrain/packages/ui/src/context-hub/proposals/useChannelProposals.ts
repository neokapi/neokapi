import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import type {
  ChannelAliasJudgement,
  ChannelAliasProposalsResponse,
} from "../../types/channel-proposals";

/** The workspace's channel-slug equivalence proposals, judged and unjudged. */
export function useChannelProposals() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery<ChannelAliasProposalsResponse>({
    queryKey: ["channel-proposals", ws],
    queryFn: () => api.listChannelProposals(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

/**
 * Settles one proposal. The listing is refetched rather than patched in place:
 * a judgement is the server's to record, and reading it back is what proves the
 * next re-sighting will not reopen it.
 */
export function useJudgeChannelProposal() {
  const api = useApi();
  const queryClient = useQueryClient();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useMutation({
    mutationFn: (judgement: ChannelAliasJudgement) => api.judgeChannelProposal(ws, judgement),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["channel-proposals", ws] });
    },
  });
}

/** The key that identifies one proposal, for list keys and pending state. */
export function channelProposalKey(p: {
  profile?: string;
  proposed_channel: string;
  existing_channel: string;
}): string {
  return `${p.profile ?? ""}|${p.proposed_channel}|${p.existing_channel}`;
}
