import { createRoot } from "react-dom/client";
import { StrictMode } from "react";
import { ThemeProvider } from "@neokapi/ui/context/ThemeContext";
import { KcPage } from "./kc.gen";

// In dev mode, allow rendering any page via ?kcPageId= query parameter.
// This enables E2E testing of all theme pages with mock contexts.
if (import.meta.env.DEV && !window.kcContext) {
  const params = new URLSearchParams(window.location.search);
  const pageId = params.get("kcPageId");
  if (pageId) {
    const { getKcContextMock } = await import("./login/KcPageStory");
    window.kcContext = getKcContextMock({
      pageId: pageId as any,
      overrides: {},
    });
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      {!window.kcContext ? <h1>No Keycloak Context</h1> : <KcPage kcContext={window.kcContext} />}
    </ThemeProvider>
  </StrictMode>,
);
