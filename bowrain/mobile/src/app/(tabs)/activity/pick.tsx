// The sheet that picks the Activity tab's project: every project of every
// workspace the person belongs to, under a header per workspace.
import type { Project, Workspace } from "@neokapi/bowrain-client";
import { useQueries, useQuery } from "@tanstack/react-query";
import { router } from "expo-router";
import { ActivityIndicator, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { Cell, Section } from "../../../components/Grouped";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { setRecentProject, useRecentProject } from "../../../lib/recentProject";

function WorkspaceProjects({
  workspace,
  projects,
  pending,
}: {
  workspace: Workspace;
  projects: Project[] | undefined;
  pending: boolean;
}) {
  const recent = useRecentProject();
  if (pending) {
    return (
      <View className="items-center p-4">
        <ActivityIndicator />
      </View>
    );
  }
  if (!projects?.length) return null;
  return (
    <Section header={workspace.name}>
      {projects.map((p) => (
        <Cell
          key={p.id}
          testID={`pick-${p.id}`}
          title={p.name}
          selected={recent?.ws === workspace.slug && recent.project === p.id}
          onPress={() => {
            setRecentProject({ ws: workspace.slug, project: p.id, name: p.name });
            router.back();
          }}
        />
      ))}
    </Section>
  );
}

export default function PickProject() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => client.workspaces() });
  const projects = useQueries({
    queries: (workspaces.data ?? []).map((w) => ({
      queryKey: ["projects", w.slug],
      queryFn: () => client.projects(w.slug),
    })),
  });

  return (
    <ScrollView className="bg-background flex-1" contentContainerClassName="gap-6 pt-6 pb-10">
      <Text accessibilityRole="header" className="text-foreground px-8 text-xl font-semibold">
        {t("activity.pickTitle")}
      </Text>
      {workspaces.data ? (
        workspaces.data.map((w, i) => (
          <WorkspaceProjects
            key={w.id}
            workspace={w}
            projects={projects[i]?.data}
            pending={projects[i]?.isPending ?? true}
          />
        ))
      ) : (
        <QueryState query={workspaces} />
      )}
    </ScrollView>
  );
}
