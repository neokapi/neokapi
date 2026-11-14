// The Account tab: the person, the server, the app's appearance, and
// signing out.
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";

import { tabRoot, useStackOptions } from "../../../components/stackOptions";

export default function AccountLayout() {
  const { t } = useTranslation();
  return (
    <Stack screenOptions={useStackOptions()}>
      <Stack.Screen name="index" options={{ ...tabRoot, title: t("account.title") }} />
    </Stack>
  );
}
