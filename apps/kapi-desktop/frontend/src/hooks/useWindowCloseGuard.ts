import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";

/** A project tab, as the close dialog names it. */
export interface UnsavedTab {
  id: string;
  name: string;
}

/** One tab that holds the window, with each reason it does. */
export interface CloseRow extends UnsavedTab {
  unsaved: boolean;
  running: boolean;
}

/** The event the backend emits when it holds a window close or a quit. */
export const CLOSE_REQUESTED_EVENT = "app:close-requested";

/** A blocker as the backend sends it (host/desktopclose.Blocker). */
export interface CloseBlocker {
  id: string;
  name: string;
  reasons: string[];
}

/** The payload of CLOSE_REQUESTED_EVENT. */
export interface CloseRequest {
  route: string;
  blockers: CloseBlocker[];
}

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

/**
 * closeRows merges the unsaved tabs and the tabs running a flow into one row
 * per tab, in tab order.
 */
export function closeRows(
  tabs: UnsavedTab[],
  unsaved: UnsavedTab[],
  runningIDs: string[],
): CloseRow[] {
  const unsavedIDs = new Set(unsaved.map((tab) => tab.id));
  const running = new Set(runningIDs);
  return tabs
    .filter((tab) => unsavedIDs.has(tab.id) || running.has(tab.id))
    .map((tab) => ({ ...tab, unsaved: unsavedIDs.has(tab.id), running: running.has(tab.id) }));
}

interface WindowCloseGuardOptions {
  /** Every open tab, in tab order. */
  tabs: UnsavedTab[];
  /** Whether a tab has unsaved changes; the same test tab close uses. */
  hasChanges: (tabID: string) => boolean;
  /** Saves one tab; resolves false when the save failed or was refused. */
  saveTab: (tabID: string) => Promise<boolean>;
  /**
   * The tabs whose latest flow has finished, from the job feed. A running row
   * the backend sent drops once its tab is here.
   */
  finishedTabIDs: string[];
  /** Cancels a tab's running flow, as tab close's Cancel Flow & Close does. */
  cancelRun: (tabID: string) => Promise<unknown>;
}

/** The rows of an open request, as the backend sent them. */
interface OpenRequest {
  unsaved: UnsavedTab[];
  running: string[];
  names: Map<string, string>;
}

/**
 * useWindowCloseGuard holds the window open while a tab has unsaved changes
 * or a running flow.
 *
 * It reports the tabs with changes to the backend. The backend holds a window
 * close or a quit while that list is not empty or a flow runs, and emits
 * CLOSE_REQUESTED_EVENT with the blockers. The dialog's rows are the
 * backend's blockers, not the page's own view: a page reloaded during a run
 * still learns that the run holds the window. The hook acknowledges each
 * request, and shows nothing when the backend answers that its native dialog
 * already asks. It clears the list when it unmounts or the page unloads, so a
 * page that is gone never holds the window.
 *
 * Save all saves the unsaved tabs and lets the close finish only when each
 * save succeeded. Discard all lets it finish without saving. Both cancel the
 * running flows first, as tab close's Cancel Flow & Close does. Cancel keeps
 * everything, as Keep running does. A flow that finishes while the dialog is
 * open drops its row; when no row is left, the close goes ahead.
 */
export function useWindowCloseGuard({
  tabs,
  hasChanges,
  saveTab,
  finishedTabIDs,
  cancelRun,
}: WindowCloseGuardOptions) {
  const unsaved = tabs.filter((tab) => hasChanges(tab.id));
  const unsavedKey = unsaved.map((tab) => tab.id).join("\n");

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

  const [request, setRequest] = useState<OpenRequest | null>(null);
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState<UnsavedTab[]>([]);

  const finished = new Set(finishedTabIDs);
  const stillRunning = request ? request.running.filter((id) => !finished.has(id)) : [];
  const runningRef = useRef(stillRunning);
  runningRef.current = stillRunning;

  // Rows in tab order, named as the page names its tabs; a blocker for a tab
  // the page does not show keeps the backend's name.
  const known = new Set(tabs.map((tab) => tab.id));
  const rowTabs = request
    ? [
        ...tabs,
        ...[...request.names].filter(([id]) => !known.has(id)).map(([id, name]) => ({ id, name })),
      ]
    : [];
  const rows = request ? closeRows(rowTabs, request.unsaved, stillRunning) : [];

  const onCloseRequested = useCallback(async (data: unknown) => {
    const show = await api.acknowledgeClose().catch(() => true);
    // The backend's native dialog already asks; one question on screen.
    if (show === false) return;
    const blockers = (data as CloseRequest | undefined)?.blockers ?? [];
    if (blockers.length === 0) {
      void api.confirmClose();
      return;
    }
    const names = new Map(blockers.map((b) => [b.id, b.name]));
    setFailed([]);
    setRequest({
      unsaved: blockers
        .filter((b) => b.reasons.includes("unsaved"))
        .map((b) => ({ id: b.id, name: b.name })),
      running: blockers.filter((b) => b.reasons.includes("running")).map((b) => b.id),
      names,
    });
  }, []);
  useWailsEvent(CLOSE_REQUESTED_EVENT, (data) => void onCloseRequested(data));

  const finish = useCallback(async () => {
    for (const id of runningRef.current) await cancelRun(id).catch(() => undefined);
    setRequest(null);
    setFailed([]);
    await api.confirmClose();
  }, [cancelRun]);

  // A flow that finishes while the dialog is open was the last thing holding
  // the window: the close the person asked for goes ahead.
  const empty = request !== null && rows.length === 0 && !saving;
  useEffect(() => {
    if (empty) void finish();
  }, [empty, finish]);

  const saveAll = useCallback(async () => {
    if (!request || saving) return;
    setSaving(true);
    try {
      const notSaved = await saveTabs(request.unsaved, saveTab);
      if (notSaved.length > 0) {
        setFailed(notSaved);
        setRequest({ ...request, unsaved: notSaved });
        return;
      }
      await finish();
    } finally {
      setSaving(false);
    }
  }, [request, saving, saveTab, finish]);

  const discardAll = useCallback(async () => {
    if (saving) return;
    await finish();
  }, [saving, finish]);

  const cancel = useCallback(async () => {
    if (saving) return;
    setRequest(null);
    setFailed([]);
    await api.cancelClose();
  }, [saving]);

  return {
    open: request !== null && rows.length > 0,
    rows,
    saving,
    failed,
    saveAll,
    discardAll,
    cancel,
    onCloseRequested,
  };
}
