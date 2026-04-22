import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import React from "react";
import ReactDOM from "react-dom/client";
import { loadTranslations } from "@neokapi/kapi-react/runtime";
import App from "./App";
import { api } from "./hooks/useApi";

// Dev-only runtime pseudo wiring. Import a small subpath only in
// development so the accent map doesn't ship in production. Exposes a
// console handle so developers can toggle without a rebuild:
//
//   window.kapi.pseudo({})                      // on, defaults
//   window.kapi.pseudo({ expansion: 30 })       // on, +30% padding
//   window.kapi.pseudo(null)                    // off
//
// State is persisted to localStorage for the session so HMR reloads
// and devtools refreshes keep the current pseudo config.
async function wireDevPseudo() {
  if (!import.meta.env.DEV) return;
  const { setPseudoMode, getPseudoMode } = await import("@neokapi/kapi-react/runtime/pseudo");
  const PERSIST_KEY = "kapi.dev.pseudo";
  const saved = localStorage.getItem(PERSIST_KEY);
  if (saved) {
    try {
      setPseudoMode(JSON.parse(saved) as Parameters<typeof setPseudoMode>[0]);
    } catch {
      localStorage.removeItem(PERSIST_KEY);
    }
  }
  // Wrap so console toggles also persist.
  const toggle = (cfg: Parameters<typeof setPseudoMode>[0]) => {
    setPseudoMode(cfg);
    if (cfg === null) localStorage.removeItem(PERSIST_KEY);
    else localStorage.setItem(PERSIST_KEY, JSON.stringify(cfg));
  };
  (window as unknown as { kapi?: Record<string, unknown> }).kapi = {
    ...(window as unknown as { kapi?: Record<string, unknown> }).kapi,
    pseudo: toggle,
    getPseudoMode,
  };
}

async function bootstrap() {
  await wireDevPseudo();

  const lang = (await api.getUILanguage()) ?? "en";
  if (lang && lang !== "en") {
    try {
      await loadTranslations(lang, `/translations/${lang}.json`);
    } catch {
      // Translations missing — fall back to source text.
    }
  }

  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
}

void bootstrap();
