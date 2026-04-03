import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, TooltipProvider, AccountMenu } from "@neokapi/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import type { RouterContext } from ".";
import { CtrlShell } from "../components/CtrlShell";
import { isAuthenticated, getAdminUser, logout } from "../auth";

export function RootLayout() {
  const { queryClient } = useRouteContext({ strict: false }) as RouterContext;
  const authenticated = isAuthenticated();
  const adminUser = getAdminUser();

  const user = adminUser
    ? { id: "", email: adminUser.email, name: adminUser.name, avatar_url: "" }
    : null;

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <TooltipProvider>
          {authenticated ? (
            <CtrlShell
              headerSlot={user && <AccountMenu user={user} onSignOut={logout} variant="icon" />}
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
