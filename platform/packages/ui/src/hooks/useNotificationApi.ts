import { useCallback, useMemo } from "react";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";
import type { NotificationInfo } from "../types/api";

export function useNotificationApi() {
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const listNotifications = useCallback(
    async (
      limit?: number,
      unreadOnly?: boolean,
    ): Promise<{ notifications: NotificationInfo[]; unread_count: number }> =>
      api.listNotifications(ws, limit, unreadOnly),
    [api, ws],
  );

  const markRead = useCallback(
    async (id: string): Promise<void> => api.markNotificationRead(ws, id),
    [api, ws],
  );

  const markAllRead = useCallback(
    async (): Promise<void> => api.markAllNotificationsRead(ws),
    [api, ws],
  );

  const deleteNotification = useCallback(
    async (id: string): Promise<void> => api.deleteNotification(ws, id),
    [api, ws],
  );

  return useMemo(
    () => ({
      listNotifications,
      markRead,
      markAllRead,
      deleteNotification,
    }),
    [listNotifications, markRead, markAllRead, deleteNotification],
  );
}
