// The review deck for one language: a stack of cards, swiped or tapped to
// decide, one at a time. Decisions go to the offline queue and are sent in
// order; Undo takes back the last one and the card comes back from the side
// it left by.
import type { DeckCard, FixReason, Project, ProjectScope } from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import {
  onlineManager,
  useInfiniteQuery,
  useMutation,
  useMutationState,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import * as Crypto from "expo-crypto";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as Haptics from "expo-haptics";
import { Stack, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ActivityIndicator, Platform, Pressable, ScrollView, Text, View } from "react-native";
import { useSharedValue } from "react-native-reanimated";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";
import { SafeAreaView } from "../../../components/SafeArea";

import { ActionBar } from "../../../components/ActionBar";
import { FixSheet } from "../../../components/FixSheet";
import { ReviewCard } from "../../../components/ReviewCard";
import { RunText } from "../../../components/RunText";
import {
  StackedCard,
  SwipeCard,
  type SwipeAction,
  type SwipeCardHandle,
  type SwipeSide,
} from "../../../components/SwipeCard";
import { useAuth } from "../../../lib/auth";
import {
  DECISION_KEY,
  dismissNotice,
  NOTICES_KEY,
  planUndo,
  undoLast,
  WITHDRAW_KEY,
  type DecisionKind,
  type DecisionNotice,
  type DecisionResult,
  type DecisionVars,
  type WithdrawVars,
} from "../../../lib/decisions";
import { cardId, deckView, mergePages } from "../../../lib/deckView";

const PAGE = 50;

/** How many cards wait visibly behind the top one. */
const STACK = 2;

const displayFont = Platform.select({ ios: "ui-serif", default: "serif" });

function useOnline(): boolean {
  return useSyncExternalStore(
    (cb) => onlineManager.subscribe(cb),
    () => onlineManager.isOnline(),
  );
}

export default function DeckScreen() {
  const { t } = useTranslation();
  const { ws, project, locale } = useLocalSearchParams<{
    ws: string;
    project: string;
    locale: string;
  }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const online = useOnline();
  const [pageBackground, pageForeground] = useCSSVariable([
    "--color-background",
    "--color-foreground",
  ]);
  const scope: ProjectScope = useMemo(() => ({ workspace: ws, project }), [ws, project]);
  const sourceLanguage =
    qc.getQueryData<Project[]>(["projects", ws])?.find((p) => p.id === project)
      ?.default_source_language ?? "";

  const deck = useInfiniteQuery({
    queryKey: ["deck", ws, project, locale],
    queryFn: ({ pageParam }) => client.deck(scope, locale, { limit: PAGE, cursor: pageParam }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor,
  });

  // The cards this person decided on this device and the server has not yet
  // dropped from the deck: queued, in flight, or landed.
  const decided = useMutationState({
    filters: { mutationKey: DECISION_KEY },
    select: (m) => {
      const v = m.state.variables as DecisionVars | undefined;
      const data = m.state.data as DecisionResult | undefined;
      if (!v) return "";
      if (m.state.status === "pending") return cardId(v.card);
      if (
        m.state.status === "success" &&
        (data?.outcome === "applied" || data?.outcome === "held")
      ) {
        return cardId(v.card);
      }
      return "";
    },
  });
  const queued = useMutationState({
    filters: { mutationKey: DECISION_KEY, status: "pending" },
    select: (m) => m.state.submittedAt,
  }).length;

  const [later, setLater] = useState<string[]>([]);
  // The sheet, and whether the card left the screen for it (a reject swipe).
  const [fixing, setFixing] = useState<{
    card: DeckCard;
    mode: "fix" | "reject";
    flown: boolean;
  } | null>(null);
  const [undoable, setUndoable] = useState(false);
  // The cards decided in this visit, for the progress line.
  const [reviewed, setReviewed] = useState<string[]>([]);
  // A card brought back by Undo, and the side it slides in from.
  const [entering, setEntering] = useState<{ id: string; from: SwipeSide } | null>(null);
  const drag = useSharedValue(0);

  const cards = useMemo(
    () => mergePages((deck.data?.pages ?? []).map((p) => p.cards)),
    [deck.data],
  );
  const view = useMemo(() => deckView(cards, new Set(decided), later), [cards, decided, later]);
  const top = view.swipe[0];
  const behind = view.swipe.slice(1, 1 + STACK);
  const total = reviewed.length + view.swipe.length;
  const ordered = deck.data?.pages[0]?.ordered ?? true;
  const counts = deck.data?.pages[0]?.counts;

  // Read the next page before the deck runs out.
  const { hasNextPage, isFetchingNextPage, fetchNextPage } = deck;
  useEffect(() => {
    if (view.swipe.length < 5 && hasNextPage && !isFetchingNextPage) void fetchNextPage();
  }, [view.swipe.length, hasNextPage, isFetchingNextPage, fetchNextPage]);

  // Cards the server has not classified yet arrive on a later request: ask
  // again when the deck runs out while some remain.
  const unclassified = counts?.unclassified ?? 0;
  const { isFetching, refetch } = deck;
  useEffect(() => {
    if (view.swipe.length === 0 && !hasNextPage && unclassified > 0 && !isFetching) void refetch();
  }, [view.swipe.length, hasNextPage, unclassified, isFetching, refetch]);

  const decide = useMutation<DecisionResult, unknown, DecisionVars>({ mutationKey: DECISION_KEY });
  const withdraw = useMutation<unknown, unknown, WithdrawVars>({ mutationKey: WITHDRAW_KEY });

  const record = useCallback(
    (
      card: DeckCard,
      kind: DecisionKind,
      extra: { runs?: Run[]; reasons?: FixReason[]; note?: string } = {},
    ) => {
      decide.mutate({ id: Crypto.randomUUID(), scope, card, kind, madeAt: Date.now(), ...extra });
      setUndoable(true);
      setReviewed((r) => [...r, cardId(card)]);
      if (kind === "approve")
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      else void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    },
    [decide, scope],
  );

  const swipeRef = useRef<SwipeCardHandle>(null);

  const onAction = useCallback(
    (action: SwipeAction) => {
      if (!top) return;
      setEntering(null);
      if (action === "approve") record(top, "approve");
      else if (action === "later") setLater((l) => [...l.filter((k) => k !== top.key), top.key]);
      else
        setFixing({
          card: top,
          mode: action === "reject" ? "reject" : "fix",
          flown: action === "reject",
        });
    },
    [top, record],
  );

  const onUndo = () => {
    const plan = planUndo(qc);
    const card = undoLast(qc, (v) => withdraw.mutate(v));
    setUndoable(planUndo(qc) !== null);
    if (!card || !plan) return;
    const id = cardId(card);
    setReviewed((r) => r.filter((k) => k !== id));
    setEntering({ id, from: plan.vars.kind === "reject" ? "left" : "right" });
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const closeSheet = () => {
    if (fixing?.flown) swipeRef.current?.restore("left");
    setFixing(null);
  };
  // Whether Undo applies changes as a decision moves from queued to sent,
  // which `decided` above does not notice. The states are read into the
  // condition so a memoizing compiler recomputes it when any of them changes.
  const decisionStates = useMutationState({
    filters: { mutationKey: DECISION_KEY },
    select: (m) => `${m.state.status}:${m.state.isPaused}`,
  });
  const canUndo = undoable && decisionStates.length > 0 && planUndo(qc) !== null;

  const notices = useQuery<DecisionNotice[]>({
    queryKey: NOTICES_KEY,
    queryFn: () => qc.getQueryData<DecisionNotice[]>(NOTICES_KEY) ?? [],
    initialData: [],
    staleTime: Infinity,
  }).data.filter((n) => n.card.locale === locale);

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      {/* The header takes the page colour, so the deck reads as one surface in both themes. */}
      <Stack.Screen
        options={{
          headerStyle: { backgroundColor: String(pageBackground) },
          headerTintColor: String(pageForeground),
          headerShadowVisible: false,
        }}
      />
      <View className="gap-2 px-5 pt-2 pb-1">
        <View className="flex-row items-center justify-between">
          <Text
            testID="deck-remaining"
            className="text-foreground text-sm font-semibold"
            accessibilityLabel={
              counts
                ? t("deck.remaining", { count: view.swipe.length, total: counts.total })
                : t("deck.remainingShort", { count: view.swipe.length })
            }
          >
            {total > 0
              ? t("deck.progress", {
                  position: Math.min(reviewed.length + 1, total),
                  total,
                })
              : ""}
          </Text>
          <Text testID="deck-sync" className="text-muted-foreground text-sm">
            {!online
              ? t("deck.offline", { count: queued })
              : queued > 0
                ? t("deck.sending", { count: queued })
                : ""}
          </Text>
        </View>
        <View className="bg-muted h-1 overflow-hidden rounded-full" importantForAccessibility="no">
          <View
            testID="deck-progress"
            className="from-prism-1 via-prism-3 to-prism-5 h-1 rounded-full bg-linear-to-r"
            style={{ width: `${total > 0 ? (reviewed.length / total) * 100 : 0}%` }}
          />
        </View>
      </View>

      {unclassified > 0 ? (
        <Text testID="deck-unclassified" className="text-muted-foreground px-4 pb-2 text-xs">
          {t("deck.unclassified", { count: unclassified })}
        </Text>
      ) : null}

      {!ordered ? (
        <Text className="text-muted-foreground px-4 pb-2 text-xs">{t("deck.unordered")}</Text>
      ) : null}

      {notices.map((n) => (
        <View
          key={n.id}
          testID={`notice-${n.kind}`}
          className="bg-warning mx-4 mb-2 gap-1 rounded-xl px-3 py-2"
        >
          <Text className="text-warning-foreground text-sm">{t(`notice.${n.kind}`)}</Text>
          {n.message && n.kind === "refused" ? (
            <Text className="text-warning-foreground text-xs">{n.message}</Text>
          ) : null}
          <Pressable accessibilityRole="button" onPress={() => dismissNotice(qc, n.id)}>
            <Text className="text-warning-foreground text-sm font-semibold">
              {t("common.dismiss")}
            </Text>
          </Pressable>
        </View>
      ))}

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
                <StackedCard key={cardId(c)} depth={i + 1} progress={drag}>
                  <ReviewCard card={c} sourceLanguage={sourceLanguage} />
                </StackedCard>
              ))
              .reverse()}
            <SwipeCard
              key={cardId(top)}
              ref={swipeRef}
              canApprove={!top.ownWork}
              onAction={onAction}
              progress={drag}
              enterFrom={entering?.id === cardId(top) ? entering.from : null}
            >
              <ReviewCard card={top} sourceLanguage={sourceLanguage} />
            </SwipeCard>
          </View>
        ) : (
          <EndState reviewed={reviewed.length} desk={view.desk} canUndo={canUndo} onUndo={onUndo} />
        )}
      </View>

      {top ? (
        <ActionBar
          canApprove={!top.ownWork}
          canUndo={canUndo}
          onUndo={onUndo}
          onAction={(a) => {
            if (swipeRef.current) swipeRef.current.swipe(a);
            else onAction(a);
          }}
        />
      ) : null}

      <FixSheet
        card={fixing?.card ?? null}
        mode={fixing?.mode}
        loadFixes={(card, reason, ai) => client.fixCandidates(scope, card, { reason, ai })}
        onClose={closeSheet}
        onFix={(card, runs, reasons) => {
          record(card, "fix", { runs, reasons });
          setFixing(null);
        }}
        onReject={(card, reasons, note) => {
          record(card, "reject", { reasons, note: note || undefined });
          setFixing(null);
        }}
      />
    </SafeAreaView>
  );
}

