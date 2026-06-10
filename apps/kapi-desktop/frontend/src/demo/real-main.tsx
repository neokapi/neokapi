/**
 * Real-app recording entry. Mounts the GENUINE {@link App} (App.tsx, the real
 * useApi bindings, the real pages) but installs a custom Wails transport that
 * forwards binding calls to the `wbridge` HTTP server (cmd/wbridge), which hosts
 * the real backend.App over HTTP and reads the real SQLite databases.
 *
 * This exists only so the walkthrough recorder can drive the real app in a
 * browser (the macOS Wails runtime is webview-only). Nothing is mocked: the
 * frontend, backend code, and data are all real — only the transport differs.
 *
 * `?theme=dark` records the dark palette; the bridge URL can be overridden with
 * VITE_WBRIDGE_URL.
 */
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "../index.css";
import React from "react";
import ReactDOM from "react-dom/client";
import { setTransport } from "@wailsio/runtime";
import idMap from "./wails-id-map.generated.json";
import App from "../App";

const BRIDGE = (import.meta.env.VITE_WBRIDGE_URL as string) || "http://localhost:5175/wbridge";
const ids = idMap as Record<string, string>;

// The Wails binding caller invokes transport.call(objectNames.Call=0, CallBinding=0,
// windowName, { "call-id", methodID, args }). Forward binding calls to wbridge by
// method name; everything else (events, dialogs, window) has no backend here.
setTransport({
  call: async (objectID: number, _method: number, _windowName: string, args: unknown) => {
    if (objectID !== 0) return null;
    const payload = args as { methodID?: number; args?: unknown[] } | null;
    const methodID = payload?.methodID;
    if (methodID == null) return null;
    const name = ids[String(methodID)];
    if (!name) throw new Error(`wbridge: unknown method id ${methodID}`);
    const res = await fetch(BRIDGE, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ method: name, args: payload?.args ?? [] }),
    });
    if (!res.ok) throw new Error(await res.text());
    const ct = res.headers.get("Content-Type") ?? "";
    return ct.includes("application/json") ? res.json() : res.text();
  },
});

const params = new URLSearchParams(window.location.search);

// Stream backend events (plugin install progress, flow:event, …) from wbridge
// and re-dispatch each into the Wails runtime, so useWailsEvent() fires exactly
// as it does in the native app. Without this, fire-and-forget backend work
// (e.g. plugin install) would never visibly complete during a recording.
const EVENTS_URL = BRIDGE.replace(/\/wbridge$/, "/wevents");
type WailsBridge = { dispatchWailsEvent?: (e: { name: string; data: unknown }) => void };
const es = new EventSource(EVENTS_URL);
es.onmessage = (ev) => {
  try {
    const { name, data } = JSON.parse(ev.data) as { name: string; data: unknown };
    (window as unknown as { _wails?: WailsBridge })._wails?.dispatchWailsEvent?.({ name, data });
  } catch {
    /* ignore malformed event frames */
  }
};

// Pin the recording palette. The recorder always loads `?theme=light|dark`, but
// the genuine app re-applies its persisted theme asynchronously on mount
// (useAppInit → getSettings), which can flip a dark recording back to light. A
// MutationObserver re-asserts the forced class whenever something clears it; the
// toggle is idempotent, so it never loops.
const forcedTheme = params.get("theme");
if (forcedTheme === "dark" || forcedTheme === "light") {
  const isDark = forcedTheme === "dark";
  const pin = () => document.documentElement.classList.toggle("dark", isDark);
  pin();
  new MutationObserver(pin).observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });
} else {
  document.documentElement.classList.toggle("dark", forcedTheme === "dark");
}

// UI locale for localized recordings. The native entry (main.tsx) boots the
// locale from api.getUILanguage(); the recorder instead honors `?lang=` so the
// walkthrough harness can record a localized UI (harness record-desktop.ts
// appends &lang=<locale> when --locale is set). Loading before mount keeps the
// first painted frame localized.
async function mount() {
  const lang = params.get("lang");
  if (lang && lang !== "en") {
    try {
      const { loadTranslations } = await import("@neokapi/kapi-react/runtime");
      await loadTranslations(lang, `/translations/${lang}.json`);
    } catch {
      // Translations missing — record with source text.
    }
  }
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}
void mount();
