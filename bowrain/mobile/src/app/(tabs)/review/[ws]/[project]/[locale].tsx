// One language's review deck split by collection, so a reviewer takes it one
// piece at a time. The server counts and orders the collections
// (GET /:ws/:id/review-deck/:ref/collections): those with anything waiting
// first, finished ones last. A row opens the deck on that collection, and the
// first row opens the whole language.
import type { DeckCollection } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useQuery } from "@tanstack/react-query";
import { router, Stack, useFocusEffect, useLocalSearchParams } from "expo-router";
import { memo, useCallback, useEffect } from "react";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { QueryState } from "../../../../../components/List";
import { useAuth } from "../../../../../lib/auth";
import { collectionParam, collectionsKey, decidedShare } from "../../../../../lib/deckCollections";
import { languageLabel } from "../../../../../lib/feed";

const CollectionRow = memo(function CollectionRow({
  item,
  name,
  testID,
  onPress,
}: {
  item: DeckCollection;
  name: string;
  testID: string;
  onPress: () => void;
}) {
  const { t } = useTranslation();
  const muted = useCSSVariable("--color-muted-foreground");
  const approve = useCSSVariable("--color-approve");
  const done = item.waiting === 0;
  const progress = t("collections.progress", { decided: item.decided, total: item.translations });
  const groups = t("languages.groups", { fast: item.fast, desk: item.desk });
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={t("collections.rowLabel", {
        name,
        count: item.waiting,
        decided: item.decided,
        total: item.translations,
      })}
      disabled={done}
      onPress={onPress}
      className="bg-card border-border gap-2 border-b px-4 py-3.5 active:opacity-70"
    >
      <View className="flex-row items-center gap-3">
        <View className="min-w-0 flex-1 gap-0.5">
          <Text className="text-foreground text-base font-medium" numberOfLines={1}>
            {name}
          </Text>
          <Text className="text-muted-foreground text-sm tabular-nums" numberOfLines={1}>
            {done ? progress : `${groups} · ${progress}`}
          </Text>
        </View>
        {done ? (
          <Ionicons name="checkmark-circle" size={22} color={String(approve)} />
        ) : (
          <>
            <View className="bg-primary/10 min-w-9 items-center rounded-full px-2.5 py-1">
              <Text className="text-primary text-sm font-semibold tabular-nums">
                {item.waiting}
              </Text>
            </View>
            <Ionicons name="chevron-forward" size={18} color={String(muted)} />
          </>
        )}
      </View>
      <View className="bg-muted h-1 overflow-hidden rounded-full" importantForAccessibility="no">
        <View
          className="from-prism-1 via-prism-3 to-prism-5 h-1 rounded-full bg-linear-to-r"
          style={{ width: `${decidedShare(item) * 100}%` }}
        />
      </View>
    </Pressable>
  );
});

export default function LanguageCollections() {
  const { t, i18n } = useTranslation();
  const { ws, project, locale } = useLocalSearchParams<{
    ws: string;
    project: string;
    locale: string;
  }>();
  const { client } = useAuth();
  const list = useQuery({
    queryKey: collectionsKey(ws, project, locale),
    queryFn: () => client.deckCollections({ workspace: ws, project }, locale),
  });
  const { refetch } = list;

  // Back from a deck, the counts have moved.
  useFocusEffect(
    useCallback(() => {
      void refetch();
    }, [refetch]),
  );

  const openDeck = useCallback(
    (collection?: DeckCollection, name?: string) => {
      router.push({
        pathname: "/[ws]/[project]/[locale]",
        params: collection
          ? { ws, project, locale, collection: collectionParam(collection.collection_id), name }
          : { ws, project, locale },
      });
    },
    [ws, project, locale],
  );

  // A server without the route has no split to offer: the whole deck opens.
  useEffect(() => {
    if (list.isSuccess && list.data === null) {
      router.replace({ pathname: "/[ws]/[project]/[locale]", params: { ws, project, locale } });
    }
  }, [list.isSuccess, list.data, ws, project, locale]);

  const nameOf = useCallback(
    (c: DeckCollection) => (c.collection_id === "" ? t("collections.none") : c.name),
    [t],
  );
  const renderItem = useCallback(
    ({ item }: { item: DeckCollection }) => (
      <CollectionRow
        item={item}
        name={nameOf(item)}
        testID={`collection-${item.collection_id || "none"}`}
        onPress={() => openDeck(item, nameOf(item))}
      />
    ),
    [nameOf, openDeck],
  );

  const data = list.data;
  return (
    <View collapsable={false} className="bg-background flex-1">
      <Stack.Screen options={{ title: languageLabel(locale, i18n.language) }} />
      <FlatList
        testID="collection-list"
        contentInsetAdjustmentBehavior="automatic"
        data={data?.collections ?? []}
        keyExtractor={(c) => c.collection_id || "-"}
        renderItem={renderItem}
        refreshing={list.isRefetching}
        onRefresh={() => void refetch()}
        ListHeaderComponent={
          data ? (
            <View className="pb-4">
              <CollectionRow
                item={data.all}
                name={t("collections.all")}
                testID="collection-all"
                onPress={() => openDeck()}
              />
            </View>
          ) : undefined
        }
        ListEmptyComponent={
          <QueryState
            query={list}
            empty={list.isSuccess && data ? t("collections.empty") : false}
          />
        }
      />
    </View>
  );
}
