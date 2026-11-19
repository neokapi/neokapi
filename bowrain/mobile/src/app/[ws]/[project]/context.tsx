// A context session over a project's context deck. The server deals up to
// ten cards from the project's one queue, widest reach first, and holds them
// for this person while the session runs, so nobody else decides the same
// cards. The session opens with what changed since the team's last session,
// then one card at a time: right keeps a card, left retires it after a
// confirmation, up puts it at the end. A proposed translation can also be
// edited, then kept in the reviewer's wording. When every card is decided or
// put off, the session ends and the summary says what it moved. Leaving the
// screen or sending the app to the background ends the session too, which
// releases its cards; the server's expiry covers a crash.
import type {
  ContextCard as Card,
  ContextDecision,
  ContextSession,
  ContextSessionEnd,
} from "@neokapi/bowrain-client";
import { useQueryClient } from "@tanstack/react-query";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as Haptics from "expo-haptics";
import { Stack, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ActivityIndicator, Alert, AppState, Pressable, Text, TextInput, View } from "react-native";
import { useSharedValue } from "react-native-reanimated";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { ActionBar } from "../../../components/ActionBar";
import { ContextCard } from "../../../components/ContextCard";
import { SinceCard, Summary } from "../../../components/ContextSession";
import { OriginSheet } from "../../../components/OriginSheet";
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
  contextCopyPrefix,
  contextDeckView,
  refusalCode,
  sessionThrough,
  sinceHasNews,
  widestReach,
} from "../../../lib/contextDeck";

const STACK = 2;

type Phase =
  | { kind: "starting" }
  | { kind: "session"; session: ContextSession }
  | { kind: "ending"; session: ContextSession }
  | { kind: "summary"; end: ContextSessionEnd }
  | { kind: "failed" };

