// A project's feed: how the project changed, newest first, as cards under a
// header per day. A card opens what it links to (a language's deck, or the
// entry's detail); a language chip in a card opens that language's deck.
// Quiet runs are left out and one actor's changes close together fold into
// one card. New entries arrive over the workspace event stream.
import { foldFeed, groupFeedByDay, type FeedEntry } from "@neokapi/bowrain-client";
import { useQueryClient } from "@tanstack/react-query";
import { router, useLocalSearchParams } from "expo-router";
import { useCallback, useMemo } from "react";
import { ActivityIndicator, SectionList, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { FeedCardItem } from "../../../../components/feed/FeedCard";
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
      const locale = deckLocaleOf(entry);
      if (locale) {
        openLanguage(locale);
        return;
      }
      router.push({
        pathname: "/[ws]/[project]/feed/[entry]",
        params: { ws, project, entry: entry.id },
      });
    },
    [ws, project, openLanguage],
  );

  return (
    <View className="bg-background flex-1">
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
        renderSectionHeader={({ section }) => (
          <View className="px-5 pt-5 pb-2">
            <Text className="text-foreground text-[17px] font-bold">{section.title}</Text>
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
    </View>
  );
}
