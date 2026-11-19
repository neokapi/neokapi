// The review deck for one language. It opens on pages: each component and
// each document with its waiting translations, reviewed together in place
// (page.tsx). Cards is the other view: a stack of cards, swiped or tapped to
// decide, one at a time, for quick strings. A decision waits on the device for a few seconds
// under an Undo bar, then goes to the offline queue and is sent in order.
// Undo takes back the last one and the card comes back from the side it left
// by; Reviewed lists every decision made here, each with its own Undo. Opened from a language's collections, the deck holds one
// collection, and its end offers the next one with anything waiting.
import type {
  DeckCard,
  DeckCollection,
  FixReason,
  Project,
  ProjectScope,
} from "@neokapi/bowrain-client";
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
import { router, Stack, useFocusEffect, useLocalSearchParams } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { ActivityIndicator, Platform, Pressable, ScrollView, Text, View } from "react-native";
import { useSharedValue } from "react-native-reanimated";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";
import { SafeAreaView } from "../../../components/SafeArea";

import { ActionBar } from "../../../components/ActionBar";
import { ContextSheet } from "../../../components/ContextSheet";
import { PagesList } from "../../../components/PagesList";
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
import { UndoSnackbar } from "../../../components/UndoSnackbar";
import {
  DECISION_KEY,
  dismissNotice,
  hidesCard,
  holdDecision,
  lastDecision,
  NOTICES_KEY,
  planUndo,
  undoDecision,
  useDecisionLog,
  WITHDRAW_KEY,
  type DecisionKind,
  type DecisionNotice,
  type LoggedDecision,
  type WithdrawVars,
} from "../../../lib/decisions";
import {
  collectionFromParam,
  collectionParam,
  collectionsKey,
  nextCollection,
} from "../../../lib/deckCollections";
import { cardId, deckView, mergePages } from "../../../lib/deckView";
import { opensOnPages } from "../../../lib/pages";
import { whereParts } from "../../../lib/where";

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
  const params = useLocalSearchParams<{
    ws: string;
    project: string;
    locale: string;
    collection?: string;
    name?: string;
    sample?: string;
  }>();
  const { ws, project, locale } = params;
  // The review plan opens the deck on the interface sample under the
  // involvement Context: one page in ten per channel, and every page with a
  // failing finding.
  const sample = params.sample === "true";
  const collection = collectionFromParam(params.collection);
  const collectionName =
    collection === undefined ? undefined : params.name || t("collections.none");
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

  // The deck by page, which the screen opens on when the server has it.
  const pages = useQuery({
    queryKey: ["deck-pages", ws, project, locale, collection ?? null, sample],
    queryFn: () => client.deckPages(scope, locale, { collection, sample }),
  });
  const refetchPages = pages.refetch;
  useFocusEffect(
    useCallback(() => {
      void refetchPages();
    }, [refetchPages]),
  );
  const [chosenView, setView] = useState<"pages" | "cards" | null>(null);
  const hasPages = opensOnPages(pages.data);
  const mode = chosenView ?? (hasPages ? "pages" : "cards");
  const openPage = (p: {
    firstBlockId: string;
    kind: string;
    title: string;
    collectionId: string;
  }) =>
    router.push({
      pathname: "/[ws]/[project]/page",
      params: {
        ws,
        project,
        locale,
        block: p.firstBlockId,
        kind: p.kind,
        title: p.title,
        collection: p.collectionId,
      },
    });

  const deck = useInfiniteQuery({
    queryKey: ["deck", ws, project, locale, collection ?? null],
    queryFn: ({ pageParam }) =>
      client.deck(scope, locale, { limit: PAGE, cursor: pageParam, collection }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (last) => last.nextCursor,
  });

  // The decisions made on this device in this deck: the cards they keep out
  // of it, the progress of this visit, and the one Undo takes back.
  const log = useDecisionLog();
  const [since] = useState(Date.now);
  const inDeck = useCallback(
    (e: LoggedDecision) =>
      e.vars.scope.workspace === ws &&
      e.vars.scope.project === project &&
      e.vars.card.locale === locale &&
      (collection === undefined || e.vars.card.collectionId === collection),
    [ws, project, locale, collection],
  );
  const decided = useMemo(
    () => log.filter((e) => inDeck(e) && hidesCard(e)).map((e) => cardId(e.vars.card)),
    [log, inDeck],
  );
  const reviewed = useMemo(
    () => log.filter((e) => inDeck(e) && e.vars.madeAt >= since && hidesCard(e)),
    [log, inDeck, since],
  );
  const last = lastDecision(log, inDeck);
  const canUndo = last !== null && planUndo(qc, last) !== null;
  // The decisions that stand here: an undone one leaves the count.
  const madeHere = log.filter(
    (e) =>
      e.phase !== "undone" &&
      e.phase !== "undoing" &&
      e.vars.scope.workspace === ws &&
      e.vars.scope.project === project &&
      e.vars.card.locale === locale,
  ).length;
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

  // The language's collections: where each card's text ships, and the next
  // collection when this one runs out.
  const collections = useQuery({
    queryKey: collectionsKey(ws, project, locale),
    queryFn: () => client.deckCollections(scope, locale),
  });
  const whereOf = useCallback(
    (c: DeckCard) =>
      whereParts(collections.data?.collections.find((x) => x.collection_id === c.collectionId)),
    [collections.data],
  );
  const [details, setDetails] = useState<DeckCard | null>(null);
  // The blocks decided here, so the context page marks them.
  const decidedBlocks = useMemo(
    () =>
      new Set(
        log
          .filter(
            (e) =>
              e.vars.card.locale === locale &&
              e.vars.scope.workspace === ws &&
              e.vars.scope.project === project &&
              hidesCard(e),
          )
          .map((e) => e.vars.card.blockId),
      ),
    [log, locale, ws, project],
  );
  const finished = !deck.isPending && !top;
  const refetchCollections = collections.refetch;
  useEffect(() => {
    if (finished && collection !== undefined) void refetchCollections();
  }, [finished, collection, refetchCollections]);
  const next = finished ? nextCollection(collections.data?.collections, collection) : null;
  const openNext = (c: DeckCollection) => {
    const name = c.collection_id === "" ? t("collections.none") : c.name;
    router.replace({
      pathname: "/[ws]/[project]/[locale]",
      params: { ws, project, locale, collection: collectionParam(c.collection_id), name },
    });
  };

  const withdraw = useMutation<unknown, unknown, WithdrawVars>({ mutationKey: WITHDRAW_KEY });

  const record = useCallback(
    (
      card: DeckCard,
      kind: DecisionKind,
      extra: { runs?: Run[]; reasons?: FixReason[]; note?: string } = {},
    ) => {
      holdDecision(qc, {
        id: Crypto.randomUUID(),
        scope,
        card,
        kind,
        madeAt: Date.now(),
        ...extra,
      });
      if (kind === "approve")
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      else void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    },
    [qc, scope],
  );

  const swipeRef = useRef<SwipeCardHandle>(null);

  const onAction = useCallback(
    (action: SwipeAction) => {
      if (!top) return;
      setEntering(null);
      if (action === "approve") record(top, "approve");
      else if (action === "context") setDetails(top);
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
    if (!last) return;
    const card = undoDecision(qc, last.vars.id, (v) => withdraw.mutate(v));
    if (!card) return;
    setEntering({ id: cardId(card), from: last.vars.kind === "reject" ? "left" : "right" });
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const closeSheet = () => {
    if (fixing?.flown) swipeRef.current?.restore("left");
    setFixing(null);
  };

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
          ...(collectionName ? { title: collectionName } : {}),
          headerRight: () =>
            madeHere > 0 ? (
              <Pressable
                testID="deck-reviewed"
                accessibilityRole="button"
                accessibilityHint={t("reviewed.openHint")}
                hitSlop={8}
                onPress={() =>
                  router.push({
                    pathname: "/[ws]/[project]/reviewed",
                    params: { ws, project, locale },
                  })
                }
              >
                <Text className="text-primary text-base">
                  {t("reviewed.open", { count: madeHere })}
                </Text>
              </Pressable>
            ) : null,
          headerStyle: { backgroundColor: String(pageBackground) },
          headerTintColor: String(pageForeground),
          headerShadowVisible: false,
        }}
      />
      {hasPages || chosenView === "pages" ? (
        <View className="px-5 pt-2">
          <ViewSwitch value={mode} onChange={setView} />
        </View>
      ) : null}
      {mode === "pages" && pages.data ? (
        <View className="flex-1 px-5 pt-3">
          {sample ? (
            <Text testID="deck-sample" className="text-muted-foreground pb-3 text-sm leading-5">
              {t("deck.sample")}
            </Text>
          ) : null}
          <PagesList pages={pages.data} onOpen={openPage} />
        </View>
      ) : null}
      <View className={mode === "pages" && pages.data ? "hidden" : "flex-1"}>
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
          <View
            className="bg-muted h-1 overflow-hidden rounded-full"
            importantForAccessibility="no"
          >
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
                    <ReviewCard card={c} sourceLanguage={sourceLanguage} where={whereOf(c)} />
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
                <ReviewCard
                  card={top}
                  sourceLanguage={sourceLanguage}
                  where={whereOf(top)}
                  onDetails={() => setDetails(top)}
                />
              </SwipeCard>
            </View>
          ) : (
            <EndState
              reviewed={reviewed.length}
              desk={view.desk}
              canUndo={canUndo}
              onUndo={onUndo}
              collectionName={collectionName}
              next={next}
              onNext={openNext}
            />
          )}
        </View>
      </View>

      {last?.phase === "held" ? (
        <UndoSnackbar entry={last} online={online} onUndo={onUndo} />
      ) : null}

      {top && mode === "cards" ? (
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

      <ContextSheet
        card={details}
        scope={scope}
        where={details ? whereOf(details) : []}
        load={(card) => client.reviewContext(scope, card.blockId, card.locale)}
        onClose={() => setDetails(null)}
        inContext={{
          loadContainer: (card) => client.reviewContainer(scope, card.blockId, card.locale),
          onApprove: (card) => record(card, "approve"),
          // The fix sheet opens over the deck, so the context sheet steps aside.
          onFix: (card) => {
            setDetails(null);
            setFixing({ card, mode: "fix", flown: false });
          },
          decided: decidedBlocks,
          loadIndex: (collectionId) => client.storyIndex(scope, collectionId),
          loadConcept: (conceptId) => client.concept(scope.workspace, conceptId),
        }}
      />

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

/** Pages or Cards: how the language's deck is laid out. */
function ViewSwitch({
  value,
  onChange,
}: {
  value: "pages" | "cards";
  onChange: (v: "pages" | "cards") => void;
}) {
  const { t } = useTranslation();
  const options = ["pages", "cards"] as const;
  return (
    <View accessibilityRole="tablist" className="bg-muted flex-row self-start rounded-lg p-0.5">
      {options.map((o) => (
        <Pressable
          key={o}
          testID={`deck-view-${o}`}
          accessibilityRole="tab"
          accessibilityState={{ selected: value === o }}
          onPress={() => onChange(o)}
          className={`rounded-md px-3 py-1 ${value === o ? "bg-background" : ""}`}
        >
          <Text
            className={`text-sm ${value === o ? "text-foreground font-semibold" : "text-muted-foreground"}`}
          >
            {t(`pages.view.${o}`)}
          </Text>
        </Pressable>
      ))}
    </View>
  );
}

/** The deck after its last card: what was done here, then the desk cards. */
function EndState({
  reviewed,
  desk,
  canUndo,
  onUndo,
  collectionName,
  next,
  onNext,
}: {
  reviewed: number;
  desk: DeckCard[];
  canUndo: boolean;
  onUndo: () => void;
  /** The collection this deck holds, when it holds one. */
  collectionName?: string;
  /** The next collection with anything waiting. */
  next: DeckCollection | null;
  onNext: (c: DeckCollection) => void;
}) {
  const { t } = useTranslation();
  let body: string;
  if (collectionName !== undefined) {
    body =
      reviewed > 0
        ? t("collections.doneBody", { count: reviewed, name: collectionName })
        : t("collections.doneEmpty", { name: collectionName });
  } else {
    body = reviewed > 0 ? t("deck.doneBody", { count: reviewed }) : t("deck.doneEmpty");
  }
  const nextName = next ? next.name || t("collections.none") : "";
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
        <Text className="text-muted-foreground px-6 text-center text-base">{body}</Text>
        {next ? (
          <Pressable
            testID="deck-next-collection"
            accessibilityRole="button"
            onPress={() => onNext(next)}
            className="bg-primary items-center gap-0.5 rounded-full px-6 py-3 active:opacity-80"
          >
            <Text className="text-primary-foreground text-base font-semibold" numberOfLines={1}>
              {t("collections.next", { name: nextName })}
            </Text>
            <Text className="text-primary-foreground/80 text-xs tabular-nums">
              {t("collections.nextWaiting", { count: next.waiting })}
            </Text>
          </Pressable>
        ) : null}
        {collectionName !== undefined ? (
          <Pressable
            testID="deck-back-collections"
            accessibilityRole="button"
            onPress={() => router.back()}
            hitSlop={12}
          >
            <Text className="text-primary text-sm font-semibold">{t("collections.back")}</Text>
          </Pressable>
        ) : null}
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
