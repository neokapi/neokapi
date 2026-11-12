// List rows and the loading and error states the list screens share.
import type { UseQueryResult } from "@tanstack/react-query";
import { ActivityIndicator, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";

export function Row({
  title,
  detail,
  trailing,
  onPress,
  testID,
}: {
  title: string;
  detail?: string;
  trailing?: string;
  onPress?: () => void;
  testID?: string;
}) {
  return (
    <Pressable
      testID={testID}
      accessibilityRole="button"
      onPress={onPress}
      className="bg-card border-border flex-row items-center gap-3 border-b px-4 py-4 active:opacity-70"
    >
      <View className="flex-1 gap-0.5">
        <Text className="text-foreground text-base font-medium">{title}</Text>
        {detail ? <Text className="text-muted-foreground text-sm">{detail}</Text> : null}
      </View>
      {trailing !== undefined ? (
        <Text className="text-foreground text-base font-semibold tabular-nums">{trailing}</Text>
      ) : null}
    </Pressable>
  );
}

/** The loading, error and empty states of a query that feeds a list. */
export function QueryState({
  query,
  empty,
}: {
  query: Pick<UseQueryResult, "isPending" | "isError" | "refetch">;
  empty?: string | false;
}) {
  const { t } = useTranslation();
  if (query.isPending) {
    return (
      <View className="items-center p-8">
        <ActivityIndicator />
      </View>
    );
  }
  if (query.isError) {
    return (
      <View className="items-center gap-3 p-8">
        <Text className="text-muted-foreground text-base">{t("common.loadFailed")}</Text>
        <Pressable accessibilityRole="button" onPress={() => void query.refetch()}>
          <Text className="text-primary text-base">{t("common.retry")}</Text>
        </Pressable>
      </View>
    );
  }
  if (empty) {
    return (
      <View className="items-center p-8">
        <Text className="text-muted-foreground text-base">{empty}</Text>
      </View>
    );
  }
  return null;
}
