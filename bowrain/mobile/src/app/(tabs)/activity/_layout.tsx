// The Activity tab: a project's feed, an entry's detail, and the sheet that
// picks the project.
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";

import { tabRoot, useStackOptions } from "../../../components/stackOptions";

export const unstable_settings = { anchor: "index" };

export default function ActivityLayout() {
  const { t } = useTranslation();
  return (
    <Stack screenOptions={useStackOptions()}>
      <Stack.Screen name="index" options={{ ...tabRoot, title: t("activity.title") }} />
      <Stack.Screen name="[ws]/[project]/[entry]" options={{ title: t("feed.title") }} />
      <Stack.Screen
        name="pick"
        options={{
          headerShown: false,
          presentation: "formSheet",
          sheetAllowedDetents: [0.6, 1],
          sheetGrabberVisible: true,
        }}
      />
    </Stack>
  );
}
