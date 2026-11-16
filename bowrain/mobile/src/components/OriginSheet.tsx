// The origin sheet: the bulk operation that brought a context item in, opened
// from the provenance line on a context card. It says what the operation did,
// who ran it, from which file and host, and when, then lists the other items
// it brought in with the card it was opened from marked. The server names the
// operation and its items; the sheet lays them out.
import type { ContextCard, ContextOrigin, ContextOriginPage } from "@neokapi/bowrain-client";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useInfiniteQuery } from "@tanstack/react-query";
import { memo, useCallback, useState, type ComponentProps } from "react";
import { ActivityIndicator, FlatList, Modal, Pressable, Text, View } from "react-native";
import Animated, { FadeIn, FadeInDown } from "react-native-reanimated";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { contextCardId } from "../lib/contextDeck";
import { isCurrentCard, originSummary } from "../lib/contextOrigin";
import { Avatar } from "./feed/Avatar";
import { SafeAreaView } from "./SafeArea";

export interface OriginSheetProps {
  /** The card the sheet was opened from; null closes the sheet. */
  card: ContextCard | null;
  /** Read one page of the items the operation brought in. */
  load: (op: string, cursor: string | undefined) => Promise<ContextOriginPage>;
  onClose: () => void;
}

export function OriginSheet({ card, load, onClose }: OriginSheetProps) {
  const origin = card?.origin;
  return (
    <Modal
      visible={!!origin}
      animationType="slide"
      presentationStyle="pageSheet"
      onRequestClose={onClose}
    >
      {card && origin ? (
        <OriginSheetBody
          key={origin.id}
          card={card}
          origin={origin}
          load={load}
          onClose={onClose}
        />
      ) : null}
    </Modal>
  );
}

type IconName = ComponentProps<typeof Ionicons>["name"];

function originIcon(origin: ContextOrigin): IconName {
  if (origin.kind === "voice_edit" || origin.source === "voice") return "mic-outline";
  if (origin.source === "terms") return "book-outline";
  if (origin.source === "memory") return "library-outline";
  return "document-text-outline";
}

