import "./app.css";
import { createRoot } from "react-dom/client";
import { RouterProvider } from "@tanstack/react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
      staleTime: 60_000,
      retry: 1,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <QueryClientProvider client={queryClient}>
    <RouterProvider router={router} context={{ queryClient }} />
  </QueryClientProvider>,
);
