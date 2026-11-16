// The review inbox: the projects whose context waits for the person's review,
// then every language they may review, across projects and workspaces, where
// translations are waiting, most waiting first. The server lists both, counts
// them as each deck counts them and orders them (GET /auth/me/review); the app
// shows the lists as they come.
import type { ContextInboxEntry, ReviewInboxLanguage } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useQuery } from "@tanstack/react-query";
import { router, useFocusEffect } from "expo-router";
import { memo, useCallback } from "react";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { QueryState } from "../../../components/List";
import { useAuth } from "../../../lib/auth";
import { languageLabel } from "../../../lib/feed";
import { setRecentProject } from "../../../lib/recentProject";

const inboxKey = ["review-inbox"] as const;

const InboxRow = memo(function InboxRow({
  item,
  showWorkspace,
}: {
  item: ReviewInboxLanguage;
  showWorkspace: boolean;
}) {
  const { t, i18n } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  const name = languageLabel(item.locale, i18n.language);
  const where = showWorkspace ? `${item.project_name} · ${item.workspace_name}` : item.project_name;
  const open = () => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws: item.workspace, project: item.project, locale: item.locale },
    });
  };
  return (
    <Pressable
      testID={`inbox-${item.project}-${item.locale}`}
      accessibilityRole="button"
      accessibilityLabel={t("inbox.rowLabel", { language: name, where, count: item.waiting })}
      onPress={open}
      className="bg-card border-border flex-row items-center gap-3 border-b px-4 py-3.5 active:opacity-70"
    >
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-base font-medium">{name}</Text>
        <Text className="text-muted-foreground text-sm" numberOfLines={1}>
          {where}
        </Text>
      </View>
      <View className="bg-primary/10 min-w-9 items-center rounded-full px-2.5 py-1">
        <Text className="text-primary text-sm font-semibold tabular-nums">{item.waiting}</Text>
      </View>
      <Ionicons name="chevron-forward" size={18} color={String(muted)} />
    </Pressable>
  );
});

const ContextRow = memo(function ContextRow({
  item,
  showWorkspace,
}: {
  item: ContextInboxEntry;
  showWorkspace: boolean;
}) {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  const where = showWorkspace ? `${item.project_name} · ${item.workspace_name}` : item.project_name;
  const open = () => {
    setRecentProject({ ws: item.workspace, project: item.project, name: item.project_name });
    router.push({
      pathname: "/[ws]/[project]/context",
      params: { ws: item.workspace, project: item.project },
    });
  };
  return (
    <Pressable
      testID={`inbox-context-${item.project}`}
      accessibilityRole="button"
      accessibilityLabel={t("context.entryLabel", { where, count: item.waiting })}
      onPress={open}
      className="bg-card border-border flex-row items-center gap-3 border-b px-4 py-3.5 active:opacity-70"
    >
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-base font-medium">
          {t("context.entry", { count: item.waiting })}
        </Text>
        <Text className="text-muted-foreground text-sm" numberOfLines={1}>
          {where}
        </Text>
        <Text className="text-muted-foreground text-xs tabular-nums" numberOfLines={1}>
          {t("context.kinds", {
            voice: item.by_kind.voice_rule ?? 0,
            terms: item.by_kind.concept ?? 0,
            rules: item.by_kind.term_rule ?? 0,
            notes: item.by_kind.note ?? 0,
          })}
        </Text>
      </View>
      <Ionicons name="chevron-forward" size={18} color={String(muted)} />
    </Pressable>
  );
});

function Empty({
  icon,
  title,
  body,
}: {
  icon: "checkmark-done" | "server";
  title: string;
  body?: string;
}) {
  const muted = useCSSVariable("--color-muted-foreground");
  return (
    <View className="items-center gap-2 px-8 pt-16">
      <Ionicons name={icon} size={44} color={String(muted)} />
      <Text className="text-foreground text-center text-lg font-semibold">{title}</Text>
      {body ? <Text className="text-muted-foreground text-center text-base">{body}</Text> : null}
    </View>
  );
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
  const renderItem = useCallback(
    ({ item }: { item: ReviewInboxLanguage }) => (
      <InboxRow item={item} showWorkspace={showWorkspace} />
    ),
    [showWorkspace],
  );

  let empty = null;
  if (inbox.isSuccess && inbox.data === null) {
    empty = (
      <Empty icon="server" title={t("inbox.unsupported")} body={t("inbox.unsupportedBody")} />
    );
  } else if (inbox.isSuccess && context.length === 0) {
    empty = (
      <Empty icon="checkmark-done" title={t("inbox.emptyTitle")} body={t("inbox.emptyBody")} />
    );
  }

  return (
    <View collapsable={false} className="bg-background flex-1">
      <FlatList
        testID="inbox-list"
        contentInsetAdjustmentBehavior="automatic"
        data={languages}
        keyExtractor={(l) => `${l.workspace}/${l.project}/${l.locale}`}
        renderItem={renderItem}
        ListHeaderComponent={
          context.length > 0 ? (
            <View testID="inbox-context">
              {context.map((c) => (
                <ContextRow
                  key={`${c.workspace}/${c.project}`}
                  item={c}
                  showWorkspace={showWorkspace}
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
