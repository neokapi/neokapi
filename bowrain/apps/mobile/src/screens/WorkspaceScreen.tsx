import React, { useEffect, useState } from "react";
import { View, Text, FlatList, TouchableOpacity, StyleSheet, ActivityIndicator } from "react-native";
import { useAuth } from "../auth/AuthContext";
import type { Workspace } from "../api/client";

interface WorkspaceScreenProps {
  onSelect: (workspace: Workspace) => void;
}

export function WorkspaceScreen({ onSelect }: WorkspaceScreenProps) {
  const { api } = useAuth();
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!api) return;
    api.get<Workspace[]>("/api/v1/workspaces")
      .then(setWorkspaces)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [api]);

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color="#c27930" />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <Text style={styles.title}>Workspaces</Text>
      <FlatList
        data={workspaces}
        keyExtractor={(w) => w.id}
        renderItem={({ item }) => (
          <TouchableOpacity style={styles.card} onPress={() => onSelect(item)}>
            <Text style={styles.cardTitle}>{item.name}</Text>
            <Text style={styles.cardSub}>{item.slug} · {item.role}</Text>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <Text style={styles.empty}>No workspaces found</Text>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0d1117", padding: 16 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "#0d1117" },
  title: { fontSize: 24, fontWeight: "700", color: "#e6edf3", marginBottom: 16 },
  card: {
    backgroundColor: "#161b22",
    borderRadius: 12,
    padding: 16,
    marginBottom: 8,
    borderWidth: 1,
    borderColor: "#30363d",
  },
  cardTitle: { fontSize: 16, fontWeight: "600", color: "#e6edf3" },
  cardSub: { fontSize: 13, color: "#8b949e", marginTop: 4 },
  empty: { color: "#8b949e", textAlign: "center", marginTop: 32 },
});
