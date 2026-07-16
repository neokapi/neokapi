import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { usePlatform } from "../platform";
import { invalidateForEvent } from "./useWorkspaceEvents";

/**
 * Desktop content-freshness. The web shell keeps views fresh with a same-origin
 * SSE EventSource (`useWorkspaceEvents`); the desktop webview can't authenticate
 * that stream (keychain-Bearer over Wails), so the Go backend runs the watcher
 * and re-emits changes as Wails events. This subscribes through the platform
 * seam (`watchProject`) and invalidates the exact same React Query caches via
 * the shared `invalidateForEvent`, so external changes (another user, a kapi
 * push, a connector sync, a flow/automation completion) refresh open views.
 *
 * No-op on web (the seam member is absent) and when there is no workspace. Gated
 * to `kind === "desktop"` so the SSE path stays the only web mechanism.
 */
export function useDesktopFreshness(workspaceSlug: string | undefined, projectId?: string): void {
  const platform = usePlatform();
  const queryClient = useQueryClient();

  useEffect(() => {
    if (platform.kind !== "desktop" || !platform.watchProject || !workspaceSlug) return;
    return platform.watchProject(projectId, (event) => {
      // A reconnect after an offline gap may have missed any number of external
      // changes; refetch everything rather than guess what went stale.
      if (event.type === "reconnected") {
        void queryClient.invalidateQueries();
        return;
      }
      invalidateForEvent(queryClient, workspaceSlug, { type: event.type });
    });
  }, [platform, workspaceSlug, projectId, queryClient]);
}
