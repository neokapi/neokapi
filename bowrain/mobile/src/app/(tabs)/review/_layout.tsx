// The Review tab: the languages waiting for the person across projects. A
// language opens its collections, and a collection opens its deck over the
// tabs.
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";

import { tabRoot, useStackOptions } from "../../../components/stackOptions";

export default function ReviewLayout() {
  const { t } = useTranslation();
  return (
    <Stack screenOptions={useStackOptions()}>
      <Stack.Screen name="index" options={{ ...tabRoot, title: t("inbox.title") }} />
      <Stack.Screen name="[ws]/[project]/[locale]" options={{ title: t("collections.title") }} />
    </Stack>
  );
}
