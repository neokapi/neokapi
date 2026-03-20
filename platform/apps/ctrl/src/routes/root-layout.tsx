import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, TooltipProvider } from "@neokapi/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import type { RouterContext } from ".";
import { CtrlShell } from "../components/CtrlShell";
import { isAuthenticated, getAdminUser, logout } from "../auth";

export function RootLayout() {
  const { queryClient } = useRouteContext({ strict: false }) as RouterContext;
  const authenticated = isAuthenticated();
  const adminUser = getAdminUser();

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          {authenticated ? (
            <CtrlShell
              headerSlot={
                <div className="flex items-center gap-3">
                  {adminUser && (
                    <span className="text-sm text-muted-foreground">{adminUser.email}</span>
                  )}
                  <button
                    onClick={logout}
                    className="text-sm text-muted-foreground hover:text-foreground cursor-pointer"
                  >
                    Sign out
                  </button>
                </div>
              }
            >
              <Outlet />
            </CtrlShell>
          ) : (
            <Outlet />
          )}
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
