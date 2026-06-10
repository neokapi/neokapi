import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "@fontsource-variable/outfit";
import "@fontsource-variable/dm-sans";
import "@fontsource-variable/source-code-pro";
import "./index.css";
import { loadTranslations } from "@neokapi/kapi-react/runtime";
import { resolveLocale } from "./lib/locale";

async function bootstrap() {
  const locale = resolveLocale();
  document.documentElement.lang = locale;
  if (locale !== "en") {
    try {
      await loadTranslations(locale, `${import.meta.env.BASE_URL}translations/${locale}.json`);
    } catch {
      // Dictionary missing or unreachable — fall back to the source text.
    }
  }
  // Import the app only after the dictionary is active so module-scope
  // t() data (feature cards, tab labels, …) resolves in the right locale.
  const { default: App } = await import("./App");
  createRoot(document.getElementById("root")!).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}

void bootstrap();
