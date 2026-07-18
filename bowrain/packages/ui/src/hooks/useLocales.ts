import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import type { LocaleInfo } from "../types/api";

const EMPTY_LOCALES: LocaleInfo[] = [];

/**
 * The adapter's known-locale catalog, cached for the session. The list is
 * static server metadata (GET /api/v1/info on the web adapter), so it is
 * fetched once per QueryClient and shared by every consumer — previously each
 * mounting component issued its own uncached /info request.
 */
export function useLocales() {
  const api = useApi();
  const {
    data: locales = EMPTY_LOCALES,
    isPending: loading,
    error,
  } = useQuery({
    queryKey: ["known-locales"],
    queryFn: () => api.getKnownLocales(),
    staleTime: Infinity,
    gcTime: Infinity,
  });

  const getDisplayName = useCallback(
    (code: string): string => {
      const info = locales.find((l) => l.code === code);
      return info ? info.display_name : code;
    },
    [locales],
  );

  return { locales, loading, error: error ? error.message : null, getDisplayName };
}
