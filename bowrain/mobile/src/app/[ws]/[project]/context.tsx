// The context deck of a project: the voice rules, terms, term rules and notes
// nobody has reviewed at their current revision, one card at a time. Right
// keeps a card (an attestation on the server), left retires it after a
// confirmation, up puts it at the end. The server says which actions each
// card offers and what each one does.
import type { ContextCard as Card, ContextDecision } from "@neokapi/bowrain-client";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as Haptics from "expo-haptics";
import { Stack, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Alert, Pressable, Text, View } from "react-native";
import { useSharedValue } from "react-native-reanimated";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { ActionBar } from "../../../components/ActionBar";
import { ContextCard } from "../../../components/ContextCard";
import { SafeAreaView } from "../../../components/SafeArea";
import {
  StackedCard,
  SwipeCard,
  type SwipeAction,
  type SwipeCardHandle,
} from "../../../components/SwipeCard";
import { useAuth } from "../../../lib/auth";
import {
  contextCardId,
  contextDeckView,
  mergeContextPages,
  refusalCode,
} from "../../../lib/contextDeck";

const PAGE = 50;
const STACK = 2;

export default function ContextDeckScreen() {
  const { t } = useTranslation();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const [pageBackground, pageForeground] = useCSSVariable([
    "--color-background",
    "--color-foreground",
  ]);

  const deck = useInfiniteQuery({
    queryKey: ["context-deck", ws, project],
    queryFn: ({ pageParam }) => client.contextDeck(ws, project, { limit: PAGE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.next_cursor,
  });

  const [decided, setDecided] = useState<string[]>([]);
  const [later, setLater] = useState<string[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const drag = useSharedValue(0);
  const swipeRef = useRef<SwipeCardHandle>(null);

  const cards = useMemo(
    () => mergeContextPages((deck.data?.pages ?? []).map((p) => p.cards)),
    [deck.data],
  );
  const view = useMemo(
    () => contextDeckView(cards, new Set(decided), later),
    [cards, decided, later],
  );
  const top = view[0];
  const behind = view.slice(1, 1 + STACK);
  const first = deck.data?.pages[0];
  const total = first?.counts.total ?? 0;

  const { hasNextPage, isFetchingNextPage, fetchNextPage } = deck;
  useEffect(() => {
    if (view.length < 5 && hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [view.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  const decide = useMutation({
    mutationFn: (d: ContextDecision) => client.decideContext(ws, project, d),
    onSuccess: (result) => {
      if (result.outcome === "proposed") setNotice(t("context.outcome.proposed"));
      void qc.invalidateQueries({ queryKey: ["review-inbox"] });
      void qc.invalidateQueries({ queryKey: ["context-count", ws, project] });
    },
    onError: (err, d) => {
      // The card comes back at the end of the deck with what the server said.
      const id = contextCardId(d);
      setDecided((ids) => ids.filter((x) => x !== id));
      setLater((ids) => [...ids.filter((x) => x !== id), id]);
      setNotice(
        t(`context.refused.${refusalCode(err)}`, { defaultValue: t("context.refused.failed") }),
      );
    },
  });

  const record = useCallback(
    (card: Card, action: "keep" | "reject") => {
      setDecided((ids) => [...ids, contextCardId(card)]);
      decide.mutate({ item: card.item, revision: card.revision, action });
      if (action === "keep")
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      else void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    },
    [decide],
  );

  const onAction = useCallback(
    (action: SwipeAction) => {
      if (!top) return;
      if (action === "approve") record(top, "keep");
      else if (action === "later") {
        const id = contextCardId(top);
        setLater((ids) => [...ids.filter((x) => x !== id), id]);
      } else if (action === "reject") {
        Alert.alert(t("context.retireTitle"), undefined, [
          {
            text: t("common.cancel"),
            style: "cancel",
            onPress: () => swipeRef.current?.restore("left"),
          },
          { text: t("context.retire"), style: "destructive", onPress: () => record(top, "reject") },
        ]);
      }
    },
    [top, record, t],
  );

  const canKeep = top?.actions.includes("keep") ?? false;
  const canReject = top?.actions.includes("reject") ?? false;
  const reviewed = decided.length;

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <Stack.Screen
        options={{
          headerStyle: { backgroundColor: String(pageBackground) },
          headerTintColor: String(pageForeground),
          headerShadowVisible: false,
        }}
      />
      <View className="gap-2 px-5 pt-2 pb-1">
        <View className="flex-row items-center justify-between">
          <Text testID="context-progress" className="text-foreground text-sm font-semibold">
            {total > 0
              ? t("deck.progress", { position: Math.min(reviewed + 1, total), total })
              : ""}
          </Text>
          {first ? (
            <Text testID="context-split" className="text-muted-foreground text-sm">
              {t("context.split", { agent: first.counts.agent, person: first.counts.person })}
            </Text>
          ) : null}
        </View>
        <View className="bg-muted h-1 overflow-hidden rounded-full" importantForAccessibility="no">
          <View
            className="from-prism-1 via-prism-3 to-prism-5 h-1 rounded-full bg-linear-to-r"
            style={{ width: `${total > 0 ? (reviewed / total) * 100 : 0}%` }}
          />
        </View>
      </View>

      {first && !first.bound ? (
        <Text className="text-muted-foreground px-5 pb-2 text-xs">{t("context.unbound")}</Text>
      ) : null}

      {notice ? (
        <View testID="context-notice" className="bg-warning mx-4 mb-2 gap-1 rounded-xl px-3 py-2">
          <Text className="text-warning-foreground text-sm">{notice}</Text>
          <Pressable accessibilityRole="button" onPress={() => setNotice(null)}>
            <Text className="text-warning-foreground text-sm font-semibold">
              {t("common.dismiss")}
            </Text>
          </Pressable>
        </View>
      ) : null}

      <View className="flex-1 px-5 pt-3 pb-6">
        {deck.isPending ? (
          <ActivityIndicator className="flex-1" />
        ) : deck.isError && cards.length === 0 ? (
          <View className="flex-1 items-center justify-center gap-3">
            <Text className="text-muted-foreground">{t("common.loadFailed")}</Text>
            <Pressable accessibilityRole="button" onPress={() => void deck.refetch()}>
              <Text className="text-primary">{t("common.retry")}</Text>
            </Pressable>
          </View>
        ) : top ? (
          <View className="flex-1">
            {behind
              .map((c, i) => (
                <StackedCard key={contextCardId(c)} depth={i + 1} progress={drag}>
                  <ContextCard card={c} />
                </StackedCard>
              ))
              .reverse()}
            <SwipeCard
              key={contextCardId(top)}
              ref={swipeRef}
              canApprove={canKeep}
              canReject={canReject}
              canFix={false}
              stamps={{ approve: t("context.keep"), reject: t("context.retire") }}
              onAction={onAction}
              progress={drag}
            >
              <ContextCard card={top} />
            </SwipeCard>
          </View>
        ) : (
          <Done reviewed={reviewed} />
        )}
      </View>

      {top ? (
        <ActionBar
          canApprove={canKeep}
          canReject={canReject}
          showFix={false}
          canUndo={false}
          onUndo={() => {}}
          labels={{
            approve: t("context.keep"),
            approveHint: t("context.keepHint"),
            approveDenied: t("context.ownWork"),
            reject: t("context.retire"),
            rejectHint: t("context.retireHint"),
          }}
          onAction={(a) => {
            if (swipeRef.current) swipeRef.current.swipe(a);
            else onAction(a);
          }}
        />
      ) : null}
    </SafeAreaView>
  );
}

function Done({ reviewed }: { reviewed: number }) {
  const { t } = useTranslation();
  const [approve] = useCSSVariable(["--color-approve"]);
  return (
    <View testID="context-done" className="flex-1 items-center justify-center gap-4">
      <Ionicons name="checkmark-circle" size={64} color={String(approve)} />
      <Text accessibilityRole="header" className="text-foreground text-2xl font-semibold">
        {t("context.doneTitle")}
      </Text>
      <Text className="text-muted-foreground text-base">
        {reviewed > 0 ? t("context.doneBody", { count: reviewed }) : t("context.doneEmpty")}
      </Text>
    </View>
  );
}
