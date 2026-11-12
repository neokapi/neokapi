// Sign in to a Bowrain server. The server sends the browser to the
// workspace's identity provider and back to the app.
import { router } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, Text, TextInput, View } from "react-native";
import { useTranslation } from "react-i18next";
import { SafeAreaView } from "react-native-safe-area-context";

import { useAuth } from "../lib/auth";
import { defaultServerUrl } from "../lib/config";

export default function SignIn() {
  const { t } = useTranslation();
  const { signIn } = useAuth();
  const [server, setServer] = useState(defaultServerUrl);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const start = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await signIn(server.trim());
      if (res.ok) router.replace("/workspaces");
      else if (res.reason === "failed") setError(t("signIn.failed"));
    } catch {
      setError(t("signIn.unreachable"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <SafeAreaView className="bg-background flex-1">
      <View className="flex-1 justify-center gap-8 px-6">
        <View className="gap-2">
          <Text className="text-foreground text-3xl font-bold">{t("signIn.title")}</Text>
          <Text className="text-muted-foreground text-base">{t("signIn.lead")}</Text>
        </View>
        <View className="gap-2">
          <Text className="text-muted-foreground text-xs font-semibold uppercase">{t("signIn.server")}</Text>
          <TextInput
            testID="server-url"
            className="bg-card border-border text-foreground rounded-xl border p-4 text-base"
            value={server}
            onChangeText={setServer}
            autoCapitalize="none"
            autoCorrect={false}
            keyboardType="url"
            inputMode="url"
          />
        </View>
        <Pressable
          testID="sign-in"
          accessibilityRole="button"
          disabled={busy || server.trim() === ""}
          onPress={start}
          className="bg-primary items-center rounded-2xl py-4 active:opacity-80"
        >
          {busy ? (
            <ActivityIndicator color="#ffffff" />
          ) : (
            <Text className="text-primary-foreground text-base font-semibold">{t("signIn.button")}</Text>
          )}
        </Pressable>
        {error ? (
          <Text testID="sign-in-error" className="text-corrective text-base">
            {error}
          </Text>
        ) : null}
      </View>
    </SafeAreaView>
  );
}
