import { useState, useCallback, useEffect, type ReactNode } from "react";
import { cn } from "../lib/utils";
import { useIsMobile } from "../hooks/useIsMobile";
import { AnimatedBackgroundGlass } from "./ui/animated-background";
import { SidebarGlass } from "./ui/sidebar";
import { AppSidebar, type AppSidebarProps } from "./AppSidebar";
import { BreadcrumbProvider, useBreadcrumb } from "../context/BreadcrumbContext";
import { X, Menu } from "./icons";

export interface AppShellProps<V extends string = string> extends Omit<
  AppSidebarProps<V>,
  "collapsed" | "onCollapsedChange" | "collapsedWidth"
> {
  /** Controlled collapsed state. true = sidebar collapsed (icon mode). */
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /** Slot rendered above the main content (e.g. TopBar or Header). */
  headerSlot?: ReactNode;
  /** Main content. */
  children: ReactNode;
  /** Extra className on the content area. */
  contentClassName?: string;
  /** Show a full-width top bar above the sidebar+content row (desktop / macOS). */
  topBar?: boolean;
}

function HeaderBar({ headerSlot }: { headerSlot?: ReactNode }) {
  const breadcrumb = useBreadcrumb();
  return (
    <div className="flex flex-wrap items-center shrink-0">
      {breadcrumb && <div className="flex items-center h-10 pl-4">{breadcrumb}</div>}
      <div className="flex-1 min-w-0" />
      {headerSlot}
    </div>
  );
}

function MobileHeader({
  headerSlot,
  onOpenMenu,
}: {
  headerSlot?: ReactNode;
  onOpenMenu: () => void;
}) {
  const breadcrumb = useBreadcrumb();
  return (
    <div
      className="flex items-center shrink-0 h-12 px-3 gap-2"
      style={{ borderBottom: "1px solid var(--sidebar-border, hsl(var(--border)))" }}
    >
      <button
        onClick={onOpenMenu}
        className="flex items-center justify-center w-8 h-8 rounded bg-transparent border-none cursor-pointer text-foreground"
        aria-label="Open menu"
      >
        <Menu className="w-5 h-5" />
      </button>
      {breadcrumb && <div className="flex items-center">{breadcrumb}</div>}
      <div className="flex-1 min-w-0" />
      {headerSlot}
    </div>
  );
}

function MobileDrawer<V extends string = string>({
  open,
  onClose,
  sidebarProps,
}: {
  open: boolean;
  onClose: () => void;
  sidebarProps: Omit<AppSidebarProps<V>, "collapsed" | "onCollapsedChange">;
}) {
  // Lock body scroll when drawer is open
  useEffect(() => {
    if (open) {
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = "";
      };
    }
  }, [open]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" onClick={onClose} />
      {/* Drawer panel */}
      <div
        className="relative z-10 flex flex-col h-full w-[280px] max-w-[80vw]"
        style={{
          background: "var(--sidebar-bg, hsl(var(--card)))",
          borderRight: "1px solid var(--sidebar-border, hsl(var(--border)))",
        }}
      >
        <div className="flex items-center justify-end px-3 py-2">
          <button
            onClick={onClose}
            className="flex items-center justify-center w-8 h-8 rounded bg-transparent border-none cursor-pointer text-foreground"
            aria-label="Close menu"
          >
            <X className="w-5 h-5" />
          </button>
        </div>
        <AppSidebar
          collapsed={false}
          onCollapsedChange={() => onClose()}
          {...sidebarProps}
          onViewChange={(view) => {
            sidebarProps.onViewChange(view);
            onClose();
          }}
        />
      </div>
    </div>
  );
}

export function AppShell<V extends string = string>({
  collapsed,
  onCollapsedChange,
  headerSlot,
  children,
  contentClassName,
  topBar: hasTopBar = false,
  ...sidebarProps
}: AppShellProps<V>) {
  const isMobile = useIsMobile();
  const [drawerOpen, setDrawerOpen] = useState(false);

  const handleOpenMenu = useCallback(() => setDrawerOpen(true), []);
  const handleCloseMenu = useCallback(() => setDrawerOpen(false), []);

  if (isMobile) {
    return (
      <>
        <AnimatedBackgroundGlass />
        <SidebarGlass.Provider
          open={false}
          onOpenChange={() => {}}
          collapsible="icon"
          keyboardShortcut={false}
        >
          <BreadcrumbProvider>
            <div className="relative z-10 flex flex-col h-screen overflow-hidden">
              <MobileHeader headerSlot={headerSlot} onOpenMenu={handleOpenMenu} />
              <div
                className={cn("flex-1 p-4 flex flex-col min-h-0 overflow-auto", contentClassName)}
              >
                {children}
              </div>
            </div>
            <MobileDrawer open={drawerOpen} onClose={handleCloseMenu} sidebarProps={sidebarProps} />
          </BreadcrumbProvider>
        </SidebarGlass.Provider>
      </>
    );
  }

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
            style={
              {
                "--sidebar-width": "220px",
                "--sidebar-width-icon": "60px",
              } as React.CSSProperties
            }
          >
            {hasTopBar && (
              <div
                className="shrink-0 flex items-center h-12 glass-surface bg-card/80"
                style={
                  {
                    "--wails-draggable": "drag",
                    borderBottom: "1px solid var(--sidebar-border)",
                  } as React.CSSProperties
                }
              >
                <div
                  className="shrink-0 transition-[width] duration-300 ease-in-out"
                  style={{
                    width: collapsed ? "var(--sidebar-width-icon)" : "var(--sidebar-width)",
                    minWidth: 78,
                  }}
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
                {...sidebarProps}
              />
              <SidebarGlass.Inset className="bg-transparent !min-h-0 flex flex-col">
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
