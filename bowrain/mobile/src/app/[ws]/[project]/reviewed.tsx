// The decisions made on this device in a project, newest first, with where
// each one is and Undo on every one that can still be taken back. Opened
// from the deck, it lists that language; a held or queued decision is
// dropped, a sent one is withdrawn with a new decision.
import { onlineManager, useMutation, useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { useLocalSearchParams } from "expo-router";
import { useSyncExternalStore } from "react";
import { FlatList, Text } from "react-native";
import { useTranslation } from "react-i18next";

import { DecisionRow } from "../../../components/DecisionRow";
import { SafeAreaView } from "../../../components/SafeArea";
import {
  planUndo,
  undoDecision,
  useDecisionLog,
  WITHDRAW_KEY,
  type LoggedDecision,
  type WithdrawVars,
} from "../../../lib/decisions";
import { ageLabel, languageLabel } from "../../../lib/feed";

/** Where a decision is, in the reader's words. */
export function statusKey(e: LoggedDecision, online: boolean): string {
  switch (e.phase) {
    case "held":
      return "reviewed.status.held";
    case "queued":
      return online ? "reviewed.status.sending" : "reviewed.status.offline";
    case "undoing":
      return "reviewed.status.undoing";
    case "undone":
      return "reviewed.status.undone";
    case "done":
      switch (e.result?.outcome) {
        case "held":
          return "reviewed.status.secondReview";
        case "stale":
          return "reviewed.status.stale";
        case "refused":
          return "reviewed.status.refused";
        default:
          return e.vars.kind === "fix" ? "reviewed.status.fixLanded" : "reviewed.status.sent";
      }
  }
}

function useOnline(): boolean {
  return useSyncExternalStore(
    (cb) => onlineManager.subscribe(cb),
    () => onlineManager.isOnline(),
  );
}

export default function ReviewedScreen() {
  const { t, i18n } = useTranslation();
  const { ws, project, locale } = useLocalSearchParams<{
    ws: string;
    project: string;
    locale?: string;
  }>();
  const qc = useQueryClient();
  const online = useOnline();
  const log = useDecisionLog();
  const withdraw = useMutation<unknown, unknown, WithdrawVars>({ mutationKey: WITHDRAW_KEY });
  const entries = log
    .filter(
      (e) =>
        e.vars.scope.workspace === ws &&
        e.vars.scope.project === project &&
        (!locale || e.vars.card.locale === locale),
    )
    .slice()
    .reverse();

  const undo = (id: string) => {
    if (undoDecision(qc, id, (v) => withdraw.mutate(v))) {
      void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
    }
  };

  return (
    <SafeAreaView className="bg-background flex-1" edges={["bottom", "left", "right"]}>
      <FlatList
        data={entries}
        keyExtractor={(e) => e.vars.id}
        contentContainerClassName="gap-3 px-4 py-4"
        contentInsetAdjustmentBehavior="automatic"
        ListHeaderComponent={
          locale ? (
            <Text className="text-muted-foreground pb-1 text-sm">
              {t("reviewed.lead", { language: languageLabel(locale, i18n.language) })}
            </Text>
          ) : undefined
        }
        ListEmptyComponent={
          <Text testID="reviewed-empty" className="text-muted-foreground py-10 text-center">
            {t("reviewed.empty")}
          </Text>
        }
        renderItem={({ item }) => (
          <DecisionRow
            testID={`reviewed-${item.vars.id}`}
            kind={item.vars.kind}
            status={`${t(statusKey(item, online))} · ${ageLabel(new Date(item.vars.madeAt).toISOString(), t, i18n.language)}`}
            target={
              item.vars.kind === "fix" && item.vars.runs ? item.vars.runs : item.vars.card.target
            }
            source={item.vars.card.source}
            dimmed={item.phase === "undone"}
            canUndo={planUndo(qc, item) !== null}
            onUndo={() => undo(item.vars.id)}
          />
        )}
      />
    </SafeAreaView>
  );
}
