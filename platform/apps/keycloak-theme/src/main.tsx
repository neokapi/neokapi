import { createRoot } from "react-dom/client";
import { StrictMode } from "react";
import { ThemeProvider } from "@neokapi/ui/context/ThemeContext";
import { KcPage } from "./kc.gen";

// Uncomment to test a specific page with `npm run dev`:
/*
import { getKcContextMock } from "./login/KcPageStory";

if (import.meta.env.DEV) {
    window.kcContext = getKcContextMock({
        pageId: "login.ftl",
        overrides: {}
    });
}
*/

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      {!window.kcContext ? <h1>No Keycloak Context</h1> : <KcPage kcContext={window.kcContext} />}
    </ThemeProvider>
  </StrictMode>,
);
