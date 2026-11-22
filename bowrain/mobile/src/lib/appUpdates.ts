// Over-the-air updates, applied in one launch. The app checks when it opens
// and each time it returns to the foreground, and downloads a new update in
// the background. When the app next comes to the foreground with one ready
// and the person is not in the middle of something, it reloads behind the
// splash screen; otherwise it offers a restart (UpdateBanner) and applies it
// at the next safe foreground. Decisions waiting on the device are sent and
// the cache is saved before any reload, so a reload loses nothing.
import { useSyncExternalStore } from "react";

/** The part of expo-updates the controller uses. */
export interface UpdatesApi {
  isEnabled: boolean;
  checkForUpdateAsync(): Promise<{ isAvailable: boolean }>;
  fetchUpdateAsync(): Promise<{ isNew: boolean }>;
  reloadAsync(): Promise<void>;
}

/** How often a foreground checks again, at most. */
export const CHECK_INTERVAL_MS = 10 * 60 * 1000;

export interface UpdateController {
  /** Whether a downloaded update waits to be applied. */
  ready(): boolean;
  subscribe(listener: () => void): () => void;
  /** Look for an update and download it. */
  check(now: number): Promise<void>;
  /** The app came to the foreground: apply a ready update if it is safe, else check. */
  foreground(now: number): Promise<void>;
  /** Apply the ready update now (the banner's Restart). */
  apply(): Promise<void>;
}

export function createUpdateController(opts: {
  updates: UpdatesApi | null;
  isBusy: () => boolean;
  /** Send held decisions and save the cache. */
  beforeReload: () => Promise<void>;
}): UpdateController {
  const { updates, isBusy, beforeReload } = opts;
  let isReady = false;
  let lastCheck = -Infinity;
  let checking: Promise<void> | null = null;
  const listeners = new Set<() => void>();
  const setReady = () => {
    isReady = true;
    for (const l of listeners) l();
  };

  const apply = async () => {
    if (!updates || !isReady) return;
    await beforeReload();
    await updates.reloadAsync();
  };

  const check = (now: number): Promise<void> => {
    if (!updates?.isEnabled || isReady) return Promise.resolve();
    if (checking) return checking;
    lastCheck = now;
    checking = (async () => {
      try {
        const found = await updates.checkForUpdateAsync();
        if (!found.isAvailable) return;
        const fetched = await updates.fetchUpdateAsync();
        if (fetched.isNew) setReady();
      } catch {
        // Offline or the update server unreachable: the next foreground tries again.
      } finally {
        checking = null;
      }
    })();
    return checking;
  };

  return {
    ready: () => isReady,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    check,
    async foreground(now) {
      if (isReady) {
        if (!isBusy()) await apply();
        return;
      }
      if (now - lastCheck >= CHECK_INTERVAL_MS) await check(now);
    },
    apply,
  };
}

/** expo-updates, when its native module is in the binary. */
export function loadUpdates(): UpdatesApi | null {
  try {
    // eslint-disable-next-line @typescript-eslint/no-require-imports
    return require("expo-updates") as UpdatesApi;
  } catch {
    return null;
  }
}

/** Whether an update waits for a restart. */
export function useUpdateReady(controller: UpdateController | null): boolean {
  return useSyncExternalStore(
    (cb) => controller?.subscribe(cb) ?? (() => undefined),
    () => controller?.ready() ?? false,
  );
}
