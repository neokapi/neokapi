// The "Try again" control a failed load offers beside its message: the plan
// card, the counts notice and the fix sheet's ready-made fixes. The screen
// hides it while the request runs, so it never asks twice at once.
import { Pressable, Text } from "react-native";
import { useTranslation } from "react-i18next";

export function RetryButton({
  onPress,
  testID,
  size = "sm",
}: {
  onPress: () => void;
  testID: string;
  /** The text size of the message it sits beside. */
  size?: "sm" | "base";
}) {
  const { t } = useTranslation();
  return (
    <Pressable testID={testID} accessibilityRole="button" onPress={onPress} hitSlop={8}>
      <Text className={size === "base" ? "text-primary text-base" : "text-primary text-sm"}>
        {t("common.retry")}
      </Text>
    </Pressable>
  );
}
