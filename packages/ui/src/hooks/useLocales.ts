import { useState, useEffect, useCallback } from "react";
import { useApi } from "../context/ApiContext";
import type { LocaleInfo } from "../types/api";

export function useLocales() {
  const api = useApi();
  const [locales, setLocales] = useState<LocaleInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api.getKnownLocales()
      .then((r) => setLocales(r))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false));
  }, [api]);

  const getDisplayName = useCallback(
    (code: string): string => {
      const info = locales.find((l) => l.code === code);
      return info ? info.display_name : code;
    },
    [locales],
  );

  return { locales, loading, error, getDisplayName };
}
