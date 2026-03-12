import { Outlet } from "@tanstack/react-router";
import { AnimatedBackgroundGlass } from "@neokapi/ui";

/** Auth layout — renders auth pages without sidebar/workspace chrome. */
export function AuthLayout() {
  return (
    <>
      <AnimatedBackgroundGlass />
      <div className="relative z-10">
        <Outlet />
      </div>
    </>
  );
}
