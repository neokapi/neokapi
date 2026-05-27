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
document.documentElement.classList.toggle("dark", params.get("theme") === "dark");

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
