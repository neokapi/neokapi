import { useState, useEffect, useCallback } from "react";
import { api } from "./useApi";

let cachedHome: string | null = null;

/**
 * Returns a function that replaces the user's home directory with ~/ in paths.
 * Fetches the home directory from the backend once and caches it.
 * Works on macOS, Linux, and Windows.
 */
export function useShortenHome(): (path: string) => string {
  const [home, setHome] = useState(cachedHome);

  useEffect(() => {
    if (cachedHome) return;
    api.getHomeDir().then((h) => {
      if (h) {
        cachedHome = h;
        setHome(h);
      }
    });
  }, []);

  return useCallback(
    (path: string): string => {
      if (!home || !path) return path;
      // Handle both forward and back slashes (Windows).
      const normalized = path.replace(/\\/g, "/");
      const normalizedHome = home.replace(/\\/g, "/");
      if (normalized.startsWith(normalizedHome)) {
        return "~" + normalized.slice(normalizedHome.length);
      }
      return path;
    },
    [home],
  );
}
