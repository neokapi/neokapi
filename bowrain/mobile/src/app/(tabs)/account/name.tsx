// The name the person shows under, on their feed entries and their reviews.
// Left empty, Bowrain shows their handle instead.
import { router } from "expo-router";
import { useState } from "react";
import { Pressable, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";

import { useProfile, useSetName } from "../../../lib/profile";

/** The longest name the server takes, in characters. */
const MAX_NAME = 80;

export default function Name() {
  const { t } = useTranslation();
  const profile = useProfile();
  const setName = useSetName();
  const [value, setValue] = useState(profile.ownName ?? "");

  const save = () => {
    setName.mutate(value.trim(), { onSuccess: () => router.back() });
  };

  return (
    <View className="bg-background flex-1 gap-3 px-4 pt-6">
      <TextInput
        testID="account-name-input"
        accessibilityLabel={t("account.name.title")}
        className="bg-card border-border text-foreground rounded-xl border p-4 text-base"
        value={value}
        onChangeText={setValue}
        placeholder={profile.name}
        maxLength={MAX_NAME}
        autoCapitalize="words"
        autoComplete="name"
        textContentType="name"
        autoFocus
        returnKeyType="done"
        onSubmitEditing={save}
      />
      <Text className="text-muted-foreground px-2 text-sm">{t("account.name.footer")}</Text>
      {setName.isError ? (
        <Text testID="account-name-error" className="text-corrective px-2 text-sm">
          {t("account.name.failed")}
        </Text>
      ) : null}
      <Pressable
        testID="account-name-save"
        accessibilityRole="button"
        disabled={setName.isPending}
        onPress={save}
        className="bg-primary items-center rounded-2xl py-4 active:opacity-80 disabled:opacity-50"
      >
        <Text className="text-primary-foreground text-base font-semibold">
          {t("account.name.save")}
        </Text>
      </Pressable>
    </View>
  );
}
