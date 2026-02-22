import type { ReactNode } from "react";
import { cn } from "../lib/utils";
import { AnimatedBackgroundGlass } from "./ui/animated-background";
import { SidebarGlass } from "./ui/sidebar";
import { AppSidebar, type AppSidebarProps } from "./AppSidebar";

export interface AppShellProps<V extends string = string>
  extends Omit<AppSidebarProps<V>, "collapsed" | "onCollapsedChange" | "collapsedWidth"> {
  /** Controlled collapsed state. true = sidebar collapsed (icon mode). */
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /** Slot rendered above the main content (e.g. TopBar or Header). */
  headerSlot?: ReactNode;
  /** Main content. */
  children: ReactNode;
  /** Extra className on the content area. */
  contentClassName?: string;
}

export function AppShell<V extends string = string>({
  collapsed,
  onCollapsedChange,
  headerSlot,
  children,
  contentClassName,
  ...sidebarProps
}: AppShellProps<V>) {
  return (
    <>
      <AnimatedBackgroundGlass />
      <SidebarGlass.Provider
        open={!collapsed}
        onOpenChange={(open: boolean) => onCollapsedChange(!open)}
        collapsible="icon"
        keyboardShortcut={false}
        style={{
          "--sidebar-width": "220px",
          "--sidebar-width-icon": "60px",
        } as React.CSSProperties}
      >
        <AppSidebar collapsed={collapsed} onCollapsedChange={onCollapsedChange} {...sidebarProps} />
        <SidebarGlass.Inset className="bg-transparent">
          {headerSlot}
          <div className={cn("flex-1 p-6 flex flex-col min-h-0", contentClassName)}>
            {children}
          </div>
        </SidebarGlass.Inset>
      </SidebarGlass.Provider>
    </>
  );
}
