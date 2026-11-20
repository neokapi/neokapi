// The query client, its on-device persistence, and the signals that tell it
// when the phone is online and the app is in the foreground.
import { createAsyncStoragePersister } from "@tanstack/query-async-storage-persister";
import { focusManager, onlineManager, QueryClient } from "@tanstack/react-query";
import type { PersistedClient, Persister } from "@tanstack/react-query-persist-client";
import * as Network from "expo-network";
import Storage from "expo-sqlite/kv-store";
import * as Updates from "expo-updates";
import { AppState, Platform } from "react-native";

import { isNetworkError } from "@neokapi/bowrain-client";

import { registerDecisionDefaults, releaseDue, shouldRetry } from "./decisions";

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

/** The persisted cache's version: a different value discards what a device holds. */
export const CACHE_BUSTER = "1";

const stored = createAsyncStoragePersister({
  storage: Storage,
  key: "bowrain.query-cache",
  throttleTime: 500,
});

/** Where the device records which update wrote the persisted cache. */
const CACHE_UPDATE_KEY = "bowrain.query-cache.update";

/** The JavaScript update running now, or "embedded" for the build's own bundle. */
function runningUpdate(): string {
  return Updates.updateId ?? "embedded";
}

/**
 * Keeps the cached queries only when the update that wrote them is the one
 * running. A new update may read fields an older response lacks, so its first
 * launch starts from fresh queries. Queued decisions are mutations, and they
 * are kept across updates so an offline review is never lost.
 */
export function forRunningUpdate(
  client: PersistedClient | undefined,
  writtenBy: string | null,
  running: string,
): PersistedClient | undefined {
  if (!client || writtenBy === running) return client;
  return { ...client, clientState: { ...client.clientState, queries: [] } };
}

export const persister: Persister = {
  persistClient: async (client) => {
    await stored.persistClient(client);
    await Storage.setItem(CACHE_UPDATE_KEY, runningUpdate());
  },
  restoreClient: async () => {
    const client = await stored.restoreClient();
    const writtenBy = await Storage.getItem(CACHE_UPDATE_KEY);
    return forRunningUpdate(client, writtenBy, runningUpdate());
  },
  removeClient: () => stored.removeClient(),
};

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

/**
 * Send held decisions as their grace runs out, and every held decision when
 * the app leaves the foreground, where nobody can undo one and timers stop.
 * Returns the function that stops it.
 */
export function startDecisionClock(qc: QueryClient): () => void {
  const timer = setInterval(() => releaseDue(qc, Date.now()), 1000);
  const sub = AppState.addEventListener("change", (status) => {
    if (status !== "active") releaseDue(qc, Date.now(), true);
  });
  return () => {
    clearInterval(timer);
    sub.remove();
  };
}
