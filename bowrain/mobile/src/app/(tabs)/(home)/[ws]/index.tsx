// The projects in a workspace, under the workspace's name as a large title.
// Each card shows the project's languages as badges, lit where translations
// wait for the person, and its context waiting for review. The projects with
// work waiting come first, in the server's order otherwise.
import type { Project, Workspace } from "@neokapi/bowrain-client";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { router, Stack, useFocusEffect, useLocalSearchParams } from "expo-router";
import { useCallback, useMemo } from "react";
import { FlatList, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../../components/List";
import { ProjectCard } from "../../../../components/home/HomeCards";
import { EmptyState, SkeletonCards } from "../../../../components/home/parts";
import { useAuth } from "../../../../lib/auth";
import { projectKey, summarize, useReviewHome } from "../../../../lib/home";

export default function Projects() {
  const { t } = useTranslation();
  const { ws } = useLocalSearchParams<{ ws: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const projects = useQuery({ queryKey: ["projects", ws], queryFn: () => client.projects(ws) });
  const inbox = useReviewHome();
  const { refetch: refetchInbox } = inbox;
  const wsName = qc.getQueryData<Workspace[]>(["workspaces"])?.find((w) => w.slug === ws)?.name;

  useFocusEffect(
    useCallback(() => {
      void refetchInbox();
    }, [refetchInbox]),
  );

  const summary = useMemo(() => summarize(inbox.data), [inbox.data]);
  const sorted = useMemo(() => {
    const list = projects.data ?? [];
    const waits = (p: Project) => {
      const w = summary.byProject.get(projectKey(ws, p.id));
      return (w?.translations ?? 0) + (w?.context ?? 0) > 0 ? 1 : 0;
    };
    // Array.prototype.sort is stable, so the server's order holds within each part.
    return [...list].sort((a, b) => waits(b) - waits(a));
  }, [projects.data, summary, ws]);

  const open = useCallback(
    (p: Project) => router.push({ pathname: "/[ws]/[project]", params: { ws, project: p.id } }),
    [ws],
  );
  const renderItem = useCallback(
    ({ item }: { item: Project }) => (
      <ProjectCard
        project={item}
        waiting={summary.byProject.get(projectKey(ws, item.id))}
        onPress={open}
      />
    ),
    [summary, ws, open],
  );

  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: wsName ?? t("projects.title"), headerLargeTitle: true }} />
      <FlatList
        testID="project-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pt-2 pb-8"
        data={sorted}
        keyExtractor={(p) => p.id}
        renderItem={renderItem}
        refreshing={projects.isRefetching || inbox.isRefetching}
        onRefresh={() => {
          void projects.refetch();
          void refetchInbox();
        }}
        ListEmptyComponent={
          projects.isPending ? (
            <SkeletonCards testID="projects-loading" />
          ) : projects.isSuccess ? (
            <EmptyState icon="folder-open-outline" title={t("projects.empty")} />
          ) : (
            <QueryState query={projects} />
          )
        }
      />
    </View>
  );
}