export function OriginSheetBody({
  card,
  origin,
  load,
  onClose,
}: {
  card: ContextCard;
  origin: ContextOrigin;
  load: OriginSheetProps["load"];
  onClose: () => void;
}) {
  const { t, i18n } = useTranslation();
  const [why, setWhy] = useState(false);
  const [muted, chipInk] = useCSSVariable(["--color-muted-foreground", "--color-chip-foreground"]);

  const items = useInfiniteQuery({
    queryKey: ["context-origin", origin.id],
    queryFn: ({ pageParam }) => load(origin.id, pageParam),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  });
  const first = items.data?.pages[0];
  const siblings = (items.data?.pages ?? []).flatMap((p) => p.cards);

  const author = origin.author;
  const name = author.name || t("context.unknownAuthor");
  const when = new Date(origin.at);
  const whenText = Number.isNaN(when.getTime())
    ? ""
    : `${when.toLocaleDateString(i18n.language, { dateStyle: "medium" })} · ${when.toLocaleTimeString(i18n.language, { timeStyle: "short" })}`;

  const renderItem = useCallback(
    ({ item, index }: { item: ContextCard; index: number }) => (
      <Sibling card={item} current={isCurrentCard(item, card)} index={index} />
    ),
    [card],
  );

  const header = (
    <View className="gap-5 pb-4">
      <View className="flex-row items-start gap-4">
        <View className="from-prism-1 to-prism-3 h-14 w-14 items-center justify-center rounded-2xl bg-linear-to-br">
          <Ionicons name={originIcon(origin)} size={28} color="#ffffff" />
        </View>
        <View className="min-w-0 flex-1 gap-1">
          <Text
            testID="origin-summary"
            accessibilityRole="header"
            className="text-foreground text-2xl leading-tight font-semibold"
          >
            {originSummary(t, origin)}
          </Text>
          {origin.kind === "voice_edit" && origin.count ? (
            <Text className="text-muted-foreground text-sm">
              {t("context.origin.fields", { count: origin.count })}
            </Text>
          ) : null}
        </View>
      </View>

      <View className="bg-card border-border gap-3 rounded-2xl border p-4">
        <View className="flex-row items-center gap-3" testID="origin-author">
          <Avatar
            actor={{ id: name, kind: author.agent_authored ? "agent" : author.kind, name }}
            name={name}
            size={36}
          />
          <View className="min-w-0 flex-1">
            <Text className="text-muted-foreground text-xs">{t("context.origin.ranBy")}</Text>
            <Text className="text-foreground text-base font-semibold" numberOfLines={1}>
              {name}
            </Text>
          </View>
          {author.host ? (
            <View
              testID="origin-host"
              className="bg-chip flex-row items-center gap-1 rounded-full px-3 py-1"
            >
              <Ionicons name="sparkles" size={12} color={String(chipInk)} />
              <Text className="text-chip-foreground text-xs font-semibold">{author.host}</Text>
            </View>
          ) : null}
        </View>
        {origin.path ? (
          <Fact icon="document-outline" label={t("context.origin.file")}>
            <Text testID="origin-path" className="text-foreground font-mono text-sm" selectable>
              {origin.path}
            </Text>
          </Fact>
        ) : null}
        {whenText ? (
          <Fact icon="time-outline" label={t("context.origin.when")}>
            <Text className="text-foreground text-sm">{whenText}</Text>
          </Fact>
        ) : null}
        {origin.note ? <Text className="text-muted-foreground text-sm">{origin.note}</Text> : null}
      </View>

      {first && first.total > 0 ? (
        <View className="gap-2">
          <Text testID="origin-waiting" className="text-foreground text-sm font-semibold">
            {t("context.origin.waiting", { pending: first.pending, total: first.total })}
          </Text>
          <View
            className="bg-muted h-1.5 overflow-hidden rounded-full"
            importantForAccessibility="no"
          >
            <View
              className="from-prism-1 via-prism-3 to-prism-5 h-1.5 rounded-full bg-linear-to-r"
              style={{ width: `${((first.total - first.pending) / first.total) * 100}%` }}
            />
          </View>
        </View>
      ) : null}

      <Pressable
        testID="origin-why"
        accessibilityRole="button"
        accessibilityState={{ expanded: why }}
        onPress={() => setWhy((w) => !w)}
        className="bg-muted gap-2 rounded-2xl px-4 py-3"
      >
        <View className="flex-row items-center gap-2">
          <Ionicons name="help-circle-outline" size={18} color={String(muted)} />
          <Text className="text-foreground flex-1 text-sm font-semibold">
            {t("context.origin.why")}
          </Text>
          <Ionicons name={why ? "chevron-up" : "chevron-down"} size={16} color={String(muted)} />
        </View>
        {why ? (
          <Animated.View entering={FadeIn.duration(180)} className="gap-2">
            <Text testID="origin-why-body" className="text-muted-foreground text-sm">
              {t("context.origin.whyBody")}
            </Text>
            {author.host ? (
              <Text className="text-muted-foreground text-sm">
                {t("context.origin.whyAgent", { host: author.host })}
              </Text>
            ) : null}
          </Animated.View>
        ) : null}
      </Pressable>

      <Text className="text-muted-foreground text-xs font-semibold tracking-wide uppercase">
        {t("context.origin.items")}
      </Text>
    </View>
  );

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <View className="items-center pt-2">
        <View className="bg-border h-1 w-10 rounded-full" importantForAccessibility="no" />
      </View>
      <View className="flex-row justify-end px-4 pt-1">
        <Pressable
          testID="origin-close"
          accessibilityRole="button"
          onPress={onClose}
          hitSlop={12}
          className="px-2 py-1"
        >
          <Text className="text-primary text-base font-semibold">{t("common.close")}</Text>
        </Pressable>
      </View>
      <FlatList
        data={siblings}
        keyExtractor={contextCardId}
        renderItem={renderItem}
        ListHeaderComponent={header}
        contentContainerClassName="gap-2 px-5 pb-8"
        ListEmptyComponent={
          items.isPending ? (
            <ActivityIndicator />
          ) : items.isError ? (
            <Pressable accessibilityRole="button" onPress={() => void items.refetch()}>
              <Text className="text-muted-foreground">{t("common.loadFailed")}</Text>
              <Text className="text-primary">{t("common.retry")}</Text>
            </Pressable>
          ) : (
            <Text className="text-muted-foreground text-sm">{t("context.origin.noItems")}</Text>
          )
        }
        ListFooterComponent={
          items.hasNextPage ? (
            <Pressable
              testID="origin-more"
              accessibilityRole="button"
              onPress={() => void items.fetchNextPage()}
              className="items-center py-3"
            >
              {items.isFetchingNextPage ? (
                <ActivityIndicator />
              ) : (
                <Text className="text-primary text-sm font-semibold">
                  {t("context.origin.more")}
                </Text>
              )}
            </Pressable>
          ) : undefined
        }
      />
    </SafeAreaView>
  );
}

