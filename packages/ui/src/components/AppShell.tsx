import type { ReactNode } from "react";
import { cn } from "../lib/utils";
import { AnimatedBackgroundGlass } from "./ui/animated-background";
import { SidebarGlass } from "./ui/sidebar";
import { AppSidebar, type AppSidebarProps } from "./AppSidebar";
import { BreadcrumbProvider, useBreadcrumb } from "../context/BreadcrumbContext";

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

function HeaderBar({ headerSlot }: { headerSlot?: ReactNode }) {
  const breadcrumb = useBreadcrumb();
  return (
    <div className="flex flex-wrap items-center shrink-0">
      {breadcrumb && (
        <div className="flex items-center h-10 pl-4">{breadcrumb}</div>
      )}
      <div className="flex-1 min-w-0" />
      {headerSlot}
    </div>
  );
}

export function AppShell<V extends string = string>({
  collapsed,
  onCollapsedChange,
  headerSlot,
  children,
  contentClassName,
  ...sidebarProps
}: AppShellProps<V>) {
  const hasTopBar = (sidebarProps.topSpacer ?? 0) > 0;

  return (
    <>
      <AnimatedBackgroundGlass />
      <SidebarGlass.Provider
        open={!collapsed}
        onOpenChange={(open: boolean) => onCollapsedChange(!open)}
        collapsible="icon"
        keyboardShortcut={false}
      >
        <BreadcrumbProvider>
          <div
            className="relative z-10 flex flex-col h-screen overflow-hidden"
            style={{
              "--sidebar-width": "220px",
              "--sidebar-width-icon": "60px",
            } as React.CSSProperties}
          >
            {hasTopBar && (
              <div
                className="shrink-0 flex items-center h-12 border-b border-border glass-surface bg-card/80"
                style={{ "--wails-draggable": "drag" } as React.CSSProperties}
              >
                <div
                  className="shrink-0 transition-[width] duration-300 ease-in-out"
                  style={{ width: collapsed ? "var(--sidebar-width-icon)" : "var(--sidebar-width)" }}
                />
                <div className="flex-1 min-w-0">
                  <HeaderBar headerSlot={headerSlot} />
                </div>
              </div>
            )}
            <div className="flex flex-1 min-h-0 overflow-hidden">
              <AppSidebar
                collapsed={collapsed}
                onCollapsedChange={onCollapsedChange}
                topSpacer={hasTopBar ? 0 : sidebarProps.topSpacer}
                {...sidebarProps}
              />
              <SidebarGlass.Inset className="bg-transparent min-h-0">
                {!hasTopBar && <HeaderBar headerSlot={headerSlot} />}
                <div className={cn("flex-1 p-6 flex flex-col min-h-0", contentClassName)}>
                  {children}
                </div>
              </SidebarGlass.Inset>
            </div>
          </div>
        </BreadcrumbProvider>
      </SidebarGlass.Provider>
    </>
  );
}
