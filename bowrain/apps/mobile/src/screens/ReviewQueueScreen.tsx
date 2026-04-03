import React, { useEffect, useState, useCallback, useRef } from "react";
import { View, Text, StyleSheet, ActivityIndicator } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { useAuth } from "../auth/AuthContext";
import { SwipeCard } from "../components/SwipeCard";
import { EntityReviewCard } from "../components/EntityReviewCard";
import type {
  ReviewItem,
  ReviewDecision,
  ProjectInfo,
  Workspace,
  SyncRequest,
  SyncResponse,
} from "../api/client";

interface ReviewQueueScreenProps {
  workspace: Workspace;
  project: ProjectInfo;
  onBack: () => void;
}

export function ReviewQueueScreen({ workspace, project, onBack }: ReviewQueueScreenProps) {
  const { api } = useAuth();
  const [items, setItems] = useState<ReviewItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [currentIndex, setCurrentIndex] = useState(0);
  const pendingDecisions = useRef<ReviewDecision[]>([]);

  const loadItems = useCallback(async () => {
    if (!api) return;
    try {
      const result = await api.get<{ items: ReviewItem[] }>(
        `/api/v1/workspaces/${workspace.slug}/projects/${project.id}/review-queue?status=pending&limit=50`,
      );
      setItems(result.items ?? []);
      setCurrentIndex(0);
    } catch {
      // ignore
    }
    setLoading(false);
  }, [api, workspace.slug, project.id]);

  useEffect(() => {
    loadItems();
  }, [loadItems]);

  // Sync pending decisions periodically and on unmount.
  const syncDecisions = useCallback(async () => {
    if (!api || pendingDecisions.current.length === 0) return;
    const decisions = [...pendingDecisions.current];
    pendingDecisions.current = [];

    try {
      await api.post<SyncResponse>(
        `/api/v1/workspaces/${workspace.slug}/projects/${project.id}/review-queue/sync`,
        { decisions } as SyncRequest,
      );
    } catch {
      // Re-queue failed decisions.
      pendingDecisions.current = [...decisions, ...pendingDecisions.current];
    }
  }, [api, workspace.slug, project.id]);

  useEffect(() => {
    const timer = setInterval(syncDecisions, 10000); // Sync every 10s.
    return () => {
      clearInterval(timer);
      syncDecisions(); // Flush on unmount.
    };
  }, [syncDecisions]);

  const handleDecision = useCallback(
    (status: "approved" | "rejected" | "skipped") => {
      const item = items[currentIndex];
      if (!item) return;

      pendingDecisions.current.push({ item_id: item.id, status });
      setCurrentIndex((i) => i + 1);

      // Auto-sync after every 5 decisions.
      if (pendingDecisions.current.length >= 5) {
        syncDecisions();
      }
    },
    [items, currentIndex, syncDecisions],
  );

  const remaining = items.length - currentIndex;
  const currentItem = items[currentIndex];

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color="#c27930" />
      </View>
    );
  }

  return (
    <GestureHandlerRootView style={styles.container}>
      {/* Header */}
      <View style={styles.header}>
        <Text style={styles.back} onPress={onBack}>
          ← {project.name}
        </Text>
        <View style={styles.badge}>
          <Text style={styles.badgeText}>{remaining}</Text>
        </View>
      </View>

      <Text style={styles.title}>Review Queue</Text>
      <Text style={styles.subtitle}>Swipe right to approve, left to reject</Text>

      {/* Card stack */}
      <View style={styles.cardStack}>
        {currentItem ? (
          currentItem.type === "entity_review" ? (
            <EntityReviewCard
              key={currentItem.id}
              item={currentItem}
              onApprove={() => handleDecision("approved")}
              onReject={() => handleDecision("rejected")}
              onSkip={() => handleDecision("skipped")}
            />
          ) : (
            <SwipeCard
              key={currentItem.id}
              item={currentItem}
              onApprove={() => handleDecision("approved")}
              onReject={() => handleDecision("rejected")}
              onSkip={() => handleDecision("skipped")}
            />
          )
        ) : (
          <View style={styles.emptyState}>
            <Text style={styles.emptyEmoji}>🎉</Text>
            <Text style={styles.emptyTitle}>All caught up!</Text>
            <Text style={styles.emptyText}>No items left to review</Text>
          </View>
        )}
      </View>
    </GestureHandlerRootView>
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
  badge: {
    backgroundColor: "#c27930",
    borderRadius: 12,
    paddingHorizontal: 10,
    paddingVertical: 4,
    minWidth: 28,
    alignItems: "center",
  },
  badgeText: { color: "#fff", fontWeight: "700", fontSize: 13 },
  title: { fontSize: 24, fontWeight: "700", color: "#e6edf3", marginBottom: 4 },
  subtitle: { fontSize: 14, color: "#8b949e", marginBottom: 24 },
  cardStack: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
  },
  emptyState: { alignItems: "center" },
  emptyEmoji: { fontSize: 48, marginBottom: 16 },
  emptyTitle: { fontSize: 20, fontWeight: "700", color: "#e6edf3", marginBottom: 4 },
  emptyText: { fontSize: 14, color: "#8b949e" },
});
