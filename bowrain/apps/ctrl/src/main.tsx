import "./app.css";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { QueryClient } from "@tanstack/react-query";
import { loadTranslations } from "@neokapi/kapi-react/runtime";
import { router } from "./routes";
import { initAnalytics, capturePageview } from "./analytics";

initAnalytics();
// One pageview per navigation (initial load included), carrying the matched
// route pattern rather than the concrete URL.
router.subscribe("onResolved", () => {
  const matches = router.state.matches;
  const leaf = matches[matches.length - 1];
  if (leaf) capturePageview(leaf.routeId);
});

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

// Activate the persisted UI locale (same key the shared Bowrain app writes)
// before first render so no store subscription is needed. Untranslated or
// missing catalogs fall back to the English source — never a blocker.
async function bootstrap() {
  const locale = localStorage.getItem("bowrain.ui-locale");
  if (locale && locale !== "en") {
    await loadTranslations(locale, `/translations/${locale}.json`).catch(() => {});
  }
  createRoot(document.getElementById("root")!).render(
    <RouterProvider router={router} context={{ queryClient }} />,
  );
}

void bootstrap();
