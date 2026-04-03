import { useCallback, useMemo } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { ActivityInfo } from "../types/api";

export function useActivities() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const listActivities = useCallback(
    async (query?: {
      project_id?: string;
      stream?: string;
      actor_id?: string;
      type?: string;
      cursor?: string;
      limit?: number;
    }): Promise<{ activities: ActivityInfo[]; next_cursor: string }> =>
      api.listActivities(ws, query),
    [api, ws],
  );

  return useMemo(() => ({ listActivities }), [listActivities]);
}
