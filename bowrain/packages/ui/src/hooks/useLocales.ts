import { useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import { useApi } from "../context/ApiContext";
import { resolveLocaleName } from "@neokapi/ui-primitives";
import type { LocaleInfo } from "../types/api";

const EMPTY_LOCALES: LocaleInfo[] = [];

/**
 * The adapter's known-locale catalog, cached for the session. The list is
 * static server metadata (GET /api/v1/info on the web adapter), so it is
 * fetched once per QueryClient and shared by every consumer, so a mounting
 * component never issues its own uncached /info request.
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

  // The catalog is a curated list of major tags, so a project target such as
  // "fr-FR" is routinely absent from it. Falling back to the code itself put
  // raw BCP-47 in front of readers wherever a picker or a label asked for a
  // name; CLDR knows every well-formed tag, so ask it before giving up.
  const getDisplayName = useCallback(
    (code: string): string => {
      const info = locales.find((l) => l.code === code);
      return info ? info.display_name : resolveLocaleName(code);
    },
    [locales],
  );

  return { locales, loading, error: error ? error.message : null, getDisplayName };
}
