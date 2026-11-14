// The signed-in person's avatar in the top-right of the Home header. It
// opens the Account tab.
import { router, Stack } from "expo-router";
import { Pressable } from "react-native";
import { useTranslation } from "react-i18next";

import { useProfile } from "../lib/profile";
import { Avatar } from "./feed/Avatar";

export function AccountButton() {
  const { t } = useTranslation();
  const profile = useProfile();
  return (
    <Stack.Toolbar placement="right">
      <Stack.Toolbar.View hidesSharedBackground>
        <Pressable
          testID="account-button"
          accessibilityRole="button"
          accessibilityLabel={t("account.open", { name: profile.name })}
          hitSlop={6}
          onPress={() => router.navigate("/account")}
          className="active:opacity-70"
        >
          <Avatar actor={{ id: profile.key, kind: "person" }} name={profile.name} size={34} />
        </Pressable>
      </Stack.Toolbar.View>
    </Stack.Toolbar>
  );
}
