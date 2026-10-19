/**
 * Keep a project's block cache current without asking the person.
 *
 * The cache is the app's own reading of the project's files. Building it is
 * local and calls no provider, so the app does it when a project tab becomes
 * active and after the file watcher reports a change. `useProjectStoreSync`
 * drives that for the active tab; `useStoreSyncState` lets a surface show that
 * the files are being read, or that reading them failed.
 */

import { useCallback, useEffect, useRef } from "react";
import { useIsMutating, useMutation, useMutationState } from "@tanstack/react-query";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";
import { qk } from "../lib/queryKeys";

/** How long a burst of file changes settles before the cache is synced. */
const SETTLE_MS = 400;

/** Sync the cache of `tabID` now, and again after each change to its files. */
export function useProjectStoreSync(tabID: string | undefined): void {
  const { mutate } = useMutation({
    mutationKey: qk.storeSync(tabID ?? ""),
    mutationFn: () => api.syncProjectStore(tabID ?? ""),
  });
  const mutateRef = useRef(mutate);
  mutateRef.current = mutate;
  const tabRef = useRef(tabID);
  tabRef.current = tabID;
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const schedule = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = setTimeout(() => {
      timer.current = null;
      if (tabRef.current) mutateRef.current();
    }, SETTLE_MS);
  }, []);

  useEffect(() => {
    if (tabID) mutateRef.current();
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [tabID]);

  useWailsEvent("project-files-changed", (data) => {
    if (data === tabRef.current) schedule();
  });
}

/** Whether the cache of `tabID` is being synced, and the last sync's error. */
export function useStoreSyncState(tabID: string | undefined): {
  syncing: boolean;
  error: unknown;
} {
  const key = qk.storeSync(tabID ?? "");
  const syncing = useIsMutating({ mutationKey: key }) > 0;
  // The last settled sync decides, so an error a later sync cleared is not shown.
  const settled = useMutationState({
    filters: { mutationKey: key, predicate: (m) => m.state.status !== "pending" },
    select: (m) => ({ at: m.state.submittedAt, error: m.state.error }),
  });
  const last = settled.reduce<{ at: number; error: unknown } | null>(
    (latest, m) => (!latest || m.at >= latest.at ? m : latest),
    null,
  );
  return { syncing, error: last?.error ?? null };
}