/** The deck after its last card: what was done here, then the desk cards. */
function EndState({
  reviewed,
  desk,
  canUndo,
  onUndo,
}: {
  reviewed: number;
  desk: DeckCard[];
  canUndo: boolean;
  onUndo: () => void;
}) {
  const { t } = useTranslation();
  const [approve] = useCSSVariable(["--color-approve"]);
  return (
    <ScrollView contentContainerClassName="grow gap-8 pb-6">
      <View testID="deck-done" className="grow items-center justify-center gap-4 py-10">
        <View className="from-prism-1 via-prism-3 to-prism-5 h-28 w-28 rounded-full bg-linear-to-br p-1">
          <View className="bg-background flex-1 items-center justify-center rounded-full">
            <Ionicons name="checkmark" size={56} color={String(approve)} />
          </View>
        </View>
        <Text
          accessibilityRole="header"
          className="text-foreground text-3xl font-semibold tracking-tight"
          style={{ fontFamily: displayFont }}
        >
          {t("deck.doneTitle")}
        </Text>
        <Text className="text-muted-foreground px-6 text-center text-base">
          {reviewed > 0 ? t("deck.doneBody", { count: reviewed }) : t("deck.doneEmpty")}
        </Text>
        {canUndo ? (
          <Pressable
            testID="undo"
            accessibilityRole="button"
            accessibilityHint={t("undo.hint")}
            onPress={onUndo}
            hitSlop={12}
            className="border-border bg-card flex-row items-center gap-2 rounded-full border px-4 py-2 active:opacity-80"
          >
            <Ionicons name="arrow-undo" size={16} color={String(approve)} />
            <Text className="text-foreground text-sm font-semibold">{t("undo.button")}</Text>
          </Pressable>
        ) : null}
      </View>
      {desk.length > 0 ? <DeskList cards={desk} /> : null}
    </ScrollView>
  );
}

function DeskList({ cards }: { cards: DeckCard[] }) {
  const { t } = useTranslation();
  return (
    <View className="gap-3">
      <Text className="text-foreground text-base font-semibold">
        {t("deck.deskTitle", { count: cards.length })}
      </Text>
      <Text className="text-muted-foreground text-sm">{t("deck.deskLead")}</Text>
      {cards.map((c) => (
        <View
          key={c.key}
          testID="desk-card"
          className="bg-card border-border gap-2 rounded-xl border p-3"
        >
          <RunText runs={c.source} />
          {c.reason ? <Text className="text-muted-foreground text-xs">{c.reason}</Text> : null}
        </View>
      ))}
    </View>
  );
}
