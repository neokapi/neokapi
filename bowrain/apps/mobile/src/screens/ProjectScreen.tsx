import React, { useEffect, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  StyleSheet,
  ActivityIndicator,
} from "react-native";
import { useAuth } from "../auth/AuthContext";
import type { ProjectInfo, Workspace } from "../api/client";

interface ProjectScreenProps {
  workspace: Workspace;
  onSelect: (project: ProjectInfo) => void;
  onBack: () => void;
}

export function ProjectScreen({ workspace, onSelect, onBack }: ProjectScreenProps) {
  const { api } = useAuth();
  const [projects, setProjects] = useState<ProjectInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!api) return;
    api
      .get<ProjectInfo[]>(`/api/v1/workspaces/${workspace.slug}/editor/projects`)
      .then(setProjects)
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [api, workspace.slug]);

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color="#c27930" />
      </View>
    );
  }

  return (
    <View style={styles.container}>
      <TouchableOpacity onPress={onBack}>
        <Text style={styles.back}>← {workspace.name}</Text>
      </TouchableOpacity>
      <Text style={styles.title}>Projects</Text>
      <FlatList
        data={projects}
        keyExtractor={(p) => p.id}
        renderItem={({ item }) => (
          <TouchableOpacity style={styles.card} onPress={() => onSelect(item)}>
            <Text style={styles.cardTitle}>{item.name}</Text>
            <Text style={styles.cardSub}>
              {item.source_locale} → {item.target_locales.join(", ")}
            </Text>
          </TouchableOpacity>
        )}
        ListEmptyComponent={<Text style={styles.empty}>No projects in this workspace</Text>}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1, backgroundColor: "#0d1117", padding: 16 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "#0d1117" },
  back: { color: "#c27930", fontSize: 14, marginBottom: 8 },
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
