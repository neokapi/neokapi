import { useQuery } from "@tanstack/react-query";
import { useApi } from "../../context/ApiContext";
import { useWorkspace } from "../../context/WorkspaceContext";
import type { ContextProfile, ContextProfilesResponse } from "../../types/context-profiles";

/** The workspace's governance profiles, as the server aggregates them. */
export function useContextProfiles() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  return useQuery<ContextProfilesResponse>({
    queryKey: ["context-profiles", ws],
    queryFn: () => api.listContextProfiles(ws),
    enabled: !!ws,
    staleTime: 30_000,
  });
}

/** One profile out of the same aggregation, so the list and the detail agree. */
export function useContextProfile(slug: string) {
  const query = useContextProfiles();
  const profile: ContextProfile | undefined = query.data?.profiles.find((p) => p.slug === slug);
  return { ...query, profile };
}
