import { Outlet } from "@tanstack/react-router";

/** Auth layout — renders auth pages without sidebar/workspace chrome. */
export function AuthLayout() {
  return <Outlet />;
}
