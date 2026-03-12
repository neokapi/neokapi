import React, { useEffect, useState, useCallback } from "react";
import { View, Text, FlatList, TouchableOpacity, StyleSheet, ActivityIndicator } from "react-native";
import { useAuth } from "../auth/AuthContext";
import type { NotificationInfo } from "../api/client";

interface NotificationsScreenProps {
  workspace: { slug: string };
  onBack: () => void;
}

function timeAgo(dateStr: string): string {
  const now = Date.now();
  const then = new Date(dateStr).getTime();
  const seconds = Math.floor((now - then) / 1000);

  if (seconds < 60) return "just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.floor(hours / 24);
  return `${days}d ago`;
}

export function NotificationsScreen({ workspace, onBack }: NotificationsScreenProps) {
  const { api } = useAuth();
  const [notifications, setNotifications] = useState<NotificationInfo[]>([]);
  const [loading, setLoading] = useState(true);

  const loadNotifications = useCallback(async () => {
    if (!api) return;
    try {
      const result = await api.get<{ notifications: NotificationInfo[] }>(
        `/api/v1/workspaces/${workspace.slug}/notifications`,
      );
      setNotifications(result.notifications ?? []);
    } catch {
      // ignore
    }
    setLoading(false);
  }, [api, workspace.slug]);

  useEffect(() => {
    loadNotifications();
  }, [loadNotifications]);

  const markRead = useCallback(async (id: string) => {
    if (!api) return;
    try {
      await api.put(`/api/v1/workspaces/${workspace.slug}/notifications/${id}/read`);
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, read: true } : n)),
      );
    } catch {
      // ignore
    }
  }, [api, workspace.slug]);

  const markAllRead = useCallback(async () => {
    if (!api) return;
    try {
      await api.put(`/api/v1/workspaces/${workspace.slug}/notifications/read-all`);
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
    } catch {
      // ignore
    }
  }, [api, workspace.slug]);

  const unreadCount = notifications.filter((n) => !n.read).length;

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color="#c27930" />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <View style={styles.header}>
        <TouchableOpacity onPress={onBack}>
          <Text style={styles.back}>← Back</Text>
        </TouchableOpacity>
        {unreadCount > 0 && (
          <TouchableOpacity onPress={markAllRead}>
            <Text style={styles.markAll}>Mark all read</Text>
          </TouchableOpacity>
        )}
      </View>

      <Text style={styles.title}>
        Notifications{unreadCount > 0 ? ` (${unreadCount})` : ""}
      </Text>

      <FlatList
        data={notifications}
        keyExtractor={(n) => n.id}
        renderItem={({ item }) => (
          <TouchableOpacity
            style={[styles.card, !item.read && styles.cardUnread]}
            onPress={() => markRead(item.id)}
          >
            {!item.read && <View style={styles.unreadDot} />}
            <View style={styles.cardContent}>
              <Text style={styles.cardTitle}>{item.title}</Text>
              <Text style={styles.cardBody} numberOfLines={2}>
                {item.body}
              </Text>
              <Text style={styles.cardTime}>{timeAgo(item.created_at)}</Text>
            </View>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <Text style={styles.empty}>No notifications</Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0d1117", padding: 16 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "#0d1117" },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8,
  },
  back: { color: "#c27930", fontSize: 14 },
  markAll: { color: "#58a6ff", fontSize: 13 },
  title: { fontSize: 24, fontWeight: "700", color: "#e6edf3", marginBottom: 16 },
  card: {
    backgroundColor: "#161b22",
    borderRadius: 12,
    padding: 16,
    marginBottom: 8,
    borderWidth: 1,
    borderColor: "#30363d",
    flexDirection: "row",
    alignItems: "flex-start",
  },
  cardUnread: {
    borderColor: "#c27930",
  },
  unreadDot: {
    width: 8,
    height: 8,
    borderRadius: 4,
    backgroundColor: "#c27930",
    marginTop: 6,
    marginRight: 12,
  },
  cardContent: { flex: 1 },
  cardTitle: { fontSize: 15, fontWeight: "600", color: "#e6edf3", marginBottom: 4 },
  cardBody: { fontSize: 13, color: "#8b949e", marginBottom: 6, lineHeight: 18 },
  cardTime: { fontSize: 11, color: "#484f58" },
  empty: { color: "#8b949e", textAlign: "center", marginTop: 32 },
});
