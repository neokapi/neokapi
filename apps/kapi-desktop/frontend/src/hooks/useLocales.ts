import { useState, useEffect, useCallback } from "react";
import type { LocaleInfo } from "@neokapi/ui-primitives";
import { api } from "./useApi";

/**
 * Fetches the known locale list from the Wails backend and provides
 * display name lookup. Caches the result for the component lifetime.
 */
export function useLocales() {
  const [locales, setLocales] = useState<LocaleInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    api
      .getKnownLocales()
      .then((raw) => {
        if (raw) {
          setLocales(raw.map((l) => ({ code: l.code, displayName: l.display_name })));
        }
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const getDisplayName = useCallback(
    (code: string): string => {
      const info = locales.find((l) => l.code === code);
      return info ? info.displayName : code;
    },
    [locales],
  );

  return { locales, loading, getDisplayName };
}
