import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { PermissionName } from "../types/api";

/**
 * useCallerPermissions reports what the signed-in user may do on one project,
 * as the server resolved it. A surface asks so it can offer only the actions
 * the server will accept: a translator sees no Approve button rather than a
 * 403 after clicking it.
 *
 * `can` is optimistic while the answer is in flight, so a decision bar does not
 * flicker from disabled to enabled on every mount. The server gates every call
 * regardless; this only decides what to draw.
 */
export function useCallerPermissions(projectId: string | undefined) {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const query = useQuery({
    queryKey: ["caller-permissions", ws, projectId],
    queryFn: () => api.getCallerPermissions(ws, projectId!),
    enabled: !!ws && !!projectId,
    staleTime: 60_000,
  });

  const data = query.data;

  /** Whether the caller holds a permission, and (for a language-scoped one) holds it for that language. */
  const can = (permission: PermissionName, locale?: string): boolean => {
    if (!data) return true;
    if (!data.permissions.includes(permission)) return false;
    if (!locale || data.languages.length === 0) return true;
    return data.languages.includes(locale);
  };

  return { ...query, can };
}
