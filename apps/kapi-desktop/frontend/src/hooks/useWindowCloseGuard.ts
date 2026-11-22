import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";

/** A project tab with unsaved changes, as the close dialog lists it. */
export interface UnsavedTab {
  id: string;
  name: string;
}

/** The event the backend emits when it holds a window close or a quit. */
export const CLOSE_REQUESTED_EVENT = "app:close-requested";

/**
 * saveTabs saves each tab in turn and returns the tabs whose save failed. It
 * keeps going after a failure, so one tab that cannot be written does not
 * leave the others unsaved.
 */
export async function saveTabs(
  tabs: UnsavedTab[],
  saveTab: (tabID: string) => Promise<boolean>,
): Promise<UnsavedTab[]> {
  const failed: UnsavedTab[] = [];
  for (const tab of tabs) {
    const saved = await saveTab(tab.id).catch(() => false);
    if (!saved) failed.push(tab);
  }
  return failed;
}

interface WindowCloseGuardOptions {
  /** Every open tab, in tab order. */
  tabs: UnsavedTab[];
  /** Whether a tab has unsaved changes; the same test tab close uses. */
  hasChanges: (tabID: string) => boolean;
  /** Saves one tab; resolves false when the save failed or was refused. */
  saveTab: (tabID: string) => Promise<boolean>;
}

/**
 * useWindowCloseGuard holds the window open while a tab has unsaved changes.
 *
 * It reports the tabs with changes to the backend, which holds a window close
 * or a quit while that list is not empty and emits CLOSE_REQUESTED_EVENT. The
 * hook acknowledges each request at once, so the backend keeps its native
 * fallback dialog closed, and clears the list when it unmounts or the page
 * unloads, so a page that is gone never holds the window. It then exposes
 * the request for one dialog: Save all saves every listed
 * tab and lets the close go ahead only when each save succeeded, Discard all
 * lets it go ahead, and Cancel drops it.
 */
export function useWindowCloseGuard({ tabs, hasChanges, saveTab }: WindowCloseGuardOptions) {
  const unsaved = tabs.filter((tab) => hasChanges(tab.id));
  const unsavedKey = unsaved.map((tab) => tab.id).join("\n");
  const unsavedRef = useRef(unsaved);
  unsavedRef.current = unsaved;

  useEffect(() => {
    const ids = unsavedKey ? unsavedKey.split("\n") : [];
    void api.setUnsavedTabs(ids).catch(() => undefined);
  }, [unsavedKey]);

  // A page that unmounts or unloads (a reload, a crash recovery) can no
  // longer answer for its tabs, so it stops holding the window.
  useEffect(() => {
    const clear = () => void api.setUnsavedTabs([]).catch(() => undefined);
    window.addEventListener("pagehide", clear);
    return () => {
      window.removeEventListener("pagehide", clear);
      clear();
    };
  }, []);

  const [request, setRequest] = useState<UnsavedTab[] | null>(null);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState<UnsavedTab[]>([]);

  const onCloseRequested = useCallback(() => {
    void api.acknowledgeClose().catch(() => undefined);
    const pending = unsavedRef.current;
    if (pending.length === 0) {
      void api.confirmClose();
      return;
    }
    setFailed([]);
    setRequest(pending);
  }, []);
  useWailsEvent(CLOSE_REQUESTED_EVENT, onCloseRequested);

  const saveAll = useCallback(async () => {
    if (!request || saving) return;
    setSaving(true);
    try {
      const notSaved = await saveTabs(request, saveTab);
      if (notSaved.length > 0) {
        setFailed(notSaved);
        setRequest(notSaved);
        return;
      }
      setRequest(null);
      await api.confirmClose();
    } finally {
      setSaving(false);
    }
  }, [request, saving, saveTab]);

  const discardAll = useCallback(async () => {
    if (saving) return;
    setRequest(null);
    await api.confirmClose();
  }, [saving]);

  const cancel = useCallback(async () => {
    if (saving) return;
    setRequest(null);
    setFailed([]);
    await api.cancelClose();
  }, [saving]);

  return { request, saving, failed, saveAll, discardAll, cancel, onCloseRequested };
}