function Fact({
  icon,
  label,
  children,
}: {
  icon: IconName;
  label: string;
  children: React.ReactNode;
}) {
  const muted = useCSSVariable("--color-muted-foreground");
  return (
    <View className="flex-row items-start gap-3">
      <View className="h-9 w-9 items-center justify-center">
        <Ionicons name={icon} size={18} color={String(muted)} />
      </View>
      <View className="min-w-0 flex-1">
        <Text className="text-muted-foreground text-xs">{label}</Text>
        {children}
      </View>
    </View>
  );
}

function TermChip({ text, locale }: { text: string; locale: string }) {
  return (
    <View className="bg-muted flex-row items-center gap-1 rounded-full px-2.5 py-1">
      <Text className="text-muted-foreground font-mono text-[10px] uppercase">{locale}</Text>
      <Text className="text-foreground text-xs">{text}</Text>
    </View>
  );
}

/** One item the operation brought in. A concept expands to its definition and its other languages. */
const Sibling = memo(function Sibling({
  card,
  current,
  index,
}: {
  card: ContextCard;
  current: boolean;
  index: number;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const approve = useCSSVariable("--color-approve");
  const muted = useCSSVariable("--color-muted-foreground");
  const terms = card.terms ?? [];
  const expandable = terms.length > 1 || !!card.text;

  return (
    <Animated.View entering={FadeInDown.delay(Math.min(index, 8) * 35).duration(220)}>
      <Pressable
        testID={current ? "origin-item-current" : "origin-item"}
        accessibilityRole={expandable ? "button" : undefined}
        accessibilityState={expandable ? { expanded: open } : undefined}
        disabled={!expandable}
        onPress={() => setOpen((o) => !o)}
        className={`gap-2 rounded-2xl border px-4 py-3 ${current ? "border-primary bg-card" : "border-border bg-card"}`}
      >
        <View className="flex-row items-center gap-2">
          <View className="min-w-0 flex-1">
            {terms.length > 0 ? (
              <Text className="text-foreground text-base font-semibold" numberOfLines={1}>
                {terms[0].text}
              </Text>
            ) : card.pattern ? (
              <View className="gap-0.5">
                <Text className="text-muted-foreground text-xs">
                  {t(`context.list.${card.pattern.list}`)}
                </Text>
                <Text className="text-foreground font-mono text-sm" numberOfLines={1}>
                  {card.pattern.regex}
                </Text>
              </View>
            ) : (
              <Text className="text-foreground text-sm" numberOfLines={2}>
                {card.text ?? card.term?.term}
              </Text>
            )}
          </View>
          {current ? (
            <View className="bg-primary rounded-full px-2 py-0.5">
              <Text className="text-primary-foreground text-[10px] font-semibold uppercase">
                {t("context.origin.thisCard")}
              </Text>
            </View>
          ) : null}
          {card.reviewed ? (
            <Ionicons
              name="checkmark-circle"
              size={18}
              color={String(approve)}
              accessibilityLabel={t("context.origin.reviewed")}
            />
          ) : null}
          {expandable ? (
            <Ionicons name={open ? "chevron-up" : "chevron-down"} size={14} color={String(muted)} />
          ) : null}
        </View>
        {card.pattern?.description ? (
          <Text className="text-muted-foreground text-sm" numberOfLines={open ? undefined : 2}>
            {card.pattern.description}
          </Text>
        ) : null}
        {terms.length > 1 && !open ? (
          <View className="flex-row flex-wrap gap-1.5">
            {terms.slice(1, 4).map((term, i) => (
              <TermChip
                key={`${term.locale}:${term.text}:${i}`}
                text={term.text}
                locale={term.locale}
              />
            ))}
          </View>
        ) : null}
        {open && terms.length > 0 ? (
          <Animated.View
            entering={FadeIn.duration(160)}
            className="gap-2"
            testID="origin-item-detail"
          >
            {card.text ? (
              <View className="gap-0.5">
                <Text className="text-muted-foreground text-xs">
                  {t("context.origin.definition")}
                </Text>
                <Text className="text-foreground text-sm">{card.text}</Text>
              </View>
            ) : null}
            {terms.length > 1 ? (
              <View className="gap-1">
                <Text className="text-muted-foreground text-xs">
                  {t("context.origin.otherForms")}
                </Text>
                <View className="flex-row flex-wrap gap-1.5">
                  {terms.slice(1).map((term, i) => (
                    <TermChip
                      key={`${term.locale}:${term.text}:${i}`}
                      text={term.text}
                      locale={term.locale}
                    />
                  ))}
                </View>
              </View>
            ) : null}
          </Animated.View>
        ) : null}
      </Pressable>
    </Animated.View>
  );
});
