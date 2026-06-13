import {
  Separator,
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
  cn,
  useSidebar,
} from "@neokapi/ui-primitives";
import { useRef, type ReactNode } from "react";
import { AppSidebar, type AppSidebarProps, type SubNavItem, subNavConfig } from "./AppSidebar";
import { BreadcrumbProvider, useBreadcrumb } from "../context/BreadcrumbContext";

export interface AppShellProps<V extends string = string> extends Omit<
  AppSidebarProps<V>,
  "collapsed" | "onCollapsedChange" | "collapsedWidth"
> {
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  headerSlot?: ReactNode;
  /** Slot for a right-side panel (e.g. @bravo chat). Rendered inline beside main content. */
  rightPanelSlot?: ReactNode;
  children: ReactNode;
  contentClassName?: string;
  topBar?: boolean;
}

export type { SidebarContext } from "./AppSidebar";

function Header({ headerSlot }: { headerSlot?: ReactNode }) {
  const breadcrumb = useBreadcrumb();
  const { isMobile } = useSidebar();

  return (
    <header className="flex h-12 shrink-0 items-center gap-2">
      <div className="flex items-center gap-2 px-4">
        {isMobile && (
          <>
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 data-[orientation=vertical]:h-4" />
          </>
        )}
        {breadcrumb}
      </div>
      <div className="flex-1 min-w-0" />
      {headerSlot && <div className="flex items-center gap-1 px-4">{headerSlot}</div>}
    </header>
  );
}

function SecondaryPanel({
  title,
  items,
  activeId,
  onSelect,
  open,
}: {
  title: string;
  items: SubNavItem[];
  activeId?: string;
  onSelect: (id: string) => void;
  open: boolean;
}) {
  return (
    <div
      className={cn(
        "shrink-0 bg-sidebar text-sidebar-foreground overflow-hidden transition-[width,border-width] duration-200 ease-in-out hidden md:flex flex-col",
        open ? "border-r" : "border-r-0",
      )}
      style={{ width: open ? 208 : 0 }}
    >
      <div className="flex flex-col h-full w-52">
        <div className="px-4 py-3">
          <h2 className="text-sm font-medium whitespace-nowrap">{title}</h2>
        </div>
        <nav className="flex-1 px-2 pb-2">
          <ul className="flex flex-col gap-0.5">
            {items.map((item) => (
              <li key={item.id}>
                <button
                  onClick={() => onSelect(item.id)}
                  tabIndex={open ? 0 : -1}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none border-none cursor-pointer bg-transparent whitespace-nowrap",
                    activeId === item.id
                      ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                      : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                  )}
                >
                  <span className="[&_svg]:size-4 shrink-0">{item.icon}</span>
                  <span>{item.label}</span>
                </button>
              </li>
            ))}
          </ul>
        </nav>
      </div>
    </div>
  );
}

const viewLabels: Record<string, string> = {
  translate: "Projects",
  brand: "Brand",
  memory: "Memory",
  auditlog: "Audit Log",
  bin: "Recycle Bin",
  settings: "Settings",
};

export function AppShell<V extends string = string>({
  collapsed,
  onCollapsedChange,
  headerSlot,
  rightPanelSlot,
  children,
  contentClassName,
  activeSubNav,
  onSubNavChange,
  ...sidebarProps
}: AppShellProps<V>) {
  const activeView =
    sidebarProps.sidebarContext?.level === "workspace"
      ? sidebarProps.sidebarContext.activeView
      : undefined;
  const subNavItems = activeView ? subNavConfig[activeView] : undefined;
  const showSecondary = !!(subNavItems && onSubNavChange);

  const lastSubNavRef = useRef<{ items: SubNavItem[]; title: string } | null>(null);
  if (subNavItems && activeView) {
    lastSubNavRef.current = { items: subNavItems, title: viewLabels[activeView] ?? activeView };
  }

  const panelData = showSecondary
    ? { items: subNavItems!, title: viewLabels[activeView!] ?? activeView! }
    : lastSubNavRef.current;

  return (
    <SidebarProvider>
      <BreadcrumbProvider>
        <AppSidebar
          collapsed={collapsed}
          onCollapsedChange={onCollapsedChange}
          activeSubNav={activeSubNav}
          onSubNavChange={onSubNavChange}
          {...sidebarProps}
        />
        <SidebarInset>
          <Header headerSlot={headerSlot} />
          <div className="flex flex-1 min-h-0 min-w-0 overflow-hidden">
            {panelData && onSubNavChange && (
              <SecondaryPanel
                title={panelData.title}
                items={panelData.items}
                activeId={activeSubNav}
                onSelect={onSubNavChange}
                open={showSecondary}
              />
            )}
            <div
              className={cn(
                "flex-1 flex flex-col min-h-0 min-w-0 overflow-auto p-4",
                contentClassName,
              )}
            >
              {children}
            </div>
            {rightPanelSlot}
          </div>
        </SidebarInset>
      </BreadcrumbProvider>
    </SidebarProvider>
  );
}
