// The first screen of the Home tab: the next step of the review plan, one tap
// from opening, then the workspaces the person belongs to, each with what
// waits in it. Their avatar sits at the top right.
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
import {
  CountsNotice,
  EmptyState,
  SectionHeader,
  SkeletonCards,
} from "../../../components/home/parts";
import { useAuth } from "../../../lib/auth";
import {
  countsNotice,
  hasAnswered,
  homeStep,
  inboxCounts,
  inboxFailed,
  summarize,
  useReviewHome,
  type HomeStep,
} from "../../../lib/home";
import { usePullRefresh } from "../../../lib/pullRefresh";
import { useHeldLayout } from "../../../lib/heldLayout";
import { useRecentProject } from "../../../lib/recentProject";
import { openContext, openDeck, openSetup, openStep } from "../../../lib/reviewNav";

function openNext(next: HomeStep) {
  void Haptics.selectionAsync().catch(() => undefined);
  const p = next.project;
  switch (next.kind) {
    case "setup":
      openSetup(p, p.plan?.next ?? undefined);
      return;
    case "deck":
      openDeck(next.entry);
      return;
    case "step":
      if (next.step.level === "context") {
        openContext({ workspace: p.workspace, project: p.project, project_name: p.projectName });
      } else {
        openStep(p, next.step.level);
      }
      return;
    case "waiting":
      return;
  }
}

function openWorkspace(w: Workspace) {
  router.push({ pathname: "/[ws]", params: { ws: w.slug } });
}

export default function Workspaces() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const workspaces = useQuery({
    queryKey: ["workspaces"],
    queryFn: ({ signal }) => client.with({ signal }).workspaces(),
  });
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
  const next = useMemo(() => homeStep(inbox.data, recent), [inbox.data, recent]);
  const showWorkspace = (workspaces.data?.length ?? 0) > 1;

  const counts = inboxCounts(inbox);
  const unknown = counts === "unknown";
  const renderItem = useCallback(
    ({ item }: { item: Workspace }) => (
      <WorkspaceCard
        workspace={item}
        waiting={summary.byWorkspace.get(item.slug)}
        unknown={unknown}
        onPress={openWorkspace}
      />
    ),
    [summary, unknown],
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
  } else if (
    inbox.isSuccess &&
    inbox.data &&
    !summary.total.context &&
    !summary.total.translations
  ) {
    hero = <CaughtUpCard />;
  }

  // The next step sits above the workspaces, and its height depends on what
  // the inbox says, as do the counts on every workspace card. The cards wait
  // for the inbox's first answer, so the next step never arrives above them
  // and pushes them down. A failed attempt counts as an answer.
  const settling = workspaces.isPending || !hasAnswered(inbox);
  const hasWorkspaces = !settling && (workspaces.data?.length ?? 0) > 0;

  // When the first paint had no next step (the inbox failed), one that a
  // later attempt brings waits for the next visit or pull rather than
  // arriving above the workspaces.
  const layout = useHeldLayout(!settling, [], hero !== null);
  const pull = usePullRefresh(() =>
    Promise.all([workspaces.refetch(), refetchInbox()]).finally(layout.release),
  );
  const notice = countsNotice(counts, inboxFailed(inbox));

  return (
    <View collapsable={false} className="bg-background flex-1">
      <FlatList
        testID="workspace-list"
        contentInsetAdjustmentBehavior="automatic"
        contentContainerClassName="pb-8"
        data={settling ? [] : (workspaces.data ?? [])}
        keyExtractor={(w) => w.id}
        renderItem={renderItem}
        refreshing={pull.refreshing}
        onRefresh={pull.onRefresh}
        ListHeaderComponent={
          <View className="pt-2">
            {settling || !layout.above ? null : hero}
            {hasWorkspaces ? <SectionHeader title={t("home.workspacesHeader")} /> : null}
          </View>
        }
        ListEmptyComponent={
          settling ? (
            <SkeletonCards testID="workspaces-loading" />
          ) : workspaces.isSuccess ? (
            <EmptyState icon="business-outline" title={t("workspaces.empty")} />
          ) : (
            <QueryState query={workspaces} />
          )
        }
        ListFooterComponent={
          !settling && hasWorkspaces && notice ? (
            <CountsNotice kind={notice} onRetry={() => void refetchInbox()} />
          ) : undefined
        }
      />
    </View>
  );
}
