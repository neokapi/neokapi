// The query client, its on-device persistence, and the signals that tell it
// when the phone is online and the app is in the foreground.
import { createAsyncStoragePersister } from "@tanstack/query-async-storage-persister";
import { focusManager, onlineManager, QueryClient } from "@tanstack/react-query";
import * as Network from "expo-network";
import Storage from "expo-sqlite/kv-store";
import { AppState, Platform } from "react-native";

import { isNetworkError } from "@neokapi/bowrain-client";

import { registerDecisionDefaults, shouldRetry } from "./decisions";

/** How long a persisted cache stays usable: a reviewer may stay offline for a day. */
export const CACHE_MAX_AGE = 24 * 60 * 60 * 1000;

export function createQueryClient(): QueryClient {
  const qc = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: CACHE_MAX_AGE,
        staleTime: 30 * 1000,
        retry: (count, err) => isNetworkError(err) && count < 2,
      },
      mutations: {
        gcTime: 10 * 60 * 1000,
        retry: shouldRetry,
      },
    },
  });
  registerDecisionDefaults(qc);
  return qc;
}

export const persister = createAsyncStoragePersister({
  storage: Storage,
  key: "bowrain.query-cache",
  throttleTime: 500,
});

let wired = false;

/** Feed connectivity and foreground state to TanStack Query. Safe to call more than once. */
export function wireAppSignals(): void {
  if (wired) return;
  wired = true;
  onlineManager.setEventListener((setOnline) => {
    const sub = Network.addNetworkStateListener((state) => {
      setOnline(state.isConnected === true && state.isInternetReachable !== false);
    });
    void Network.getNetworkStateAsync().then((state) =>
      setOnline(state.isConnected === true && state.isInternetReachable !== false),
    );
    return () => sub.remove();
  });
  focusManager.setEventListener((setFocused) => {
    const sub = AppState.addEventListener("change", (status) => {
      if (Platform.OS !== "web") setFocused(status === "active");
    });
    return () => sub.remove();
  });
}
