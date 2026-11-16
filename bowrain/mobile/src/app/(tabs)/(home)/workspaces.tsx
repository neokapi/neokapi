// The first screen of the Home tab: the deck that waits for the person, one
// tap from opening, then the workspaces they belong to, each with what waits
// in it. The person's avatar sits at the top right.
import type { Workspace } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { router, useFocusEffect } from "expo-router";
import { useCallback, useMemo } from "react";
import { FlatList, View } from "react-native";
import { useTranslation } from "react-i18next";

import { QueryState } from "../../../components/List";
import { WorkspaceCard } from "../../../components/home/HomeCards";
import { CaughtUpCard, NextUpCard } from "../../../components/home/NextUpCard";
import { EmptyState, SectionHeader, SkeletonCards } from "../../../components/home/parts";
import { useAuth } from "../../../lib/auth";
import { nextUp, summarize, useReviewHome, type NextUp } from "../../../lib/home";
import { setRecentProject, useRecentProject } from "../../../lib/recentProject";

function openNext(next: NextUp) {
  void Haptics.selectionAsync().catch(() => undefined);
  const { workspace: ws, project, project_name: name } = next.entry;
  setRecentProject({ ws, project, name });
  if (next.kind === "language") {
    router.push({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws, project, locale: next.entry.locale },
    });
  } else {
    router.push({ pathname: "/[ws]/[project]/context", params: { ws, project } });
  }
}

function openWorkspace(w: Workspace) {
  router.push({ pathname: "/[ws]", params: { ws: w.slug } });
}

export default function Workspaces() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const workspaces = useQuery({ queryKey: ["workspaces"], queryFn: () => client.workspaces() });
  const inbox = useReviewHome();
  const recent = useRecentProject();
  const { refetch: refetchInbox } = inbox;

  // Back from a deck, the counts have moved.
  useFocusEffect(
    useCallback(() => {
      void refetchInbox();
    }, [refetchInbox]),
  );

  const summary = useMemo(() => summarize(inbox.data), [inbox.data]);
  const next = useMemo(() => nextUp(inbox.data, recent), [inbox.data, recent]);
  const showWorkspace = (workspaces.data?.length ?? 0) > 1;

  const renderItem = useCallback(
    ({ item }: { item: Workspace }) => (
      <WorkspaceCard
        workspace={item}
        waiting={summary.byWorkspace.get(item.slug)}
        onPress={openWorkspace}
      />
    ),
    [summary],
  );

  // A server without the inbox route (data null) shows the workspaces alone.
  let hero = null;
  if (next) {
    hero = (
      <NextUpCard
        next={next}
        showWorkspace={showWorkspace}
        onOpen={() => openNext(next)}
        onMore={() => router.navigate("/review")}
      />
    );
  } else if (inbox.isSuccess && inbox.data) {
    hero = <CaughtUpCard />;
  }

  const hasWorkspaces = (workspaces.data?.length ?? 0) > 0;

  return (
    <View collapsable={false} className="bg-background flex-1">
      <FlatList
        testID="workspace-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pb-8"
        data={workspaces.data ?? []}
        keyExtractor={(w) => w.id}
        renderItem={renderItem}
        refreshing={workspaces.isRefetching || inbox.isRefetching}
        onRefresh={() => {
          void workspaces.refetch();
          void refetchInbox();
        }}
        ListHeaderComponent={
          <View className="pt-2">
            {inbox.isPending && workspaces.isPending ? null : hero}
            {hasWorkspaces ? <SectionHeader title={t("home.workspacesHeader")} /> : null}
          </View>
        }
        ListEmptyComponent={
          workspaces.isPending ? (
            <SkeletonCards testID="workspaces-loading" />
          ) : workspaces.isSuccess ? (
            <EmptyState icon="business-outline" title={t("workspaces.empty")} />
          ) : (
            <QueryState query={workspaces} />
          )
        }
      />
    </View>
  );
}
