import { useEffect, useRef } from "react";
import type { KapiProject, TabInfo } from "../types/api";
import { api } from "./useApi";
import { useWailsEvent, wailsEventsReady } from "./useWailsEvent";
import { useError } from "../components/ErrorBanner";

interface MenuEventsOptions {
  activeTabID: string | null;
  openProject: () => void;
  openRecent: (path: string) => void;
  addTab: (tab: TabInfo, project: KapiProject) => Promise<void>;
  updateTabInfo: (tab: TabInfo) => void;
  setShowNewProjectForm: (v: boolean) => void;
}

export function useMenuEvents({
  activeTabID,
  openProject,
  openRecent,
  addTab,
  updateTabInfo,
  setShowNewProjectForm,
}: MenuEventsOptions) {
  const { showError } = useError();

  // Use ref so event callbacks always see the latest activeTabID.
  const activeTabIDRef = useRef(activeTabID);
  activeTabIDRef.current = activeTabID;

  useWailsEvent("menu:new-project", () => setShowNewProjectForm(true));

  useWailsEvent("menu:open-project", () => openProject());

  useWailsEvent("menu:open-recent", (data) => {
    const p = data as string;
    if (p) openRecent(p);
  });

  useWailsEvent("menu:save-project", async () => {
    const tabID = activeTabIDRef.current;
    if (!tabID) return;
    try {
      const path = await api.getProjectPath(tabID);
      if (path) await api.saveProject(tabID);
      else {
        const u = await api.saveProjectDialog(tabID);
        if (u) updateTabInfo(u);
      }
    } catch (err) {
      showError("Failed to save project", err);
    }
  });

  useWailsEvent("menu:save-project-as", async () => {
    const tabID = activeTabIDRef.current;
    if (!tabID) return;
    try {
      const u = await api.saveProjectDialog(tabID);
      if (u) updateTabInfo(u);
    } catch (err) {
      showError("Failed to save project", err);
    }
  });

  useWailsEvent("open-project-tab", async (data) => {
    const tab = data as TabInfo;
    if (tab?.id) {
      const proj = await api.getProject(tab.id);
      if (proj) await addTab(tab, proj);
    }
  });

  // A folder handed to the app as it starts (`kapi open`, a path argument, a
  // recipe dropped on the dock icon) can open before this window listens for
  // "open-project-tab". Once the listener is in place, every tab the backend
  // holds is shown; addTab leaves a tab that is already shown as it is.
  useEffect(() => {
    let cancelled = false;
    void wailsEventsReady().then(async (ready) => {
      if (!ready || cancelled) return;
      const tabs = (await api.listTabs()) ?? [];
      for (const tab of tabs) {
        if (cancelled) return;
        const proj = await api.getProject(tab.id);
        if (proj) await addTab(tab, proj);
      }
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- runs once, as the window starts
  }, []);
}
