"use client";

/**
 * @neokapi/i18n-react/ship/react — React binding for the ship-aware picker.
 *
 * A thin hook over `loadShipStatus` + `languagePickerModel`: it fetches the
 * manifest once and returns the picker render model for the app's locale list.
 * The transform is pure, so consumers can also render entirely from the headless
 * `languagePickerModel` without this hook. Marked "use client" because it uses
 * React state/effects — under RSC it must run on the client.
 */

import { useEffect, useState } from "react";

import {
  languagePickerModel,
  loadShipStatus,
  type LocaleInput,
  type PickerLocale,
  type ShipStatus,
} from "./index.ts";

export {
  languagePickerModel,
  loadShipStatus,
  type LocaleBadge,
  type LocaleInput,
  type PickerLocale,
  type ShipEntry,
  type ShipStatus,
} from "./index.ts";

/**
 * Load the ship manifest and return the picker render model for `locales`. On
 * first render (and until the manifest resolves) the model is computed from an
 * empty manifest, so every locale shows unbadged; it repaints once ship.json
 * loads. A missing/malformed manifest stays empty (the loader tolerates it).
 *
 * `url` defaults to `/ship.json`. The transform is pure and cheap, so it runs
 * on every render — no memoization needed for a picker-sized locale list.
 */
export function useShipStatus(
  locales: ReadonlyArray<string | LocaleInput>,
  url?: string,
): PickerLocale[] {
  const [status, setStatus] = useState<ShipStatus>({});
  useEffect(() => {
    let alive = true;
    void loadShipStatus(url).then((s) => {
      if (alive) setStatus(s);
    });
    return () => {
      alive = false;
    };
  }, [url]);
  return languagePickerModel(status, locales);
}