export default function ContextSessionScreen() {
  const { t } = useTranslation();
  const router = useRouter();
  const { ws, project } = useLocalSearchParams<{ ws: string; project: string }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const [pageBackground, pageForeground] = useCSSVariable([
    "--color-background",
    "--color-foreground",
  ]);

  const [phase, setPhase] = useState<Phase>({ kind: "starting" });
  const [showSince, setShowSince] = useState(false);
  const [decided, setDecided] = useState<string[]>([]);
  const [later, setLater] = useState<string[]>([]);
  const [notice, setNotice] = useState<string | null>(null);
  const [originOf, setOriginOf] = useState<Card | null>(null);
  const drag = useSharedValue(0);
  const swipeRef = useRef<SwipeCardHandle>(null);

  // The session the server still holds cards for, and the decisions in
  // flight, which the session waits for before it ends.
  const live = useRef<string | null>(null);
  const inFlight = useRef(new Set<Promise<unknown>>());

  const endQuietly = useCallback(() => {
    const sid = live.current;
    if (!sid) return;
    live.current = null;
    void client.endContextSession(ws, project, sid).catch(() => {});
  }, [client, ws, project]);

  const start = useCallback(async () => {
    endQuietly();
    setPhase({ kind: "starting" });
    setDecided([]);
    setLater([]);
    setNotice(null);
    try {
      const session = await client.startContextSession(ws, project);
      if (session.cards.length > 0) live.current = session.session.id;
      else void client.endContextSession(ws, project, session.session.id).catch(() => {});
      setShowSince(sinceHasNews(session.since));
      setPhase({ kind: "session", session });
    } catch {
      setPhase({ kind: "failed" });
    }
  }, [client, ws, project, endQuietly]);

  // Start on open; end on leaving. Sending the app to the background ends the
  // session, and coming back starts a new one.
  const startRef = useRef(start);
  startRef.current = start;
  const endRef = useRef(endQuietly);
  endRef.current = endQuietly;
  useEffect(() => {
    void startRef.current();
    let resume = false;
    const sub = AppState.addEventListener("change", (state) => {
      if (state === "background" && live.current) {
        resume = true;
        endRef.current();
      } else if (state === "active" && resume) {
        resume = false;
        void startRef.current();
      }
    });
    return () => {
      sub.remove();
      endRef.current();
    };
  }, []);

  const session = phase.kind === "session" || phase.kind === "ending" ? phase.session : null;
  const cards = session?.cards ?? [];
  const view = useMemo(
    () => contextDeckView(cards, new Set(decided), later),
    [cards, decided, later],
  );
  const through =
    session !== null && cards.length > 0 && sessionThrough(cards, new Set(decided), later);
  const top = through ? undefined : view[0];
  const behind = through ? [] : view.slice(1, 1 + STACK);
  const widest = useMemo(() => widestReach(cards), [cards]);
  const added = useMemo(
    () => new Set((session?.since?.added ?? []).map((a) => `${a.item.kind}:${a.item.id}`)),
    [session],
  );

  // Once every card is decided or put off, the session ends and the summary
  // takes its place.
  useEffect(() => {
    if (!through || phase.kind !== "session") return;
    const current = phase.session;
    setPhase({ kind: "ending", session: current });
    void (async () => {
      await Promise.allSettled(inFlight.current);
      live.current = null;
      try {
        const end = await client.endContextSession(ws, project, current.session.id);
        setPhase({ kind: "summary", end });
      } catch {
        setPhase({ kind: "failed" });
      }
      void qc.invalidateQueries({ queryKey: ["review-inbox"] });
    })();
  }, [through, phase, client, ws, project, qc]);

  const send = useCallback(
    (d: ContextDecision) => {
      const p = client
        .decideContext(ws, project, d)
        .then((result) => {
          if (result.outcome === "proposed") setNotice(t("context.outcome.proposed"));
          void qc.invalidateQueries({ queryKey: ["review-inbox"] });
        })
        .catch((err: unknown) => {
          // The card counts as put off, with what the server said.
          const id = contextCardId(d);
          setDecided((ids) => ids.filter((x) => x !== id));
          setLater((ids) => [...ids.filter((x) => x !== id), id]);
          setNotice(
            t(`context.refused.${refusalCode(err)}`, { defaultValue: t("context.refused.failed") }),
          );
        })
        .finally(() => inFlight.current.delete(p));
      inFlight.current.add(p);
    },
    [client, ws, project, qc, t],
  );

  const record = useCallback(
    (card: Card, action: "keep" | "reject", text?: string) => {
      setDecided((ids) => [...ids, contextCardId(card)]);
      const wording = text && text !== card.translation?.proposed ? text : undefined;
      send({ item: card.item, revision: card.revision, action, text: wording });
      if (action === "keep")
        void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      else void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    },
    [send],
  );

  // A proposed term's actions say what they do to it: keep makes it a term
  // of the project, and retire records that it is not one. A proposed
  // translation's keep adds the term in its language.
  const prefix = contextCopyPrefix(top);
  const copy = useMemo(() => {
    return {
      keep: t(`${prefix}.keep`),
      keepHint: t(`${prefix}.keepHint`),
      retire: t(`${prefix}.retire`),
      retireHint: t(`${prefix}.retireHint`),
      retireTitle: t(`${prefix}.retireTitle`),
    };
  }, [prefix, t]);

  // The reviewer's own wording for a proposed translation, while they edit it.
  // It is held against the card it was written for, so the next card opens
  // with none.
  const [edit, setEdit] = useState<{ id: string; text: string } | null>(null);
  const topId = top ? contextCardId(top) : "";
  const wording = edit && edit.id === topId ? edit.text : null;
  const setWording = useCallback(
    (text: string | null) => setEdit(text === null ? null : { id: topId, text }),
    [topId],
  );

  const onAction = useCallback(
    (action: SwipeAction) => {
      if (!top) return;
      if (action === "approve") record(top, "keep");
      else if (action === "later") {
        const id = contextCardId(top);
        setLater((ids) => [...ids.filter((x) => x !== id), id]);
      } else if (action === "reject") {
        Alert.alert(copy.retireTitle, undefined, [
          {
            text: t("common.cancel"),
            style: "cancel",
            onPress: () => swipeRef.current?.restore("left"),
          },
          { text: copy.retire, style: "destructive", onPress: () => record(top, "reject") },
        ]);
      }
    },
    [top, record, t, copy],
  );

  const canKeep = top?.actions.includes("keep") ?? false;
  const canReject = top?.actions.includes("reject") ?? false;
  const handled = new Set([...decided, ...later]).size;
  const total = cards.length;
  const sinceCard = showSince && session?.since && !through ? session.since : null;
  const sessionCard = (c: (typeof cards)[number]) => ({
    widest,
    channels: c.channels,
    proposals: c.proposals,
    isNew: added.has(`${c.item.kind}:${c.item.id}`),
  });

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <Stack.Screen
        options={{
          headerStyle: { backgroundColor: String(pageBackground) },
          headerTintColor: String(pageForeground),
          headerShadowVisible: false,
        }}
      />
      {session && total > 0 ? (
        <View className="gap-2 px-5 pt-2 pb-1">
          <Text testID="context-progress" className="text-foreground text-sm font-semibold">
            {t("deck.progress", { position: Math.min(handled + 1, total), total })}
          </Text>
          <View
            className="bg-muted h-1 overflow-hidden rounded-full"
            importantForAccessibility="no"
          >
            <View
              className="from-prism-1 via-prism-3 to-prism-5 h-1 rounded-full bg-linear-to-r"
              style={{ width: `${(handled / total) * 100}%` }}
            />
          </View>
        </View>
      ) : null}

      {session && !session.bound ? (
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
        {phase.kind === "starting" || phase.kind === "ending" ? (
          <ActivityIndicator className="flex-1" />
        ) : phase.kind === "failed" ? (
          <View className="flex-1 items-center justify-center gap-3">
            <Text className="text-muted-foreground">{t("common.loadFailed")}</Text>
            <Pressable accessibilityRole="button" onPress={() => void start()}>
              <Text className="text-primary">{t("common.retry")}</Text>
            </Pressable>
          </View>
        ) : phase.kind === "summary" ? (
          <Summary end={phase.end} onMore={() => void start()} onStop={() => router.back()} />
        ) : sinceCard ? (
          <SinceCard since={sinceCard} onStart={() => setShowSince(false)} />
        ) : top ? (
          <View className="flex-1">
            {behind
              .map((c, i) => (
                <StackedCard key={contextCardId(c)} depth={i + 1} progress={drag}>
                  <ContextCard card={c} session={sessionCard(c)} />
                </StackedCard>
              ))
              .reverse()}
            <SwipeCard
              key={contextCardId(top)}
              ref={swipeRef}
              canApprove={canKeep}
              canReject={canReject}
              canFix={false}
              stamps={{ approve: copy.keep, reject: copy.retire }}
              onAction={onAction}
              progress={drag}
            >
              <ContextCard
                card={top}
                scope={{ ws, project }}
                session={sessionCard(top)}
                onOpenOrigin={() => {
                  void Haptics.selectionAsync();
                  setOriginOf(top);
                }}
              />
            </SwipeCard>
          </View>
        ) : (
          <Empty heldByOthers={session?.held_by_others ?? 0} />
        )}
      </View>

      <OriginSheet
        card={originOf}
        load={(op, cursor) => client.contextOrigin(ws, project, op, { cursor, limit: 50 })}
        onClose={() => setOriginOf(null)}
      />

      {top?.translation && canKeep && !sinceCard ? (
        <TranslationEdit
          wording={wording}
          proposed={top.translation.proposed}
          onEdit={setWording}
          onKeep={(text) => record(top, "keep", text)}
        />
      ) : null}

      {top && !sinceCard ? (
        <ActionBar
          canApprove={canKeep}
          canReject={canReject}
          showFix={false}
          canUndo={false}
          onUndo={() => {}}
          labels={{
            approve: copy.keep,
            approveHint: copy.keepHint,
            approveDenied: t("context.ownWork"),
            reject: copy.retire,
            rejectHint: copy.retireHint,
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

/**
 * Edit, then keep: the reviewer writes the term in their own words and keeps
 * that instead of the proposal.
 */
function TranslationEdit({
  wording,
  proposed,
  onEdit,
  onKeep,
}: {
  wording: string | null;
  proposed: string;
  onEdit: (text: string | null) => void;
  onKeep: (text: string) => void;
}) {
  const { t } = useTranslation();
  if (wording === null) {
    return (
      <Pressable
        testID="context-translation-edit"
        accessibilityRole="button"
        accessibilityHint={t("context.translation.editHint")}
        onPress={() => onEdit(proposed)}
        hitSlop={8}
        className="self-center px-4 pb-2 active:opacity-70"
      >
        <Text className="text-primary text-sm font-medium">{t("context.translation.edit")}</Text>
      </Pressable>
    );
  }
  const text = wording.trim();
  return (
    <View className="gap-2 px-5 pb-2">
      <TextInput
        testID="context-translation-input"
        accessibilityLabel={t("context.translation.editLabel")}
        value={wording}
        onChangeText={onEdit}
        autoFocus
        autoCapitalize="none"
        autoCorrect={false}
        className="bg-muted text-foreground rounded-xl px-3 py-2 text-base"
      />
      <View className="flex-row justify-end gap-4">
        <Pressable accessibilityRole="button" onPress={() => onEdit(null)} hitSlop={8}>
          <Text className="text-muted-foreground text-sm font-medium">{t("common.cancel")}</Text>
        </Pressable>
        <Pressable
          testID="context-translation-save"
          accessibilityRole="button"
          disabled={text === ""}
          onPress={() => onKeep(text)}
          hitSlop={8}
          className={text === "" ? "opacity-50" : "active:opacity-70"}
        >
          <Text className="text-primary text-sm font-semibold">
            {t("context.translation.save")}
          </Text>
        </Pressable>
      </View>
    </View>
  );
}

/** A session that dealt nothing: no card waits, or other people's sessions hold them. */
function Empty({ heldByOthers }: { heldByOthers: number }) {
  const { t } = useTranslation();
  const [approve] = useCSSVariable(["--color-approve"]);
  return (
    <View testID="context-done" className="flex-1 items-center justify-center gap-4">
      <Ionicons name="checkmark-circle" size={64} color={String(approve)} />
      <Text accessibilityRole="header" className="text-foreground text-2xl font-semibold">
        {t("context.doneTitle")}
      </Text>
      <Text className="text-muted-foreground text-center text-base">
        {heldByOthers > 0
          ? t("context.inSession.heldByOthers", { count: heldByOthers })
          : t("context.doneEmpty")}
      </Text>
    </View>
  );
}
