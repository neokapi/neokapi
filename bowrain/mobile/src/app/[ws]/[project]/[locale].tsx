// The review deck for one language: swipe or tap to decide, one card at a
// time. Decisions go to the offline queue and are sent in order; the undo bar
// takes back the last one.
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
import * as Haptics from "expo-haptics";
import { useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ActivityIndicator, Pressable, ScrollView, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { SafeAreaView } from "react-native-safe-area-context";

import { ActionBar } from "../../../components/ActionBar";
import { FixSheet } from "../../../components/FixSheet";
import { ReviewCard } from "../../../components/ReviewCard";
import { RunText } from "../../../components/RunText";
import { SwipeCard, type SwipeAction, type SwipeCardHandle } from "../../../components/SwipeCard";
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
  const [fixing, setFixing] = useState<DeckCard | null>(null);
  const [undoable, setUndoable] = useState(false);

  const cards = useMemo(
    () => mergePages((deck.data?.pages ?? []).map((p) => p.cards)),
    [deck.data],
  );
  const view = useMemo(() => deckView(cards, new Set(decided), later), [cards, decided, later]);
  const top = view.swipe[0];
  const next = view.swipe[1];
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
      if (action === "approve") record(top, "approve");
      else if (action === "later") setLater((l) => [...l.filter((k) => k !== top.key), top.key]);
      else setFixing(top);
    },
    [top, record],
  );

  const onUndo = () => {
    undoLast(qc, (v) => withdraw.mutate(v));
    setUndoable(planUndo(qc) !== null);
  };

  const notices = useQuery<DecisionNotice[]>({
    queryKey: NOTICES_KEY,
    queryFn: () => qc.getQueryData<DecisionNotice[]>(NOTICES_KEY) ?? [],
    initialData: [],
    staleTime: Infinity,
  }).data.filter((n) => n.card.locale === locale);

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <View className="flex-row items-center justify-between px-4 py-2">
        <Text testID="deck-remaining" className="text-muted-foreground text-sm">
          {counts
            ? t("deck.remaining", { count: view.swipe.length, total: counts.total })
            : t("deck.remainingShort", { count: view.swipe.length })}
        </Text>
        <Text testID="deck-sync" className="text-muted-foreground text-sm">
          {!online
            ? t("deck.offline", { count: queued })
            : queued > 0
              ? t("deck.sending", { count: queued })
              : ""}
        </Text>
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

      <View className="flex-1 px-4 py-2">
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
            {next ? (
              <View className="absolute inset-0 scale-95 opacity-60" pointerEvents="none">
                <ReviewCard card={next} sourceLanguage={sourceLanguage} />
              </View>
            ) : null}
            <SwipeCard
              key={cardId(top)}
              ref={swipeRef}
              canApprove={!top.ownWork}
              onAction={onAction}
            >
              <ReviewCard card={top} sourceLanguage={sourceLanguage} />
            </SwipeCard>
          </View>
        ) : (
          <DeskList cards={view.desk} />
        )}
      </View>

      {top ? (
        <ActionBar
          canApprove={!top.ownWork}
          onAction={(a) => {
            if (swipeRef.current) swipeRef.current.swipe(a);
            else onAction(a);
          }}
        />
      ) : null}

      {undoable && planUndo(qc) ? (
        <View className="bg-foreground mx-4 mb-2 flex-row items-center justify-between rounded-xl px-4 py-3">
          <Text className="text-background text-sm">{t("undo.label")}</Text>
          <Pressable testID="undo" accessibilityRole="button" onPress={onUndo} hitSlop={12}>
            <Text className="text-background text-sm font-semibold">{t("undo.button")}</Text>
          </Pressable>
        </View>
      ) : null}

      <FixSheet
        card={fixing}
        loadFixes={(card, reason, ai) => client.fixCandidates(scope, card, { reason, ai })}
        onClose={() => setFixing(null)}
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

function DeskList({ cards }: { cards: DeckCard[] }) {
  const { t } = useTranslation();
  if (cards.length === 0) {
    return (
      <View testID="deck-done" className="flex-1 items-center justify-center">
        <Text className="text-foreground text-lg font-semibold">{t("deck.done")}</Text>
      </View>
    );
  }
  return (
    <ScrollView contentContainerClassName="gap-3 pb-6">
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
    </ScrollView>
  );
}
