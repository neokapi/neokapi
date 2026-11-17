// The app shell: gestures, the session, the persisted query cache with the
// offline decision queue, and the screen stack. The stack holds the tabs
// ((tabs)/_layout.tsx), the sign-in screen and its server sheet, and the
// review deck, which opens over the tabs from Home, Review or Activity so a
// reviewer has the whole screen.
import "../global.css";
import "../lib/i18n";

import {
  persistQueryClientSave,
  PersistQueryClientProvider,
} from "@tanstack/react-query-persist-client";
import { DarkTheme, DefaultTheme, Stack, ThemeProvider } from "expo-router";
import * as SplashScreen from "expo-splash-screen";
import { StatusBar } from "expo-status-bar";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { AppState } from "react-native";
import { useTranslation } from "react-i18next";
import { useColorScheme } from "react-native";
import { GestureHandlerRootView } from "react-native-gesture-handler";
import { SafeAreaProvider } from "react-native-safe-area-context";
import { useCSSVariable } from "uniwind";

import { loadAppearance } from "../lib/appearance";
import { AuthProvider, useAuth } from "../lib/auth";
import { UpdateBanner } from "../components/UpdateBanner";
import { createUpdateController, loadUpdates, useUpdateReady } from "../lib/appUpdates";
import { isBusy } from "../lib/busy";
import { releaseDue, resendLost, setDecisionClient } from "../lib/decisions";
import {
  CACHE_BUSTER,
  CACHE_MAX_AGE,
  createQueryClient,
  persister,
  startDecisionClock,
  wireAppSignals,
} from "../lib/queryClient";
import { loadRecentProject, setRecentProject } from "../lib/recentProject";

void SplashScreen.preventAutoHideAsync();
wireAppSignals();
void loadAppearance();
void loadRecentProject();

// A deep link to the deck opens it over the tabs, so Back leads into the app.
export const unstable_settings = { anchor: "(tabs)" };

function QueryProvider({ children }: { children: ReactNode }) {
  const { ready, client, session } = useAuth();
  const [queryClient] = useState(createQueryClient);
  // Queued decisions are sent with the signed-in session's client, set before
  // the restored queue resumes.
  setDecisionClient(session ? client : null);

  useEffect(() => {
    if (ready) void SplashScreen.hideAsync();
  }, [ready]);

  useEffect(() => startDecisionClock(queryClient), [queryClient]);

  // Updates: checked on launch and on each return to the foreground, applied
  // at the next safe foreground or from the banner.
  const [updates] = useState(() =>
    createUpdateController({
      updates: loadUpdates(),
      isBusy,
      beforeReload: async () => {
        releaseDue(queryClient, Date.now(), true);
        await persistQueryClientSave({ queryClient, persister, buster: CACHE_BUSTER });
      },
    }),
  );
  useEffect(() => {
    void updates.check(Date.now());
    const sub = AppState.addEventListener("change", (status) => {
      if (status === "active") void updates.foreground(Date.now());
    });
    return () => sub.remove();
  }, [updates]);
  const updateReady = useUpdateReady(updates);

  // Signing out drops the cache, any decision still queued and the project
  // the Activity tab remembers, so the next person on the device starts from
  // nothing.
  const signedOut = ready && !session;
  useEffect(() => {
    if (!signedOut) return;
    queryClient.clear();
    void persister.removeClient();
    setRecentProject(null);
  }, [signedOut, queryClient]);

  if (!ready) return null;
  return (
    <PersistQueryClientProvider
      client={queryClient}
      persistOptions={{ persister, maxAge: CACHE_MAX_AGE, buster: CACHE_BUSTER }}
      onSuccess={() => {
        resendLost(queryClient);
        void queryClient.resumePausedMutations();
      }}
    >
      {children}
      {updateReady ? <UpdateBanner onRestart={() => void updates.apply()} /> : null}
    </PersistQueryClientProvider>
  );
}

/**
 * The navigation theme in the app's colours, so a screen's background, the
 * headers and the tab bar's glass match the page in light and dark.
 */
function useNavigationTheme() {
  const scheme = useColorScheme();
  const [background, card, foreground, border, primary] = useCSSVariable([
    "--color-background",
    "--color-card",
    "--color-foreground",
    "--color-border",
    "--color-primary",
  ]);
  return useMemo(() => {
    const base = scheme === "dark" ? DarkTheme : DefaultTheme;
    return {
      ...base,
      colors: {
        ...base.colors,
        background: String(background),
        card: String(card),
        text: String(foreground),
        border: String(border),
        primary: String(primary),
      },
    };
  }, [scheme, background, card, foreground, border, primary]);
}

function Screens() {
  const { t } = useTranslation();
  // The transparent iOS header keeps a dark title in dark mode unless it is
  // given the foreground colour.
  const foreground = useCSSVariable("--color-foreground");
  // The tabs and the deck exist only while signed in, and the sign-in screen
  // only while signed out, so neither mounts beneath the other.
  const signedIn = !!useAuth().session;
  return (
    <Stack
      screenOptions={{
        headerBackButtonDisplayMode: "minimal",
        headerTitleStyle: { color: String(foreground) },
      }}
    >
      <Stack.Screen name="index" options={{ headerShown: false }} />
      <Stack.Protected guard={signedIn}>
        <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
        {/* On iOS 26 expo-router makes headers transparent, and scrolling
            lists inset themselves below one. The deck is a fixed layout, so
            it takes an opaque header and starts below it. */}
        <Stack.Screen
          name="[ws]/[project]/[locale]"
          options={{ title: t("deck.title"), headerTransparent: false }}
        />
        <Stack.Screen name="[ws]/[project]/reviewed" options={{ title: t("reviewed.title") }} />
        <Stack.Screen
          name="[ws]/[project]/context"
          options={{ title: t("context.title"), headerTransparent: false }}
        />
      </Stack.Protected>
      <Stack.Protected guard={!signedIn}>
        <Stack.Screen name="sign-in" options={{ headerShown: false }} />
      </Stack.Protected>
      <Stack.Screen
        name="server"
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

function Themed({ children }: { children: ReactNode }) {
  return <ThemeProvider value={useNavigationTheme()}>{children}</ThemeProvider>;
}

export default function RootLayout() {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <SafeAreaProvider>
        <AuthProvider>
          <QueryProvider>
            <Themed>
              <StatusBar style="auto" />
              <Screens />
            </Themed>
          </QueryProvider>
        </AuthProvider>
      </SafeAreaProvider>
    </GestureHandlerRootView>
  );
}
