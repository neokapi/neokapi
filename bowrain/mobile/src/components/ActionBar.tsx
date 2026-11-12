// A button for every gesture. The corrective action comes first and Approve
// last, at the trailing edge, as on every review surface.
import { Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

import type { SwipeAction } from "./SwipeCard";

export function ActionBar({
  canApprove,
  onAction,
}: {
  canApprove: boolean;
  onAction: (action: SwipeAction) => void;
}) {
  const { t } = useTranslation();
  return (
    <View className="flex-row items-center gap-3 px-4 pb-2">
      <Pressable
        testID="action-fix"
        accessibilityRole="button"
        accessibilityHint={t("actions.fixHint")}
        onPress={() => onAction("fix")}
        className="bg-corrective flex-1 items-center rounded-2xl py-4 active:opacity-80"
      >
        <Text className="text-corrective-foreground text-base font-semibold">{t("actions.fix")}</Text>
      </Pressable>
      <Pressable
        testID="action-later"
        accessibilityRole="button"
        accessibilityHint={t("actions.laterHint")}
        onPress={() => onAction("later")}
        className="bg-muted items-center rounded-2xl px-5 py-4 active:opacity-80"
      >
        <Text className="text-foreground text-base font-medium">{t("actions.later")}</Text>
      </Pressable>
      <Pressable
        testID="action-approve"
        accessibilityRole="button"
        accessibilityState={{ disabled: !canApprove }}
        accessibilityHint={t("actions.approveHint")}
        disabled={!canApprove}
        onPress={() => onAction("approve")}
        className={`bg-primary ml-3 flex-1 items-center rounded-2xl py-4 active:opacity-80 ${canApprove ? "" : "opacity-40"}`}
      >
        <Text className="text-primary-foreground text-base font-semibold">{t("actions.approve")}</Text>
      </Pressable>
    </View>
  );
}
