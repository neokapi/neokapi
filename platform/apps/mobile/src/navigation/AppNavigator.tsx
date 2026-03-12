import React, { useState } from "react";
import { View, Text, TouchableOpacity, ActivityIndicator, StyleSheet } from "react-native";
import { SafeAreaView } from "react-native-safe-area-context";
import { useAuth } from "../auth/AuthContext";
import { LoginScreen } from "../screens/LoginScreen";
import { WorkspaceScreen } from "../screens/WorkspaceScreen";
import { ProjectScreen } from "../screens/ProjectScreen";
import { ReviewQueueScreen } from "../screens/ReviewQueueScreen";
import { NotificationsScreen } from "../screens/NotificationsScreen";
import { SettingsScreen } from "../screens/SettingsScreen";
import type { Workspace, ProjectInfo } from "../api/client";

type Screen =
  | { name: "workspaces" }
  | { name: "projects"; workspace: Workspace }
  | { name: "review"; workspace: Workspace; project: ProjectInfo }
  | { name: "notifications"; workspace: Workspace }
  | { name: "settings" };

export function AppNavigator() {
  const { authenticated, loading } = useAuth();
  const [screen, setScreen] = useState<Screen>({ name: "workspaces" });
  const [lastWorkspace, setLastWorkspace] = useState<Workspace | null>(null);

  if (loading) {
    return (
      <View style={styles.center}>
        <ActivityIndicator size="large" color="#c27930" />
      </View>
    );
  }

  if (!authenticated) {
    return <LoginScreen />;
  }

  const renderScreen = () => {
    switch (screen.name) {
      case "workspaces":
        return (
          <WorkspaceScreen
            onSelect={(workspace) => {
              setLastWorkspace(workspace);
              setScreen({ name: "projects", workspace });
            }}
          />
        );
      case "projects":
        return (
          <ProjectScreen
            workspace={screen.workspace}
            onSelect={(project) =>
              setScreen({ name: "review", workspace: screen.workspace, project })
            }
            onBack={() => setScreen({ name: "workspaces" })}
          />
        );
      case "review":
        return (
          <ReviewQueueScreen
            workspace={screen.workspace}
            project={screen.project}
            onBack={() =>
              setScreen({ name: "projects", workspace: screen.workspace })
            }
          />
        );
      case "notifications":
        return (
          <NotificationsScreen
            workspace={screen.workspace}
            onBack={() => setScreen({ name: "workspaces" })}
          />
        );
      case "settings":
        return (
          <SettingsScreen
            onBack={() => setScreen({ name: "workspaces" })}
          />
        );
    }
  };

  // Show tab bar only at top-level screens (workspaces, notifications, settings).
  const showTabBar = screen.name === "workspaces" || screen.name === "notifications" || screen.name === "settings";

  return (
    <SafeAreaView style={styles.safeArea}>
      <View style={styles.content}>{renderScreen()}</View>
      {showTabBar && (
        <View style={styles.tabBar}>
          <TabButton
            label="Review"
            active={screen.name === "workspaces"}
            onPress={() => setScreen({ name: "workspaces" })}
          />
          <TabButton
            label="Notifications"
            active={screen.name === "notifications"}
            onPress={() => {
              if (lastWorkspace) {
                setScreen({ name: "notifications", workspace: lastWorkspace });
              }
            }}
            disabled={!lastWorkspace}
          />
          <TabButton
            label="Settings"
            active={screen.name === "settings"}
            onPress={() => setScreen({ name: "settings" })}
          />
        </View>
      )}
    </SafeAreaView>
  );
}

function TabButton({ label, active, onPress, disabled }: {
  label: string;
  active: boolean;
  onPress: () => void;
  disabled?: boolean;
}) {
  return (
    <TouchableOpacity
      style={styles.tab}
      onPress={onPress}
      disabled={disabled}
    >
      <Text
        style={[
          styles.tabText,
          active && styles.tabTextActive,
          disabled && styles.tabTextDisabled,
        ]}
      >
        {label}
      </Text>
    </TouchableOpacity>
  );
}

const styles = StyleSheet.create({
  center: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "#0d1117",
  },
  safeArea: {
    flex: 1,
    backgroundColor: "#0d1117",
  },
  content: {
    flex: 1,
  },
  tabBar: {
    flexDirection: "row",
    backgroundColor: "#161b22",
    borderTopWidth: 1,
    borderTopColor: "#30363d",
    paddingVertical: 8,
  },
  tab: {
    flex: 1,
    alignItems: "center",
    paddingVertical: 8,
  },
  tabText: {
    fontSize: 12,
    fontWeight: "600",
    color: "#8b949e",
  },
  tabTextActive: {
    color: "#c27930",
  },
  tabTextDisabled: {
    opacity: 0.4,
  },
});
