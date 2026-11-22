// The appearance the person chose on the account screen: the system's, or
// light or dark whatever the system says. Kept on the device, and applied
// through Uniwind, which also sets the native interface style so headers,
// sheets and alerts follow.
import Storage from "expo-sqlite/kv-store";
import { useSyncExternalStore } from "react";
import { Uniwind } from "uniwind";

export type Appearance = "system" | "light" | "dark";

export const APPEARANCES: readonly Appearance[] = ["system", "light", "dark"];

const KEY = "bowrain.appearance";

let current: Appearance = "system";
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

function isAppearance(v: unknown): v is Appearance {
  return v === "system" || v === "light" || v === "dark";
}

/** Read the stored choice once at start-up and apply it. */
export async function loadAppearance(): Promise<void> {
  try {
    const stored = await Storage.getItem(KEY);
    if (isAppearance(stored)) current = stored;
  } catch {
    current = "system";
  }
  if (current !== "system") Uniwind.setTheme(current);
  emit();
}

export async function setAppearance(next: Appearance): Promise<void> {
  current = next;
  Uniwind.setTheme(next);
  emit();
  try {
    if (next === "system") await Storage.removeItem(KEY);
    else await Storage.setItem(KEY, next);
  } catch {
    // The choice holds for this run; the next start uses the system's.
  }
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useAppearance(): Appearance {
  return useSyncExternalStore(subscribe, () => current);
}
