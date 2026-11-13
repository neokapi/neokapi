// Sign in to a Bowrain server. The server sends the browser to the
// workspace's identity provider and back to the app. The server is the
// build's default; the cog opens the sheet that chooses another.
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Image, Platform, Pressable, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useCSSVariable } from "uniwind";

import { SafeAreaView } from "../components/SafeArea";
import { useAuth } from "../lib/auth";
import { serverHost, useServerChoice } from "../lib/serverChoice";

const mark = require("../../assets/images/splash-icon.png") as number;

// Bowrain's display face is a serif (Fraunces on the web). The app uses the
// platform's own serif rather than bundling a font: New York on iOS.
const displayFont = Platform.select({ ios: "ui-serif", default: "serif" });

/**
 * The Bowrain atmosphere behind the sign-in: the prism ramp of the mark,
 * faded into the page, as on the Bowrain web front page.
 */
function Atmosphere() {
  return (
    <View pointerEvents="none" className="absolute inset-x-0 top-0 h-[560px]">
      <View className="from-prism-1 via-prism-2 to-prism-4 absolute inset-0 bg-linear-to-r opacity-25" />
      <View className="from-background/0 via-background/60 to-background absolute inset-0 bg-linear-to-b" />
    </View>
  );
}

export default function SignIn() {
  const { t } = useTranslation();
  const { signIn } = useAuth();
  const { server, isDefault } = useServerChoice();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [mutedForeground, primaryForeground] = useCSSVariable([
    "--color-muted-foreground",
    "--color-primary-foreground",
  ]);

  const start = async () => {
    setBusy(true);
    setError(null);
    try {
      const res = await signIn(server);
      if (res.ok) router.replace("/workspaces");
      else if (res.reason === "failed") setError(t("signIn.failed"));
    } catch {
      setError(t("signIn.unreachable"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <View className="bg-background flex-1">
      <Atmosphere />
      <SafeAreaView className="flex-1">
        <View className="flex-row justify-end px-3 pt-1">
          <Pressable
            testID="server-settings"
            accessibilityRole="button"
            accessibilityLabel={t("signIn.serverSettings")}
            hitSlop={8}
            onPress={() => router.push("/server")}
            className="h-11 w-11 items-center justify-center rounded-full active:opacity-60"
          >
            <Ionicons name="settings-outline" size={24} color={String(mutedForeground)} />
          </Pressable>
        </View>
        <View className="flex-1 items-center justify-center gap-4 px-6">
          <Image source={mark} className="h-32 w-32" accessibilityIgnoresInvertColors />
          <Text
            accessibilityRole="header"
            className="text-foreground text-5xl font-semibold tracking-tight"
            style={{ fontFamily: displayFont }}
          >
            {t("signIn.title")}
          </Text>
          <View className="from-prism-1 via-prism-3 to-prism-5 h-1 w-20 rounded-full bg-linear-to-r" />
          {isDefault ? null : (
            <Text testID="server-host" className="text-muted-foreground text-sm">
              {serverHost(server)}
            </Text>
          )}
        </View>
        <View className="gap-4 px-6 pb-6">
          {error ? (
            <Text testID="sign-in-error" className="text-corrective text-center text-base">
              {error}
            </Text>
          ) : null}
          <Pressable
            testID="sign-in"
            accessibilityRole="button"
            disabled={busy}
            onPress={start}
            className="bg-primary items-center rounded-2xl py-4 active:opacity-80"
          >
            {busy ? (
              <ActivityIndicator color={String(primaryForeground)} />
            ) : (
              <Text className="text-primary-foreground text-base font-semibold">
                {t("signIn.button")}
              </Text>
            )}
          </Pressable>
        </View>
      </SafeAreaView>
    </View>
  );
}
