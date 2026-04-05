import { useState, useEffect, useCallback } from "react";
import { api } from "./useApi";
import { useWailsEvent } from "./useWailsEvent";

/**
 * App-level initialization: theme, recent files, external link intercept.
 */
export function useAppInit() {
  const [recentFiles, setRecentFiles] = useState<
    Array<{ path: string; name: string; opened_at: string }>
  >([]);
  const [samplesDismissed, setSamplesDismissed] = useState(true);

  const refreshRecent = useCallback(() => {
    void api.listRecentFiles().then((f) => {
      if (f) setRecentFiles(f);
    });
  }, []);

  // Apply persisted theme and load settings on startup.
  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        if (s) {
          setSamplesDismissed(!!s.samples_dismissed);
          const mode = s.theme || "system";
          if (mode === "system") {
            const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
            document.documentElement.classList.toggle("dark", prefersDark);
          } else {
            document.documentElement.classList.toggle("dark", mode === "dark");
          }
        }
      })
      .catch(() => {});
  }, []);

  // Per Wails v3 docs: common:ApplicationStarted fires after all
  // ServiceStartup hooks complete — data is guaranteed available.
  useWailsEvent("common:ApplicationStarted", () => refreshRecent());

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
    setSamplesDismissed(true);
    api.dismissSamples().catch(() => {});
  }, []);

  return { recentFiles, samplesDismissed, refreshRecent, dismissSamples };
}
