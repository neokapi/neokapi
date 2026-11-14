// The project the person last opened, which the Activity tab shows until
// they pick another. Kept on the device; signing out forgets it.
import Storage from "expo-sqlite/kv-store";
import { useSyncExternalStore } from "react";

export interface RecentProject {
  ws: string;
  project: string;
  name?: string;
}

const KEY = "bowrain.recent-project";

let current: RecentProject | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

/** Read a stored value as a project, or null. */
export function parseRecentProject(raw: string | null): RecentProject | null {
  if (!raw) return null;
  try {
    const p = JSON.parse(raw) as Partial<RecentProject> | null;
    if (!p || typeof p.ws !== "string" || typeof p.project !== "string") return null;
    return { ws: p.ws, project: p.project, name: typeof p.name === "string" ? p.name : undefined };
  } catch {
    return null;
  }
}

/** Read the stored project once at start-up. */
export async function loadRecentProject(): Promise<void> {
  try {
    current = parseRecentProject(await Storage.getItem(KEY));
  } catch {
    current = null;
  }
  emit();
}

/** Remember a project, or forget it with null. */
export function setRecentProject(next: RecentProject | null): void {
  if (
    next?.ws === current?.ws &&
    next?.project === current?.project &&
    next?.name === current?.name
  ) {
    return;
  }
  current = next;
  emit();
  const write = next ? Storage.setItem(KEY, JSON.stringify(next)) : Storage.removeItem(KEY);
  void write.catch(() => undefined);
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useRecentProject(): RecentProject | null {
  return useSyncExternalStore(subscribe, () => current);
}
