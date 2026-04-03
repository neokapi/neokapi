import { RouterProvider } from "@tanstack/react-router";
import { QueryClient } from "@tanstack/react-query";
import { router } from "./routes";
import { api } from "./api";
import { initPostHog } from "./posthog";

initPostHog();

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

export function App() {
  return <RouterProvider router={router} context={{ queryClient, api }} />;
}
