// The decisions a review entry of the feed counts, each beside the
// translation as it stands now, with Undo on the reader's own decisions that
// still hold. Undo withdraws the decision with a new one on the revision the
// server listed, through the same queue as the deck's decisions.
import type { FeedDecision } from "@neokapi/bowrain-client";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import * as Haptics from "expo-haptics";
import { useState } from "react";
import { ActivityIndicator, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import { useAuth } from "../../lib/auth";
import {
  feedDecisionsKey,
  lastDecision,
  readLog,
  WITHDRAW_KEY,
  type WithdrawVars,
} from "../../lib/decisions";
import { ageLabel } from "../../lib/feed";
import { DecisionRow, type DecisionRowKind } from "../DecisionRow";

const KINDS: Record<string, DecisionRowKind> = {
  approved: "approve",
  rejected: "reject",
  unreviewed: "withdraw",
};

export function ReviewDecisions({
  ws,
  project,
  entry,
}: {
  ws: string;
  project: string;
  entry: string;
}) {
  const { t, i18n } = useTranslation();
  const { client } = useAuth();
  const qc = useQueryClient();
  const decisions = useQuery({
    queryKey: feedDecisionsKey(ws, project, entry),
    queryFn: () => client.feedDecisions(ws, project, entry),
  });
  const withdraw = useMutation<unknown, unknown, WithdrawVars>({ mutationKey: WITHDRAW_KEY });
  const [undoing, setUndoing] = useState<string[]>([]);

  const undo = (d: FeedDecision) => {
    // The same decision in this device's log, when it was made here, so the
    // Reviewed list shows it taken back too.
    const mine = lastDecision(
      readLog(qc),
      (e) =>
        e.phase === "done" &&
        e.vars.card.blockId === d.card.blockId &&
        e.vars.card.locale === d.card.locale,
    );
    const id = mine?.vars.id ?? `feed:${entry}:${d.card.blockId}`;
    setUndoing((u) => [...u, d.card.blockId]);
    withdraw.mutate(
      {
        id,
        scope: { workspace: ws, project },
        card: d.card,
        revision: d.card.targetRevision,
      },
      { onSettled: () => setUndoing((u) => u.filter((b) => b !== d.card.blockId)) },
    );
    void Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Light);
  };

  if (decisions.isPending) return <ActivityIndicator className="py-6" />;
  if (decisions.isError) {
    return (
      <Text className="text-muted-foreground px-4 py-4 text-sm">{t("common.loadFailed")}</Text>
    );
  }
  const { decisions: list, bulkApproved } = decisions.data;
  return (
    <View testID="feed-decisions" className="gap-3 px-4 pt-4">
      <Text accessibilityRole="header" className="text-foreground text-base font-semibold">
        {t("feed.decisions.title", { count: list.length })}
      </Text>
      {list.map((d) => (
        <DecisionRow
          key={`${d.card.blockId}:${d.at}`}
          testID={`feed-decision-${d.card.blockId}`}
          kind={KINDS[d.decision] ?? "withdraw"}
          status={`${d.current ? "" : `${t("feed.decisions.changed")} · `}${ageLabel(d.at, t, i18n.language)}`}
          target={d.card.target}
          source={d.card.source}
          dimmed={!d.current}
          canUndo={d.canUndo}
          undoing={undoing.includes(d.card.blockId)}
          onUndo={() => undo(d)}
        />
      ))}
      {bulkApproved > 0 ? (
        <Text className="text-muted-foreground text-sm">
          {t("feed.decisions.bulk", { count: bulkApproved })}
        </Text>
      ) : null}
    </View>
  );
}
