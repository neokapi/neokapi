import { createRoot } from "react-dom/client";
import { StrictMode } from "react";
import { ThemeProvider } from "@neokapi/ui";
// Import the analytics module directly (not the package index) so the theme
// bundle never pulls the docs diagram components or their CSS.
import { captureDocsEvent, initDocsAnalytics } from "@neokapi/docs-shared/analytics.ts";
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

// Cookieless analytics (memory persistence, DNT respected): auth pages must
// never write cookies or storage for analytics. Keyless builds (self-host,
// local dev) are inert. Only the two page-view events below are emitted —
// auth *outcome* events fire post-auth in the app, keeping the theme dumb.
if (window.kcContext) {
  initDocsAnalytics({
    key: import.meta.env.VITE_POSTHOG_KEY,
    host: import.meta.env.VITE_POSTHOG_HOST,
    surface: "keycloak",
    environment: import.meta.env.PROD ? "prod" : "dev",
  });
  const { pageId } = window.kcContext;
  if (pageId === "login.ftl" || pageId === "login-username.ftl") {
    captureDocsEvent("login_page_viewed", { template: pageId });
  } else if (pageId === "register.ftl") {
    captureDocsEvent("register_page_viewed", { template: pageId });
  }
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      {!window.kcContext ? <h1>No Keycloak Context</h1> : <KcPage kcContext={window.kcContext} />}
    </ThemeProvider>
  </StrictMode>,
);
