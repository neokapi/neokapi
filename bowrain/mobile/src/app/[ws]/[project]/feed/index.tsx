// A project's feed: how the project changed, newest first, under a sticky
// header per day. A row opens the review deck it links to, or the entry's
// detail. New entries arrive over the workspace event stream.
import { groupFeedByDay, type FeedEntry } from "@neokapi/bowrain-client";
import { useQueryClient } from "@tanstack/react-query";
import { router, useLocalSearchParams } from "expo-router";
import { useMemo } from "react";
import { ActivityIndicator, SectionList, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { FeedRow } from "../../../../components/FeedRow";
import { QueryState } from "../../../../components/List";
import { dayLabel, deckLocaleOf, entriesOf, feedKey, useFeed } from "../../../../lib/feed";
import { useFeedEvents } from "../../../../lib/feedEvents";

export default function Feed() {
  const { t, i18n } = useTranslation();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
  const qc = useQueryClient();
  const feed = useFeed(ws, project);
  useFeedEvents(ws, project);

  const sections = useMemo(
    () =>
      groupFeedByDay(entriesOf(feed.data)).map((day) => ({
        key: day.key,
        title: dayLabel(day.daysAgo, day.date, t, i18n.language),
        data: day.entries,
      })),
    [feed.data, t, i18n.language],
  );

  const open = (entry: FeedEntry) => {
    const locale = deckLocaleOf(entry);
    if (locale) {
      router.push({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } });
      return;
    }
    router.push({
      pathname: "/[ws]/[project]/feed/[entry]",
      params: { ws, project, entry: entry.id },
    });
  };

  return (
    <View className="bg-background flex-1">
      <SectionList
        testID="feed-list"
        contentInsetAdjustmentBehavior="automatic"
        sections={sections}
        keyExtractor={(e) => e.id}
        stickySectionHeadersEnabled
        refreshing={feed.isRefetching && !feed.isFetchingNextPage}
        onRefresh={() => void qc.invalidateQueries({ queryKey: feedKey(ws, project) })}
        onEndReachedThreshold={0.5}
        onEndReached={() => {
          if (feed.hasNextPage && !feed.isFetchingNextPage) void feed.fetchNextPage();
        }}
        renderSectionHeader={({ section }) => (
          <View className="bg-background border-border border-b px-4 pt-4 pb-1.5">
            <Text className="text-muted-foreground text-sm font-semibold">{section.title}</Text>
          </View>
        )}
        renderItem={({ item }) => <FeedRow entry={item} onPress={() => open(item)} />}
        ListFooterComponent={
          feed.isFetchingNextPage ? (
            <View className="items-center p-4">
              <ActivityIndicator />
            </View>
          ) : undefined
        }
        ListEmptyComponent={<QueryState query={feed} empty={feed.isSuccess && t("feed.empty")} />}
      />
    </View>
  );
}
