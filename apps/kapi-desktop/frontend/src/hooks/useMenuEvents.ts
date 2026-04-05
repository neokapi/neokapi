import { useRef } from "react";
import type { TabInfo } from "../types/api";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";
import { useError } from "../components/ErrorBanner";

interface MenuEventsOptions {
  activeTabID: string | null;
  openProject: () => void;
  openRecent: (path: string) => void;
  addTab: (tab: TabInfo, project: unknown) => Promise<void>;
  updateTabInfo: (tab: TabInfo) => void;
  setShowNewProjectForm: (v: boolean) => void;
  setMode: (m: "projects") => void;
}

export function useMenuEvents({
  activeTabID,
  openProject,
  openRecent,
  addTab,
  updateTabInfo,
  setShowNewProjectForm,
  setMode,
}: MenuEventsOptions) {
  const { showError } = useError();

  // Use ref so event callbacks always see the latest activeTabID.
  const activeTabIDRef = useRef(activeTabID);
  activeTabIDRef.current = activeTabID;

  useWailsEvent("menu:new-project", () => {
    setMode("projects");
    setShowNewProjectForm(true);
  });

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
}
