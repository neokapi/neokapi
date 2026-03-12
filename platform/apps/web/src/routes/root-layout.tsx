import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, ApiProvider, AuthProvider, WorkspaceProvider } from "@gokapi/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import { api } from "../api";
import type { RouterContext } from ".";

export function RootLayout() {
  const { queryClient } = useRouteContext({ strict: false }) as RouterContext;

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
