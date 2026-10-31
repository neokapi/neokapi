// The port the Bowrain preview draws the project screens through. It reads
// and never acts: it carries the four reads and no decide, assign, fix or
// check, so the screens draw no control that changes the project.

import { useMemo } from "react";
import { t } from "@neokapi/i18n-react/runtime";
import type { ProjectScreensPort, ScreenTarget } from "@neokapi/project-screens";
import { api } from "../hooks/useApi";

/** A read that answers null, with no backend behind it, rejects instead. */
async function required<T>(read: Promise<T | null>): Promise<T> {
  const value = await read;
  if (value === null) throw new Error(t("This project could not be read from the desktop"));
  return value;
}

/**
 * A read-only port over the tab's own project, read through the bindings the
 * tab's Map, Reader and Decide read through.
 */
export function localPreviewPort(tabID: string): ProjectScreensPort {
  return {
    mapSummary: () => required(api.mapSummary(tabID)),
    readerFile: (path) => required(api.readerFile(tabID, path)),
    decideView: () => required(api.rulesDecideView(tabID)),
    keptRules: () => required(api.keptRules(tabID)),
  };
}

/** The local preview port for a tab, the same object while the tab is. */
export function useLocalPreviewPort(tabID: string): ProjectScreensPort {
  return useMemo(() => localPreviewPort(tabID), [tabID]);
}

/** The targets a preview opens: the Map, the Reader and the Rules tab. */
export function previewOpens(target: ScreenTarget): boolean {
  switch (target.kind) {
    case "map":
    case "reader":
    case "rules":
    case "kept":
      return true;
    default:
      return false;
  }
}
