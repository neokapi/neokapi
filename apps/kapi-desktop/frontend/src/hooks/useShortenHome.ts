import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";

/**
 * Returns a function that replaces the user's home directory with ~/ in paths.
 * The home directory is fetched once via react-query (cached forever) and works
 * on macOS, Linux, and Windows.
 */
export function useShortenHome(): (path: string) => string {
  const { data: home } = useQuery({
    queryKey: qk.homeDir(),
    queryFn: () => api.getHomeDir(),
    staleTime: Infinity,
  });

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
