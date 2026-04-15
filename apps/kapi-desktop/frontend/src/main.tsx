import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import React from "react";
import ReactDOM from "react-dom/client";
import { loadTranslations } from "@neokapi/react/runtime";
import App from "./App";
import { api } from "./hooks/useApi";

async function bootstrap() {
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
