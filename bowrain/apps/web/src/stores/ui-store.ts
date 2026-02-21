import { create } from "zustand";
import { persist } from "zustand/middleware";

interface UIState {
  sidebarCollapsed: boolean;
  setSidebarCollapsed: (collapsed: boolean) => void;

  /** Last active workspace slug — used to restore workspace on reload. */
  lastWorkspaceSlug: string | null;
  setLastWorkspaceSlug: (slug: string | null) => void;
}

export const useUIStore = create<UIState>()(
  persist(
    (set) => ({
      sidebarCollapsed: false,
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),

      lastWorkspaceSlug: null,
      setLastWorkspaceSlug: (slug) => set({ lastWorkspaceSlug: slug }),
    }),
    { name: "bowrain-ui" },
  ),
);
