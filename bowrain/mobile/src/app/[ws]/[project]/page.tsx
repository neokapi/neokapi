// One page of a language's deck: a component's strings, or a document walked
// section by section. The page opens at its first waiting translation's
// container; Next section moves to the container of the next translation
// waiting in the document. Decisions go through the same queue as the deck's,
// so Undo, offline sending and Reviewed treat them alike.
import type { DeckCard, FixReason, ProjectScope, ReviewContainer } from "@neokapi/bowrain-client";
import type { Run } from "@neokapi/kapi-format";
import { onlineManager, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Crypto from "expo-crypto";
import * as Haptics from "expo-haptics";
import { router, Stack, useLocalSearchParams } from "expo-router";
import { useCallback, useMemo, useState, useSyncExternalStore } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { FixSheet } from "../../../components/FixSheet";
import { InContextView, reviewContainerKey } from "../../../components/InContextView";
import { SafeAreaView } from "../../../components/SafeArea";
import { UndoSnackbar } from "../../../components/UndoSnackbar";
import { useAuth } from "../../../lib/auth";
import {
  hidesCard,
  holdDecision,
  lastDecision,
  planUndo,
  undoDecision,
  useDecisionLog,
  WITHDRAW_KEY,
  type DecisionKind,
  type WithdrawVars,
} from "../../../lib/decisions";
import { pageCard } from "../../../lib/pages";

export default function PageScreen() {
  const { t } = useTranslation();
  const { ws, project, locale, block, kind, title, collection } = useLocalSearchParams<{
    ws: string;
    project: string;
    locale: string;
    block: string;
    kind: "component" | "document";
    title: string;
    collection?: string;
  }>();
  const { client } = useAuth();
  const qc = useQueryClient();
  const online = useSyncExternalStore(
    (cb) => onlineManager.subscribe(cb),
    () => onlineManager.isOnline(),
  );
  const scope: ProjectScope = useMemo(() => ({ workspace: ws, project }), [ws, project]);
  const [current, setCurrent] = useState(block);
  const card = useMemo(
    () => pageCard(current, locale, collection ?? ""),
    [current, locale, collection],
  );
  const load = useCallback(
    (c: DeckCard) => client.reviewContainer(scope, c.blockId, c.locale),
    [client, scope],
  );
  // The same query InContextView reads, for where the document goes next.
  const container = useQuery<ReviewContainer>({
    queryKey: reviewContainerKey(scope, current, locale),
    queryFn: () => load(card),
    staleTime: 60_000,
  });

  const log = useDecisionLog();
  const inPage = useCallback(
    (e: (typeof log)[number]) =>
      e.vars.scope.workspace === ws &&
      e.vars.scope.project === project &&
      e.vars.card.locale === locale,
    [ws, project, locale],
  );
  const decided = useMemo(
    () => new Set(log.filter((e) => inPage(e) && hidesCard(e)).map((e) => e.vars.card.blockId)),
    [log, inPage],
  );
  const last = lastDecision(log, inPage);
  const withdraw = useMutation<unknown, unknown, WithdrawVars>({ mutationKey: WITHDRAW_KEY });
  const onUndo = () => {
    if (!last || planUndo(qc, last) === null) return;
    undoDecision(qc, last.vars.id, (v) => withdraw.mutate(v));
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  const record = useCallback(
    (
      c: DeckCard,
      k: DecisionKind,
      extra: { runs?: Run[]; reasons?: FixReason[]; note?: string } = {},
    ) => {
      holdDecision(qc, {
        id: Crypto.randomUUID(),
        scope,
        card: c,
        kind: k,
        madeAt: Date.now(),
        ...extra,
      });
      if (k === "approve") void Haptics.notificationAsync(Haptics.NotificationFeedbackType.Success);
      else void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    },
    [qc, scope],
  );
  const [fixing, setFixing] = useState<DeckCard | null>(null);

  const next = kind === "document" ? container.data?.nextWaiting : undefined;
  const footer = container.data ? (
    <View className="gap-3 pt-6">
      {next ? (
        <Pressable
          testID="page-next"
          accessibilityRole="button"
          onPress={() => setCurrent(next)}
          className="bg-primary items-center rounded-full px-6 py-3 active:opacity-80"
        >
          <Text className="text-primary-foreground text-base font-semibold">{t("pages.next")}</Text>
        </Pressable>
      ) : kind === "document" ? (
        <Text className="text-muted-foreground text-center text-sm">{t("pages.finished")}</Text>
      ) : null}
      <Pressable
        testID="page-done"
        accessibilityRole="button"
        onPress={() => router.back()}
        hitSlop={12}
        className="items-center py-2"
      >
        <Text className="text-primary text-sm font-semibold">{t("pages.done")}</Text>
      </Pressable>
    </View>
  ) : null;

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <Stack.Screen options={title ? { title } : {}} />
      <InContextView
        key={current}
        card={card}
        scope={scope}
        load={load}
        decided={decided}
        onApprove={(c) => record(c, "approve")}
        onFix={(c) => setFixing(c)}
        footer={footer}
        loadIndex={(collectionId) => client.storyIndex(scope, collectionId)}
        loadConcept={(conceptId) => client.concept(scope.workspace, conceptId)}
      />
      {last?.phase === "held" ? (
        <UndoSnackbar entry={last} online={online} onUndo={onUndo} />
      ) : null}
      <FixSheet
        card={fixing}
        mode="fix"
        loadFixes={(c, reason, ai) => client.fixCandidates(scope, c, { reason, ai })}
        onClose={() => setFixing(null)}
        onFix={(c, runs, reasons) => {
          record(c, "fix", { runs, reasons });
          setFixing(null);
        }}
        onReject={(c, reasons, note) => {
          record(c, "reject", { reasons, note: note || undefined });
          setFixing(null);
        }}
      />
    </SafeAreaView>
  );
}
