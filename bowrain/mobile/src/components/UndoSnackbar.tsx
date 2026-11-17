// The bar that follows a decision while it is still on the device: what was
// decided, and Undo. A thin line runs out with the grace period; when it is
// gone the decision is sent and the bar leaves. Offline it says the decision
// waits for a connection.
import { useEffect } from "react";
import { AccessibilityInfo, Pressable, Text, View } from "react-native";
import Animated, {
  Easing,
  FadeInDown,
  FadeOut,
  useAnimatedStyle,
  useSharedValue,
  withTiming,
} from "react-native-reanimated";
import { useTranslation } from "react-i18next";

import { GRACE_MS, type LoggedDecision } from "../lib/decisions";

export function UndoSnackbar({
  entry,
  online,
  onUndo,
}: {
  entry: LoggedDecision;
  online: boolean;
  onUndo: () => void;
}) {
  const { t } = useTranslation();
  const { id, kind, madeAt } = entry.vars;
  const label = t(`undo.decided.${kind}`);
  const left = useSharedValue(1);

  useEffect(() => {
    const remaining = Math.max(0, madeAt + GRACE_MS - Date.now());
    left.value = remaining / GRACE_MS;
    left.value = withTiming(0, { duration: remaining, easing: Easing.linear });
  }, [id, madeAt, left]);

  useEffect(() => {
    AccessibilityInfo.announceForAccessibility(t("undo.announce", { decision: label }));
  }, [id, label, t]);

  const bar = useAnimatedStyle(() => ({ width: `${left.value * 100}%` }));

  return (
    <Animated.View
      key={id}
      entering={FadeInDown.duration(160)}
      exiting={FadeOut.duration(160)}
      testID="undo-snackbar"
      className="bg-foreground mx-5 mb-2 overflow-hidden rounded-2xl"
    >
      <View className="flex-row items-center gap-3 py-3 pr-2 pl-4">
        <View className="flex-1">
          <Text className="text-background text-sm font-semibold">{label}</Text>
          {!online ? (
            <Text className="text-background/70 text-xs">{t("undo.whenOnline")}</Text>
          ) : null}
        </View>
        <Pressable
          testID="undo-snackbar-undo"
          accessibilityRole="button"
          accessibilityHint={t("undo.hint")}
          onPress={onUndo}
          hitSlop={8}
          className="rounded-full px-4 py-2 active:opacity-70"
        >
          <Text className="text-background text-sm font-bold">{t("undo.button")}</Text>
        </Pressable>
      </View>
      <Animated.View className="bg-background/40 h-0.5" style={bar} />
    </Animated.View>
  );
}
