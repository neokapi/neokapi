// The server the sign-in screen signs in to: the build's default, or one the
// person chose in the server sheet. The choice is kept in the keychain beside
// the session, so it outlives a sign-out.
import * as SecureStore from "expo-secure-store";
import { useSyncExternalStore } from "react";

import { defaultServerUrl } from "./config";

const SERVER_KEY = "bowrain.server";

/**
 * A server URL as typed, made canonical: trimmed, `https://` added when no
 * scheme is given, trailing slashes dropped. Anything that is not an http or
 * https URL with a host is null.
 */
export function normalizeServerUrl(input: string): string | null {
  let s = input.trim();
  if (s === "") return null;
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) s = `https://${s}`;
  const m = /^(https?):\/\/([^/?#\s]+)([^?#\s]*)$/i.exec(s);
  if (!m) return null;
  const host = m[2];
  if (host.startsWith(":") || host.endsWith(":") || host.includes("@")) return null;
  return `${m[1].toLowerCase()}://${host}${m[3]}`.replace(/\/+$/, "");
}

/** The host a URL names, for display: `app.bowrain.cloud`, `localhost:8080`. */
export function serverHost(url: string): string {
  return url.replace(/^[a-z]+:\/\//i, "").replace(/\/.*$/, "");
}

let chosen: string | null = null;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

/** Read the stored choice once at start-up. */
export async function loadServerChoice(): Promise<void> {
  try {
    chosen = normalizeServerUrl((await SecureStore.getItemAsync(SERVER_KEY)) ?? "");
  } catch {
    chosen = null;
  }
  emit();
}

/** Keep a server, or forget the choice with null. The default is never stored. */
export async function setServerChoice(url: string | null): Promise<void> {
  const next = url === null ? null : normalizeServerUrl(url);
  chosen = next === defaultServerUrl ? null : next;
  if (chosen) await SecureStore.setItemAsync(SERVER_KEY, chosen);
  else await SecureStore.deleteItemAsync(SERVER_KEY);
  emit();
}

export function currentServer(): string {
  return chosen ?? defaultServerUrl;
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

/** The server to sign in to, and whether it is the build's default. */
export function useServerChoice(): { server: string; isDefault: boolean } {
  const server = useSyncExternalStore(subscribe, currentServer);
  return { server, isDefault: server === defaultServerUrl };
}
