import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, ApiProvider, AuthProvider, WorkspaceProvider, TooltipProvider } from "@neokapi/ui";
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
              <TooltipProvider>
                <Outlet />
              </TooltipProvider>
            </WorkspaceProvider>
          </AuthProvider>
        </ApiProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
