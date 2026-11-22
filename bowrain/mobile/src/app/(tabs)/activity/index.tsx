// The Activity tab: a project's feed, how the project changed, newest first,
// as cards under a header per day. The project is the one the person last
// opened, and the button at the top picks another. A card opens what it links
// to (a language's deck, or the entry's detail); a language chip in a card
// opens that language's deck. Quiet runs are left out and one actor's changes
// close together fold into one card. New entries arrive over the workspace
// event stream.
import { foldFeed, groupFeedByDay, type FeedEntry } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useQueryClient } from "@tanstack/react-query";
import { router } from "expo-router";
import { useCallback, useMemo } from "react";
import { ActivityIndicator, Pressable, SectionList, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { FeedCardItem } from "../../../components/feed/FeedCard";
import { QueryState } from "../../../components/List";
import { dayLabel, deckLocaleOf, entriesOf, feedKey, useFeed } from "../../../lib/feed";
import { useFeedEvents } from "../../../lib/feedEvents";
import { useRecentProject, type RecentProject } from "../../../lib/recentProject";

function openPicker() {
  router.push("/activity/pick");
}

/** The button that names the feed's project and picks another. */
function ProjectButton({ name }: { name: string }) {
  const { t } = useTranslation();
  const [primary] = useCSSVariable(["--color-primary"]);
  return (
    <View className="flex-row px-4 pt-2 pb-1">
      <Pressable
        testID="activity-project"
        accessibilityRole="button"
        accessibilityLabel={t("activity.projectLabel", { name })}
        accessibilityHint={t("activity.pickHint")}
        onPress={openPicker}
        className="bg-card border-border flex-row items-center gap-2 rounded-full border px-3.5 py-2 active:opacity-70"
      >
        <Ionicons name="folder-open-outline" size={17} color={String(primary)} />
        <Text className="text-foreground text-base font-medium" numberOfLines={1}>
          {name}
        </Text>
        <Ionicons name="chevron-down" size={15} color={String(primary)} />
      </Pressable>
    </View>
  );
}

function ProjectFeed({ recent }: { recent: RecentProject }) {
  const { t, i18n } = useTranslation();
  const { ws, project } = recent;
  const qc = useQueryClient();
  const feed = useFeed(ws, project);
  useFeedEvents(ws, project);

  const sections = useMemo(
    () =>
      groupFeedByDay(entriesOf(feed.data))
        .map((day) => ({
          key: day.key,
          title: dayLabel(day.daysAgo, day.date, t, i18n.language),
          data: foldFeed(day.entries),
        }))
        .filter((s) => s.data.length > 0),
    [feed.data, t, i18n.language],
  );

  const openLanguage = useCallback(
    (locale: string) =>
      router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } }),
    [ws, project],
  );

  const open = useCallback(
    (entry: FeedEntry) => {
      // A review entry opens on its decisions, where each can be undone; the
      // deck is one button further.
      const locale = entry.kind === "review_summary" ? undefined : deckLocaleOf(entry);
      if (locale) {
        openLanguage(locale);
        return;
      }
      router.push({
        pathname: "/activity/[ws]/[project]/[entry]",
        params: { ws, project, entry: entry.id },
      });
    },
    [ws, project, openLanguage],
  );

  return (
    <SectionList
      testID="feed-list"
      contentInsetAdjustmentBehavior="automatic"
      sections={sections}
      keyExtractor={(card) => card.key}
      stickySectionHeadersEnabled={false}
      contentContainerClassName="pb-8"
      refreshing={feed.isRefetching && !feed.isFetchingNextPage}
      onRefresh={() => void qc.invalidateQueries({ queryKey: feedKey(ws, project) })}
      onEndReachedThreshold={0.5}
      onEndReached={() => {
        if (feed.hasNextPage && !feed.isFetchingNextPage) void feed.fetchNextPage();
      }}
      ListHeaderComponent={<ProjectButton name={recent.name ?? project} />}
      renderSectionHeader={({ section }) => (
        <View className="px-5 pt-5 pb-2">
          <Text accessibilityRole="header" className="text-foreground text-lg font-bold">
            {section.title}
          </Text>
        </View>
      )}
      renderItem={({ item }) => (
        <FeedCardItem card={item} onOpen={open} onOpenLanguage={openLanguage} />
      )}
      ListFooterComponent={
        feed.isFetchingNextPage ? (
          <View className="items-center p-4">
            <ActivityIndicator />
          </View>
        ) : undefined
      }
      ListEmptyComponent={<QueryState query={feed} empty={feed.isSuccess && t("feed.empty")} />}
    />
  );
}

function NoProject() {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  return (
    <View className="items-center gap-3 px-8 pt-24">
      <Ionicons name="time-outline" size={44} color={String(muted)} />
      <Text className="text-foreground text-center text-lg font-semibold">
        {t("activity.noProject")}
      </Text>
      <Text className="text-muted-foreground text-center text-base">
        {t("activity.noProjectBody")}
      </Text>
      <Pressable
        testID="activity-pick"
        accessibilityRole="button"
        onPress={openPicker}
        className="bg-primary mt-2 rounded-full px-5 py-3 active:opacity-80"
      >
        <Text className="text-primary-foreground text-base font-semibold">
          {t("activity.pick")}
        </Text>
      </Pressable>
    </View>
  );
}

export default function Activity() {
  const recent = useRecentProject();
  return (
    <View collapsable={false} className="bg-background flex-1">
      {recent ? (
        <ProjectFeed key={`${recent.ws}/${recent.project}`} recent={recent} />
      ) : (
        <NoProject />
      )}
    </View>
  );
}
