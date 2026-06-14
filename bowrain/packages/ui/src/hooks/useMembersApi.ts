// Workspace-member hooks. Authored content across the Brand hub (AD-021) records
// who acted as the OIDC user_id — a UUID — on change-sets (created_by), reviews
// (reviewer), story actors, and comment authors. These hooks resolve those UUIDs
// to the human display names the navigator should show, backed by GET /:ws/members.
import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { Membership } from "../types/api";

/** Maps a user_id to a display name. */
export type NameResolver = (id: string | null | undefined) => string;

/**
 * Build a stable user_id → display-name resolver from a workspace's members. A
 * name falls back to the member's email, then to the raw id, so an unresolved
 * actor (a former member, a system actor) still renders rather than blanking.
 */
export function buildNameResolver(members: Membership[] | undefined): NameResolver {
  const byId = new Map<string, string>();
  for (const member of members ?? []) {
    const name = member.user.name.trim();
    const email = member.user.email.trim();
    byId.set(member.user_id, name || email || member.user_id);
  }
  return (id) => {
    if (!id) return "";
    return byId.get(id) ?? id;
  };
}

/** The workspace's members (GET /:ws/members), cached for name resolution. */
export function useWorkspaceMembers() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";
  return useQuery<Membership[]>({
    queryKey: ["members", ws],
    queryFn: () => api.listMembers(ws),
    enabled: !!ws,
    staleTime: 5 * 60_000,
  });
}

/**
 * Resolve user_id UUIDs to display names. Returns a stable `nameOf(id)`; members
 * load lazily, so until they arrive `nameOf` returns the id, degrading to the raw
 * value rather than blanking.
 */
export function useUserDisplayNames(): { nameOf: NameResolver; isLoading: boolean } {
  const { data, isLoading } = useWorkspaceMembers();
  const nameOf = useMemo(() => buildNameResolver(data), [data]);
  return { nameOf, isLoading };
}
