// A language's review deck by page: each component and each document with the
// translations waiting in it. A page opens its container, where the reviewer
// decides its translations together; the swipe deck stays one tap away for
// quick strings. The server groups the pages and counts them
// (GET …/review-deck/:ref/pages); the app lists what it is given.
import type { DeckPage, DeckPages } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { fileName } from "../lib/where";

export interface PagesListProps {
  pages: DeckPages;
  onOpen: (page: DeckPage) => void;
}

export function PagesList({ pages, onOpen }: PagesListProps) {
  const { t } = useTranslation();
  if (pages.pages.length === 0) {
    return (
      <View testID="pages-empty" className="flex-1 items-center justify-center px-6">
        <Text className="text-muted-foreground text-center text-base">{t("pages.empty")}</Text>
      </View>
    );
  }
  return (
    <FlatList
      testID="pages-list"
      data={pages.pages}
      keyExtractor={(p) => `${p.item}\u0000${p.component ?? ""}`}
      contentContainerClassName="gap-2 pb-6"
      ListHeaderComponent={
        <Text className="text-muted-foreground pb-2 text-sm">{t("pages.lead")}</Text>
      }
      ListFooterComponent={
        pages.more ? (
          <Text className="text-muted-foreground pt-2 text-xs">{t("pages.more")}</Text>
        ) : undefined
      }
      renderItem={({ item }) => <PageRow page={item} onPress={() => onOpen(item)} />}
    />
  );
}

function PageRow({ page, onPress }: { page: DeckPage; onPress: () => void }) {
  const { t } = useTranslation();
  const [muted] = useCSSVariable(["--color-muted-foreground"]);
  const kind = t(`pages.kind.${page.kind}`);
  return (
    <Pressable
      testID={`page-${page.firstBlockId}`}
      accessibilityRole="button"
      accessibilityLabel={t("pages.rowLabel", { kind, title: page.title, count: page.waiting })}
      onPress={onPress}
      className="bg-card border-border flex-row items-center gap-3 rounded-xl border px-4 py-3 active:opacity-80"
    >
      <Ionicons
        name={page.kind === "component" ? "cube-outline" : "document-text-outline"}
        size={20}
        color={String(muted)}
      />
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-base font-semibold" numberOfLines={1}>
          {page.title}
        </Text>
        <Text className="text-muted-foreground font-mono text-xs" numberOfLines={1}>
          {page.kind === "component" ? `${kind} · ${fileName(page.item)}` : kind}
        </Text>
      </View>
      <Text className="text-muted-foreground text-sm tabular-nums">
        {t("pages.waiting", { count: page.waiting })}
      </Text>
      <Ionicons name="chevron-forward" size={16} color={String(muted)} />
    </Pressable>
  );
}
