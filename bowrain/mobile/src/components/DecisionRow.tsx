// One review decision in a list: what was decided, where it stands, the
// translation and its source, and Undo when it can still be taken back. The
// Reviewed screen lists the device's decisions with it, and a review entry of
// the feed lists the server's.
import Ionicons from "@expo/vector-icons/Ionicons";
import type { Run } from "@neokapi/kapi-format";
import type { ComponentProps } from "react";
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { RunText } from "./RunText";

type IconName = ComponentProps<typeof Ionicons>["name"];

/** What a row records: a decision made, or the withdrawal of one. */
export type DecisionRowKind = "approve" | "reject" | "fix" | "withdraw";

const ICONS: Record<DecisionRowKind, IconName> = {
  approve: "checkmark-circle",
  reject: "close-circle",
  fix: "create",
  withdraw: "arrow-undo-circle",
};

export function DecisionRow({
  testID,
  kind,
  status,
  target,
  source,
  dimmed,
  canUndo,
  undoing,
  onUndo,
}: {
  testID: string;
  kind: DecisionRowKind;
  /** Where the decision stands, and when it was made. */
  status: string;
  target: readonly Run[];
  source: readonly Run[];
  /** A decision that no longer holds. */
  dimmed?: boolean;
  canUndo: boolean;
  /** Undo was tapped and the withdrawal is on its way. */
  undoing?: boolean;
  onUndo: () => void;
}) {
  const { t } = useTranslation();
  const [approve, reject, corrective, muted] = useCSSVariable([
    "--color-approve",
    "--color-reject",
    "--color-corrective",
    "--color-muted-foreground",
  ]);
  const color =
    dimmed || kind === "withdraw"
      ? muted
      : kind === "approve"
        ? approve
        : kind === "reject"
          ? reject
          : corrective;
  return (
    <View
      testID={testID}
      className={`bg-card border-border gap-2 rounded-xl border p-3 ${dimmed ? "opacity-60" : ""}`}
    >
      <View className="flex-row items-center gap-2">
        <Ionicons name={ICONS[kind]} size={18} color={String(color)} />
        <Text className="text-foreground text-sm font-semibold">{t(`undo.decided.${kind}`)}</Text>
        <Text className="text-muted-foreground flex-1 text-xs" numberOfLines={1}>
          {status}
        </Text>
        {undoing ? (
          <Text className="text-muted-foreground text-xs">{t("reviewed.status.undoing")}</Text>
        ) : canUndo ? (
          <Pressable
            testID={`${testID}-undo`}
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
      <RunText runs={target} />
      <RunText runs={source} muted />
    </View>
  );
}
