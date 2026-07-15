import { Outlet, useRouteContext } from "@tanstack/react-router";
import { ThemeProvider, TooltipProvider, AccountMenu } from "@neokapi/ui";
import { QueryClientProvider, useQuery } from "@tanstack/react-query";
import type { RouterContext } from ".";
import { CtrlShell } from "../components/CtrlShell";
import { fetchAdminSession, ADMIN_SESSION_QUERY_KEY, logout } from "../auth";

function Shell() {
  const { data: session } = useQuery({
    queryKey: ADMIN_SESSION_QUERY_KEY,
    queryFn: fetchAdminSession,
  });

  const user = session
    ? { id: "", email: session.email, name: session.name, avatar_url: "" }
    : null;

  return (
    <ThemeProvider>
      <TooltipProvider>
        {user ? (
          <CtrlShell
            headerSlot={<AccountMenu user={user} onSignOut={() => void logout()} variant="icon" />}
          >
            <Outlet />
          </CtrlShell>
        ) : (
          <Outlet />
        )}
      </TooltipProvider>
    </ThemeProvider>
  );
}

export function RootLayout() {
  const { queryClient } = useRouteContext({ strict: false }) as RouterContext;

  // useQuery must run inside the provider, so the shell (which reads the admin
  // session) is a child component of QueryClientProvider.
  return (
    <QueryClientProvider client={queryClient}>
      <Shell />
    </QueryClientProvider>
  );
}
