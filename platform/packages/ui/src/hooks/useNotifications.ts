import { useState, useEffect, useCallback, useRef } from "react";
import type { NotificationInfo } from "../types/api";
import { useApi } from "../context/ApiContext";
import { useWorkspace } from "../context/WorkspaceContext";

interface UseNotificationsOptions {
  /** Poll interval in ms (default: 60000 = 1 minute). Set 0 to disable polling. */
  pollInterval?: number;
  /** Enable WebSocket for real-time updates (default: true). */
  enableWebSocket?: boolean;
}

interface UseNotificationsResult {
  notifications: NotificationInfo[];
  unreadCount: number;
  loading: boolean;
  markRead: (id: string) => Promise<void>;
  markAllRead: () => Promise<void>;
  deleteNotification: (id: string) => Promise<void>;
  refresh: () => Promise<void>;
}

/**
 * High-level notification hook that combines REST API with WebSocket updates.
 * Automatically polls and listens for real-time notifications.
 */
export function useNotifications(options: UseNotificationsOptions = {}): UseNotificationsResult {
  const { pollInterval = 60000, enableWebSocket = true } = options;
  const api = useApi();
  const { activeWorkspace } = useWorkspace();
  const ws = activeWorkspace?.slug ?? "";

  const [notifications, setNotifications] = useState<NotificationInfo[]>([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const wsRef = useRef<WebSocket | null>(null);

  const refresh = useCallback(async () => {
    if (!ws) return;
    try {
      const result = await api.listNotifications(ws, 50);
      setNotifications(result.notifications);
      setUnreadCount(result.unread_count);
    } catch {
      // Silently ignore — notifications are non-critical.
    } finally {
      setLoading(false);
    }
  }, [api, ws]);

  // Initial load + polling.
  useEffect(() => {
    if (!ws) return;
    void refresh();
    if (pollInterval > 0) {
      const timer = setInterval(refresh, pollInterval);
      return () => clearInterval(timer);
    }
  }, [ws, refresh, pollInterval]);

  // WebSocket connection for real-time updates.
  useEffect(() => {
    if (!enableWebSocket || !ws) return;

    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${protocol}//${window.location.host}/api/v1/workspaces/${ws}/notifications/ws`;

    let socket: WebSocket;
    try {
      socket = new WebSocket(url);
    } catch {
      return;
    }
    wsRef.current = socket;

    socket.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === "notification" && data.notification) {
          const n = data.notification as NotificationInfo;
          setNotifications((prev) => [n, ...prev]);
          setUnreadCount((prev) => prev + 1);
        }
      } catch {
        // Ignore malformed messages.
      }
    };

    socket.onclose = () => {
      wsRef.current = null;
    };

    return () => {
      socket.close();
      wsRef.current = null;
    };
  }, [enableWebSocket, ws]);

  const markRead = useCallback(
    async (id: string) => {
      if (!ws) return;
      await api.markNotificationRead(ws, id);
      setNotifications((prev) => prev.map((n) => (n.id === id ? { ...n, read: true } : n)));
      setUnreadCount((prev) => Math.max(0, prev - 1));
    },
    [api, ws],
  );

  const markAllRead = useCallback(async () => {
    if (!ws) return;
    await api.markAllNotificationsRead(ws);
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
    setUnreadCount(0);
  }, [api, ws]);

  const deleteNotification = useCallback(
    async (id: string) => {
      if (!ws) return;
      const n = notifications.find((x) => x.id === id);
      await api.deleteNotification(ws, id);
      setNotifications((prev) => prev.filter((x) => x.id !== id));
      if (n && !n.read) {
        setUnreadCount((prev) => Math.max(0, prev - 1));
      }
    },
    [api, ws, notifications],
  );

  return {
    notifications,
    unreadCount,
    loading,
    markRead,
    markAllRead,
    deleteNotification,
    refresh,
  };
}
