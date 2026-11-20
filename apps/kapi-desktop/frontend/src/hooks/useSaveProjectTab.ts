import { useCallback, useRef } from "react";
import { api } from "./useApi";
import type { useProjectHistory } from "./useProjectHistory";
import type { useTabManager } from "./useTabManager";

export function useSaveProjectTab(
  history: Pick<ReturnType<typeof useProjectHistory>, "snapshot" | "markSaved">,
  tm: Pick<ReturnType<typeof useTabManager>, "updateTab" | "checkPluginStatus">,
  showError: (message: string, error: unknown) => void,
) {
  const pendingSaves = useRef(new Map<string, Promise<boolean>>()).current;
  return useCallback(
    async (tabID: string, saveAs = false) => {
      const previous = pendingSaves.get(tabID);
      const saving = (async () => {
        await previous;
        const project = history.snapshot(tabID);
        if (!project) return false;
        const saved = await api.saveProjectSnapshot(tabID, project, saveAs);
        if (!saved) return false;
        tm.updateTab(tabID, { info: saved.tab, project: { ...project, name: saved.name } });
        history.markSaved(tabID, project, saved.name);
        void tm.checkPluginStatus(tabID);
        return true;
      })().catch((error: unknown) => {
        showError("Failed to save project", error);
        return false;
      });
      pendingSaves.set(tabID, saving);
      try {
        return await saving;
      } finally {
        if (pendingSaves.get(tabID) === saving) pendingSaves.delete(tabID);
      }
    },
    [history, pendingSaves, showError, tm],
  );
}
