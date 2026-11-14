// A round button for every gesture, laid out the way the card moves: Reject on
// the left, Approve on the right, Fix between them, with Undo and Later
// smaller at the edges. A card the person wrote offers no Approve: the server
// refuses that approval under a separation-of-duties policy.
import Ionicons from "@expo/vector-icons/Ionicons";
import type { ComponentProps } from "react";
import { Pressable, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import type { SwipeAction } from "./SwipeCard";

type IconName = ComponentProps<typeof Ionicons>["name"];

const LIFT = "0px 6px 16px rgba(15, 23, 42, 0.12), 0px 1px 3px rgba(15, 23, 42, 0.08)";

function RoundButton({
  testID,
  icon,
  color,
  size,
  label,
  hint,
  disabled,
  onPress,
}: {
  testID: string;
  icon: IconName;
  color: string;
  size: "sm" | "md" | "lg";
  label: string;
  hint?: string;
  disabled?: boolean;
  onPress: () => void;
}) {
  const box = size === "lg" ? "h-18 w-18" : size === "md" ? "h-15 w-15" : "h-12 w-12";
  const glyph = size === "lg" ? 36 : size === "md" ? 28 : 22;
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      accessibilityLabel={label}
      accessibilityHint={hint}
      accessibilityState={{ disabled: !!disabled }}
      disabled={disabled}
      onPress={onPress}
      hitSlop={6}
      className={`bg-card border-border items-center justify-center rounded-full border active:scale-95 active:opacity-80 ${box} ${disabled ? "opacity-35" : ""}`}
      style={disabled ? undefined : { boxShadow: LIFT }}
    >
      <Ionicons name={icon} size={glyph} color={color} />
    </Pressable>
  );
}

export function ActionBar({
  canApprove,
  canUndo,
  onAction,
  onUndo,
}: {
  canApprove: boolean;
  canUndo: boolean;
  onAction: (action: SwipeAction) => void;
  onUndo: () => void;
}) {
  const { t } = useTranslation();
  const [approve, reject, corrective, muted] = useCSSVariable([
    "--color-approve",
    "--color-reject",
    "--color-corrective",
    "--color-muted-foreground",
  ]);
  return (
    <View className="flex-row items-center justify-center gap-4 px-4 pt-3 pb-2">
      <RoundButton
        testID="undo"
        icon="arrow-undo"
        color={String(muted)}
        size="sm"
        label={t("undo.button")}
        hint={t("undo.hint")}
        disabled={!canUndo}
        onPress={onUndo}
      />
      <RoundButton
        testID="action-reject"
        icon="close"
        color={String(reject)}
        size="lg"
        label={t("actions.reject")}
        hint={t("actions.rejectHint")}
        onPress={() => onAction("reject")}
      />
      <RoundButton
        testID="action-fix"
        icon="create-outline"
        color={String(corrective)}
        size="md"
        label={t("actions.fix")}
        hint={t("actions.fixHint")}
        onPress={() => onAction("fix")}
      />
      <RoundButton
        testID="action-approve"
        icon="checkmark"
        color={String(approve)}
        size="lg"
        label={t("actions.approve")}
        hint={canApprove ? t("actions.approveHint") : t("actions.approveOwnWork")}
        disabled={!canApprove}
        onPress={() => onAction("approve")}
      />
      <RoundButton
        testID="action-later"
        icon="arrow-up"
        color={String(muted)}
        size="sm"
        label={t("actions.later")}
        hint={t("actions.laterHint")}
        onPress={() => onAction("later")}
      />
    </View>
  );
}
