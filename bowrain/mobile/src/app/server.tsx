// The server sheet, opened from the cog on the sign-in screen: sign in to
// another Bowrain server, or go back to the build's default.
import { router } from "expo-router";
import { useState } from "react";
import { Pressable, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";

import { defaultServerUrl } from "../lib/config";
import {
  normalizeServerUrl,
  serverHost,
  setServerChoice,
  useServerChoice,
} from "../lib/serverChoice";

export default function Server() {
  const { t } = useTranslation();
  const { server, isDefault } = useServerChoice();
  const [value, setValue] = useState(server);
  const [invalid, setInvalid] = useState(false);

  const save = async () => {
    const url = normalizeServerUrl(value);
    if (!url) {
      setInvalid(true);
      return;
    }
    await setServerChoice(url);
    router.back();
  };

  const reset = async () => {
    await setServerChoice(null);
    router.back();
  };

  return (
    <View className="bg-background flex-1 gap-4 px-6 pt-6">
      <Text accessibilityRole="header" className="text-foreground text-xl font-semibold">
        {t("server.title")}
      </Text>
      <View className="gap-2">
        <TextInput
          testID="server-url"
          accessibilityLabel={t("server.title")}
          className="bg-card border-border text-foreground rounded-xl border p-4 text-base"
          value={value}
          onChangeText={(v) => {
            setValue(v);
            setInvalid(false);
          }}
          placeholder={serverHost(defaultServerUrl)}
          autoCapitalize="none"
          autoCorrect={false}
          autoFocus
          keyboardType="url"
          inputMode="url"
          returnKeyType="done"
          onSubmitEditing={() => void save()}
        />
        {invalid ? (
          <Text testID="server-invalid" className="text-corrective text-sm">
            {t("server.invalid")}
          </Text>
        ) : null}
      </View>
      <Pressable
        testID="server-save"
        accessibilityRole="button"
        disabled={value.trim() === ""}
        onPress={() => void save()}
        className="bg-primary items-center rounded-2xl py-4 active:opacity-80 disabled:opacity-50"
      >
        <Text className="text-primary-foreground text-base font-semibold">{t("server.save")}</Text>
      </Pressable>
      {isDefault ? null : (
        <Pressable
          testID="server-reset"
          accessibilityRole="button"
          onPress={() => void reset()}
          className="items-center py-2 active:opacity-60"
        >
          <Text className="text-primary text-base">
            {t("server.reset", { host: serverHost(defaultServerUrl) })}
          </Text>
        </Pressable>
      )}
    </View>
  );
}
