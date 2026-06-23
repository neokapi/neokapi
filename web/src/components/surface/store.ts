import { useSyncExternalStore } from "react";

// The two surfaces a reader can show: the CLI and Kapi Desktop. Each is an
// independent on/off toggle (no separate "both") — with both on, dual-mode
// content shows both variants; with one on, only that one. At least one is
// always on. A single global preference drives `html[data-surface]`, which the
// CSS uses to show/hide <Cli> / <Desktop> blocks site-wide.
export type Surface = "cli" | "desktop";
export interface Enabled {
  cli: boolean;
  desktop: boolean;
}

export const SURFACE_KEY = "kapi-surface";

// Stable references so useSyncExternalStore snapshots compare by identity.
const BOTH: Enabled = { cli: true, desktop: true };
const CLI_ONLY: Enabled = { cli: true, desktop: false };
const DESKTOP_ONLY: Enabled = { cli: false, desktop: true };

/** The three legal states, encoded the way they persist + drive the CSS. */
function derive(e: Enabled): "cli" | "desktop" | "both" {
  if (e.cli && e.desktop) return "both";
  return e.cli ? "cli" : "desktop";
}
function fromStored(v: string | null | undefined): Enabled {
  if (v === "cli") return CLI_ONLY;
  if (v === "desktop") return DESKTOP_ONLY;
  return BOTH;
}

// Initial value from the DOM attribute the no-flash head script set from
// localStorage before paint (see the `surface-preload` plugin), so content
// visibility and the toggle agree from the first frame.
let enabled: Enabled = (() => {
  if (typeof document === "undefined") return BOTH;
  return fromStored(document.documentElement.dataset.surface);
})();

// Mounted dual-mode block count on the current page — the floating control only
// shows when this is > 0.
let blockCount = 0;

const surfaceSubs = new Set<() => void>();
const presenceSubs = new Set<() => void>();
const notify = (subs: Set<() => void>) => subs.forEach((cb) => cb());

function apply(): void {
  const v = derive(enabled);
  if (typeof document !== "undefined") {
    document.documentElement.dataset.surface = v;
    try {
      localStorage.setItem(SURFACE_KEY, v);
    } catch {
      /* private mode / storage disabled — preference is session-only */
    }
  }
  notify(surfaceSubs);
}

export function toggleSurface(kind: Surface): void {
  const next: Enabled = {
    cli: kind === "cli" ? !enabled.cli : enabled.cli,
    desktop: kind === "desktop" ? !enabled.desktop : enabled.desktop,
  };
  if (!next.cli && !next.desktop) return; // always keep at least one surface on
  enabled = next.cli && next.desktop ? BOTH : next.cli ? CLI_ONLY : DESKTOP_ONLY;
  apply();
}

function getEnabled(): Enabled {
  return enabled;
}

export function useEnabled(): Enabled {
  return useSyncExternalStore(
    (cb) => {
      surfaceSubs.add(cb);
      return () => surfaceSubs.delete(cb);
    },
    getEnabled,
    () => BOTH,
  );
}

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
