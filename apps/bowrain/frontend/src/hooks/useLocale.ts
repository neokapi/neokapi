import { useState, useEffect, useCallback } from "react";
import type { LocaleInfo } from "../types/api";

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore – generated .js bindings outside the TS project root
import * as Backend from "../../bindings/github.com/gokapi/gokapi/apps/bowrain/backend/app.js";

let cachedLocales: LocaleInfo[] | null = null;

export function useLocales() {
  const [locales, setLocales] = useState<LocaleInfo[]>(cachedLocales || []);
  const [loading, setLoading] = useState(!cachedLocales);

  useEffect(() => {
    if (cachedLocales) return;
    Backend.GetKnownLocales()
      .then((result: LocaleInfo[]) => {
        cachedLocales = result || [];
        setLocales(cachedLocales);
      })
      .catch(() => {
        // Fallback: empty list, users can still type codes
        cachedLocales = [];
        setLocales([]);
      })
      .finally(() => setLoading(false));
  }, []);

  const getDisplayName = useCallback(
    (code: string): string => {
      if (!code) return "";
      const found = (cachedLocales || locales).find(
        (l) => l.code.toLowerCase() === code.toLowerCase(),
      );
      return found ? found.display_name : code;
    },
    [locales],
  );

  return { locales, getDisplayName, loading };
}
