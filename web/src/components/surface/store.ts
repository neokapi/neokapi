import { useSyncExternalStore } from "react";

// The "surface" a reader works in: the CLI, Kapi Desktop, or both at once.
// A single global preference (navbar SurfaceToggle) drives which dual-mode
// content blocks (<Cli> / <Desktop>) are shown, via `html[data-surface]`.
export type Surface = "cli" | "desktop" | "both";

export const SURFACES: readonly Surface[] = ["cli", "desktop", "both"];
export const SURFACE_KEY = "kapi-surface";

export function isSurface(v: unknown): v is Surface {
  return typeof v === "string" && (SURFACES as readonly string[]).includes(v);
}

// Initial value comes from the DOM attribute the no-flash head script set from
// localStorage before paint (see the `surface-preload` plugin in the config), so
// the toggle's active state and the content visibility agree from the first
// frame. Falls back to "both" on the server and when nothing is stored.
let surface: Surface = (() => {
  if (typeof document === "undefined") return "both";
  const d = document.documentElement.dataset.surface;
  return isSurface(d) ? d : "both";
})();

// How many dual-mode blocks are mounted on the current page. The navbar toggle
// renders only when this is > 0 — i.e. only on pages that actually have a dual
// CLI/Desktop split.
let blockCount = 0;

const surfaceSubs = new Set<() => void>();
const presenceSubs = new Set<() => void>();
const notify = (subs: Set<() => void>) => subs.forEach((cb) => cb());

export function getSurface(): Surface {
  return surface;
}

export function setSurface(next: Surface): void {
  if (next === surface) return;
  surface = next;
  if (typeof document !== "undefined") {
    document.documentElement.dataset.surface = next;
    try {
      localStorage.setItem(SURFACE_KEY, next);
    } catch {
      /* private mode / storage disabled — preference is session-only */
    }
  }
  notify(surfaceSubs);
}

/** Register a mounted dual-mode block; returns an unregister cleanup. */
export function registerBlock(): () => void {
  blockCount += 1;
  notify(presenceSubs);
  return () => {
    blockCount -= 1;
    notify(presenceSubs);
  };
}

function hasDual(): boolean {
  return blockCount > 0;
}

export function useSurface(): Surface {
  return useSyncExternalStore(
    (cb) => {
      surfaceSubs.add(cb);
      return () => surfaceSubs.delete(cb);
    },
    getSurface,
    () => "both",
  );
}

export function useHasDual(): boolean {
  return useSyncExternalStore(
    (cb) => {
      presenceSubs.add(cb);
      return () => presenceSubs.delete(cb);
    },
    hasDual,
    () => false,
  );
}
