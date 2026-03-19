import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, TooltipProvider } from "@neokapi/ui";
import { QueryClientProvider } from "@tanstack/react-query";
import type { RouterContext } from ".";
import { AdminSidebar } from "../components/AdminSidebar";
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
            <div className="flex h-screen bg-background text-foreground">
              <AdminSidebar />
              <div className="flex flex-1 flex-col min-w-0">
                <header className="flex h-12 shrink-0 items-center justify-between border-b px-6">
                  <h1 className="text-sm font-semibold">Bowrain Control Plane</h1>
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
                </header>
                <main className="flex-1 overflow-auto p-6">
                  <Outlet />
                </main>
              </div>
            </div>
          ) : (
            <Outlet />
          )}
        </TooltipProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
