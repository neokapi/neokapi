// The offer to restart into a downloaded update, shown when the app could
// not apply it on its own because the person was in the middle of something.
// Later hides it; the next safe return to the app applies the update anyway.
import { useState } from "react";
import { Pressable, Text, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTranslation } from "react-i18next";

export function UpdateBanner({ onRestart }: { onRestart: () => void }) {
  const { t } = useTranslation();
  const insets = useSafeAreaInsets();
  const [hidden, setHidden] = useState(false);
  if (hidden) return null;
  return (
    <View
      testID="update-banner"
      pointerEvents="box-none"
      className="absolute right-0 left-0"
      style={{ top: insets.top + 8 }}
    >
      <View className="bg-foreground mx-4 flex-row items-center gap-2 rounded-2xl py-2 pr-2 pl-4">
        <Text className="text-background flex-1 text-sm font-semibold">{t("update.ready")}</Text>
        <Pressable
          testID="update-later"
          accessibilityRole="button"
          onPress={() => setHidden(true)}
          hitSlop={8}
          className="rounded-full px-3 py-2 active:opacity-70"
        >
          <Text className="text-background/70 text-sm">{t("update.later")}</Text>
        </Pressable>
        <Pressable
          testID="update-restart"
          accessibilityRole="button"
          accessibilityHint={t("update.restartHint")}
          onPress={onRestart}
          hitSlop={8}
          className="bg-background rounded-full px-4 py-2 active:opacity-80"
        >
          <Text className="text-foreground text-sm font-bold">{t("update.restart")}</Text>
        </Pressable>
      </View>
    </View>
  );
}
