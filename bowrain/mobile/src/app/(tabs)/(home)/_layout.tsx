// The Home tab: workspaces, then a workspace's projects, then a project's
// languages. A language opens its deck over the tabs.
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";

import { tabRoot, useStackOptions } from "../../../components/stackOptions";

// A deep link to a project lands on top of the workspace list.
export const unstable_settings = { anchor: "workspaces" };

export default function HomeLayout() {
  const { t } = useTranslation();
  return (
    <Stack screenOptions={useStackOptions()}>
      <Stack.Screen name="workspaces" options={{ ...tabRoot, title: t("workspaces.title") }} />
      <Stack.Screen name="[ws]/index" options={{ title: t("projects.title") }} />
      <Stack.Screen name="[ws]/[project]/index" options={{ title: t("languages.title") }} />
    </Stack>
  );
}
