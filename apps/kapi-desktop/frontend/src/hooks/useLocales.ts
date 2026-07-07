import { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { LocaleInfo } from "@neokapi/ui-primitives";
import { api } from "./useApi";
import { qk } from "../lib/queryKeys";

/**
 * Fetches the known locale list from the Wails backend and provides
 * display name lookup. react-query caches the result across the app.
 */
export function useLocales() {
  const query = useQuery({
    queryKey: qk.knownLocales(),
    queryFn: () => api.getKnownLocales(),
    staleTime: Infinity,
  });

  const locales = useMemo<LocaleInfo[]>(
    () => (query.data ?? []).map((l) => ({ code: l.code, displayName: l.display_name })),
    [query.data],
  );

  const getDisplayName = useCallback(
    (code: string): string => {
      const info = locales.find((l) => l.code === code);
      return info ? info.displayName : code;
    },
    [locales],
  );

  return { locales, loading: query.isLoading, getDisplayName };
}
