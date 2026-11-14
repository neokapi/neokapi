// The app shell: gestures, the session, the persisted query cache with the
// offline decision queue, and the screen stack.
import "../global.css";
import "../lib/i18n";

import { PersistQueryClientProvider } from "@tanstack/react-query-persist-client";
import { Stack } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { useCSSVariable } from "uniwind";

import { AuthProvider, useAuth } from "../lib/auth";
import { setDecisionClient } from "../lib/decisions";
import { CACHE_MAX_AGE, createQueryClient, persister, wireAppSignals } from "../lib/queryClient";

void SplashScreen.preventAutoHideAsync();
wireAppSignals();

function QueryProvider({ children }: { children: ReactNode }) {
  const { ready, client, session } = useAuth();
  const [queryClient] = useState(createQueryClient);
  // Queued decisions are sent with the signed-in session's client, set before
  // the restored queue resumes.
  setDecisionClient(session ? client : null);

  useEffect(() => {
    if (ready) void SplashScreen.hideAsync();
  }, [ready]);

  // Signing out drops the cache and any decision still queued, so the next
  // person on the device starts from nothing.
  const signedOut = ready && !session;
  useEffect(() => {
    if (!signedOut) return;
    queryClient.clear();
    void persister.removeClient();
  }, [signedOut, queryClient]);

  if (!ready) return null;
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister, maxAge: CACHE_MAX_AGE, buster: "1" }}
      onSuccess={() => {
        void queryClient.resumePausedMutations();
      }}
    >
      {children}
    </PersistQueryClientProvider>
  );
}

function Screens() {
  const { t } = useTranslation();
  // The transparent iOS header keeps a dark title in dark mode unless it is
  // given the foreground colour.
  const foreground = useCSSVariable("--color-foreground");
  return (
    <Stack
      screenOptions={{
        headerBackButtonDisplayMode: "minimal",
        headerTitleStyle: { color: String(foreground) },
      }}
    >
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Screen name="sign-in" options={{ headerShown: false }} />
      <Stack.Screen
        name="server"
        options={{
          headerShown: false,
          presentation: "formSheet",
          sheetAllowedDetents: [0.6, 1],
          sheetGrabberVisible: true,
        }}
      />
      <Stack.Screen name="workspaces" options={{ title: t("workspaces.title") }} />
      <Stack.Screen name="[ws]/index" options={{ title: t("projects.title") }} />
      <Stack.Screen name="[ws]/[project]/index" options={{ title: t("languages.title") }} />
      <Stack.Screen name="[ws]/[project]/feed/index" options={{ title: t("feed.title") }} />
      <Stack.Screen name="[ws]/[project]/feed/[entry]" options={{ title: t("feed.title") }} />
      {/* On iOS 26 expo-router makes headers transparent, and scrolling
          lists inset themselves below one. The deck is a fixed layout, so
          it takes an opaque header and starts below it. */}
      <Stack.Screen
        name="[ws]/[project]/[locale]"
        options={{ title: t("deck.title"), headerTransparent: false }}
      />
    </Stack>
  );
}

export default function RootLayout() {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <AuthProvider>
          <QueryProvider>
            <StatusBar style="auto" />
            <Screens />
          </QueryProvider>
        </AuthProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
