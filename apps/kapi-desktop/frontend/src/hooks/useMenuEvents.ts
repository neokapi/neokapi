import type { KapiProject, TabInfo } from "../types/api";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";

interface MenuEventsOptions {
  activeTabID: string | null;
  openProject: () => void;
  openRecent: (path: string) => void;
  addTab: (tab: TabInfo, project: KapiProject) => Promise<void>;
  saveTab: (tabID: string, saveAs?: boolean) => Promise<boolean>;
  setShowNewProjectForm: (v: boolean) => void;
}

export function useMenuEvents({
  activeTabID,
  openProject,
  openRecent,
  addTab,
  saveTab,
  setShowNewProjectForm,
}: MenuEventsOptions) {
  useWailsEvent("menu:new-project", () => setShowNewProjectForm(true));

  useWailsEvent("menu:open-project", () => openProject());

  useWailsEvent("menu:open-recent", (data) => {
    const p = data as string;
    if (p) openRecent(p);
  });

  useWailsEvent("menu:save-project", () => {
    if (activeTabID) void saveTab(activeTabID);
  });
  useWailsEvent("menu:save-project-as", () => {
    if (activeTabID) void saveTab(activeTabID, true);
  });

  useWailsEvent("open-project-tab", async (data) => {
    const tab = data as TabInfo;
    if (tab?.id) {
      const proj = await api.getProject(tab.id);
      if (proj) await addTab(tab, proj);
    }
  });
}
