// The review inbox: the projects whose context waits for the person's review,
// then every language they may review, across projects and workspaces, where
// translations are waiting, most waiting first. The server lists both, counts
// them as each deck counts them and orders them (GET /auth/me/review); the app
// shows the lists as they come. A language opens its collections, to take the
// deck one collection at a time.
import type { ContextInboxEntry, ReviewInboxLanguage } from "@neokapi/bowrain-client";
import { useQuery } from "@tanstack/react-query";
import { router, useFocusEffect } from "expo-router";
import { useCallback } from "react";
import { FlatList, View } from "react-native";
import { useTranslation } from "react-i18next";

import { ContextEntryCard, LanguageCard } from "../../../components/home/HomeCards";
import { EmptyState, SkeletonCards } from "../../../components/home/parts";
import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { setRecentProject } from "../../../lib/recentProject";

const inboxKey = ["review-inbox"] as const;

/** Where an inbox entry sits: its project, and its workspace when the list spans several. */
function whereOf(
  e: { project_name: string; workspace_name: string },
  showWorkspace: boolean,
): string {
  return showWorkspace ? `${e.project_name} · ${e.workspace_name}` : e.project_name;
}

export default function ReviewInbox() {
  const { t } = useTranslation();
  const { client } = useAuth();
  const inbox = useQuery({ queryKey: inboxKey, queryFn: () => client.reviewHome() });
  const { refetch } = inbox;

  // Back from a deck, the counts have moved.
  useFocusEffect(
    useCallback(() => {
      void refetch();
    }, [refetch]),
  );

  const languages = inbox.data?.languages ?? [];
  const context = inbox.data?.context ?? [];
  const showWorkspace =
    new Set([...languages.map((l) => l.workspace), ...context.map((c) => c.workspace)]).size > 1;
  const openLanguage = useCallback((item: ReviewInboxLanguage) => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/review/[ws]/[project]/[locale]",
      params: { ws: item.workspace, project: item.project, locale: item.locale },
    });
  }, []);
  const openContext = (item: ContextInboxEntry) => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/[ws]/[project]/context",
      params: { ws: item.workspace, project: item.project },
    });
  };
  const renderItem = useCallback(
    ({ item }: { item: ReviewInboxLanguage }) => (
      <LanguageCard
        testID={`inbox-${item.project}-${item.locale}`}
        locale={item.locale}
        waiting={item}
        where={whereOf(item, showWorkspace)}
        onPress={() => openLanguage(item)}
      />
    ),
    [showWorkspace, openLanguage],
  );

  let empty = null;
  if (inbox.isPending) {
    empty = <SkeletonCards />;
  } else if (inbox.isSuccess && inbox.data === null) {
    empty = (
      <EmptyState icon="server" title={t("inbox.unsupported")} body={t("inbox.unsupportedBody")} />
    );
  } else if (inbox.isSuccess && context.length === 0) {
    empty = (
      <EmptyState icon="checkmark-done" title={t("inbox.emptyTitle")} body={t("inbox.emptyBody")} />
    );
  }

  return (
    <View collapsable={false} className="bg-background flex-1">
      <FlatList
        testID="inbox-list"
        contentInsetAdjustmentBehavior="automatic"
        data={languages}
        keyExtractor={(l) => `${l.workspace}/${l.project}/${l.locale}`}
        contentContainerClassName="pt-2 pb-8"
        renderItem={renderItem}
        ListHeaderComponent={
          context.length > 0 ? (
            <View testID="inbox-context">
              {context.map((c) => (
                <ContextEntryCard
                  key={`${c.workspace}/${c.project}`}
                  testID={`inbox-context-${c.project}`}
                  count={c.waiting}
                  detail={[
                    whereOf(c, showWorkspace),
                    t("context.kinds", {
                      voice: c.by_kind.voice_rule ?? 0,
                      terms: c.by_kind.concept ?? 0,
                      rules: c.by_kind.term_rule ?? 0,
                      notes: c.by_kind.note ?? 0,
                    }),
                  ].join("\n")}
                  onPress={() => openContext(c)}
                />
              ))}
            </View>
          ) : undefined
        }
        refreshing={inbox.isRefetching}
        onRefresh={() => void refetch()}
        ListEmptyComponent={empty ?? <QueryState query={inbox} />}
      />
    </View>
  );
}
