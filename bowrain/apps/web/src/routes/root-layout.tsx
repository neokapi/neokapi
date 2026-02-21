import { Outlet } from "@tanstack/react-router";
import { ThemeProvider, ApiProvider, AuthProvider, WorkspaceProvider } from "@gokapi/ui";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      retry: 1,
    },
  },
});

export function RootLayout() {
  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <ApiProvider adapter={api}>
          <AuthProvider>
            <WorkspaceProvider>
              <Outlet />
            </WorkspaceProvider>
          </AuthProvider>
        </ApiProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
