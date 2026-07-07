import { useCallback, useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";
import { useInvalidateOnEvent } from "./useInvalidateOnEvent";

/**
 * App-level initialization: theme, recent files, external link intercept.
 *
 * Server state (settings, recent files) reads through react-query; the persisted
 * theme is applied as a side effect of the settings query resolving.
 */
export function useAppInit() {
  const qc = useQueryClient();

  const settingsQuery = useQuery({
    queryKey: qk.settings(),
    queryFn: () => api.getSettings(),
  });
  const recentQuery = useQuery({
    queryKey: qk.recentFiles(),
    queryFn: () => api.listRecentFiles(),
  });

  // Optimistic dismissal — flips immediately, before the settings refetch.
  const [dismissedOverride, setDismissedOverride] = useState(false);

  const recentFiles = recentQuery.data ?? [];
  const samplesDismissed = settingsQuery.data
    ? dismissedOverride || !!settingsQuery.data.samples_dismissed
    : true;

  const refreshRecent = useCallback(() => {
    void qc.invalidateQueries({ queryKey: qk.recentFiles() });
  }, [qc]);

  // Apply the persisted theme once settings load.
  useEffect(() => {
    const s = settingsQuery.data;
    if (!s) return;
    const mode = s.theme || "system";
    if (mode === "system") {
      const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
      document.documentElement.classList.toggle("dark", prefersDark);
    } else {
      document.documentElement.classList.toggle("dark", mode === "dark");
    }
  }, [settingsQuery.data]);

  // Per Wails v3 docs: common:ApplicationStarted fires after all
  // ServiceStartup hooks complete — data is guaranteed available.
  useInvalidateOnEvent("common:ApplicationStarted", [qk.recentFiles()]);

  // Intercept external link clicks and open in the system browser.
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      const anchor = (e.target as HTMLElement).closest("a[href]") as HTMLAnchorElement | null;
      if (!anchor) return;
      const href = anchor.getAttribute("href");
      if (!href || href.startsWith("#") || href.startsWith("/")) return;
      e.preventDefault();
      import("@wailsio/runtime")
        .then((m) => m.Browser.OpenURL(href))
        .catch(() => {
          window.open(href, "_blank");
        });
    };
    document.addEventListener("click", handler);
    return () => document.removeEventListener("click", handler);
  }, []);

  const dismissSamples = useCallback(() => {
    setDismissedOverride(true);
    api
      .dismissSamples()
      .catch(() => {})
      .finally(() => {
        void qc.invalidateQueries({ queryKey: qk.settings() });
      });
  }, [qc]);

  return { recentFiles, samplesDismissed, refreshRecent, dismissSamples };
}
