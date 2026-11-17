// The decisions made on this device in a project, newest first, with where
// each one is and Undo on every one that can still be taken back. Opened
// from the deck, it lists that language; a held or queued decision is
// dropped, a sent one is withdrawn with a new decision.
import Ionicons from "@expo/vector-icons/Ionicons";
import { onlineManager, useMutation, useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { useLocalSearchParams } from "expo-router";
import type { ComponentProps } from "react";
import { useSyncExternalStore } from "react";
import { FlatList, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { RunText } from "../../../components/RunText";
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

type IconName = ComponentProps<typeof Ionicons>["name"];

const ICONS: Record<LoggedDecision["vars"]["kind"], IconName> = {
  approve: "checkmark-circle",
  reject: "close-circle",
  fix: "create",
};

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
            entry={item}
            online={online}
            canUndo={planUndo(qc, item) !== null}
            onUndo={() => undo(item.vars.id)}
            age={ageLabel(new Date(item.vars.madeAt).toISOString(), t, i18n.language)}
          />
        )}
      />
    </SafeAreaView>
  );
}

function DecisionRow({
  entry,
  online,
  canUndo,
  onUndo,
  age,
}: {
  entry: LoggedDecision;
  online: boolean;
  canUndo: boolean;
  onUndo: () => void;
  age: string;
}) {
  const { t } = useTranslation();
  const { kind, card, runs } = entry.vars;
  const [approve, reject, corrective, muted] = useCSSVariable([
    "--color-approve",
    "--color-reject",
    "--color-corrective",
    "--color-muted-foreground",
  ]);
  const undone = entry.phase === "undone";
  const color = undone
    ? muted
    : kind === "approve"
      ? approve
      : kind === "reject"
        ? reject
        : corrective;
  return (
    <View
      testID={`reviewed-${entry.vars.id}`}
      className={`bg-card border-border gap-2 rounded-xl border p-3 ${undone ? "opacity-60" : ""}`}
    >
      <View className="flex-row items-center gap-2">
        <Ionicons name={ICONS[kind]} size={18} color={String(color)} />
        <Text className="text-foreground text-sm font-semibold">{t(`undo.decided.${kind}`)}</Text>
        <Text className="text-muted-foreground flex-1 text-xs" numberOfLines={1}>
          {t(statusKey(entry, online))} · {age}
        </Text>
        {canUndo ? (
          <Pressable
            testID={`reviewed-undo-${entry.vars.id}`}
            accessibilityRole="button"
            accessibilityHint={t("reviewed.undoHint")}
            onPress={onUndo}
            hitSlop={8}
            className="border-border flex-row items-center gap-1 rounded-full border px-3 py-1 active:opacity-70"
          >
            <Ionicons name="arrow-undo" size={14} color={String(muted)} />
            <Text className="text-foreground text-xs font-semibold">{t("undo.button")}</Text>
          </Pressable>
        ) : null}
      </View>
      <RunText runs={kind === "fix" && runs ? runs : card.target} />
      <RunText runs={card.source} muted />
    </View>
  );
}
