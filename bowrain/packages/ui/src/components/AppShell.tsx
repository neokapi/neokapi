import {
  Separator,
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
  cn,
  useSidebar,
} from "@neokapi/ui-primitives";
import type { ReactNode } from "react";
import {
  AppSidebar,
  type AppSidebarProps,
  type SidebarContext as SidebarContextType,
  type SubNavItem,
  NavCount,
  openProjectSection,
  projectSubNavConfig,
  subNavConfig,
  viewLabel,
} from "./AppSidebar";
import {
  BreadcrumbProvider,
  useBreadcrumb,
  type BreadcrumbItem,
} from "../context/BreadcrumbContext";
import { BreadcrumbBar } from "./BreadcrumbBar";

export interface AppShellProps<V extends string = string> extends Omit<
  AppSidebarProps<V>,
  "collapsed" | "onCollapsedChange" | "collapsedWidth"
> {
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  /**
   * The trail the shell derives from the URL, shown in the header. A mounted
   * route appends what only it can name (a collection's title, a file) with
   * useBreadcrumbExtra.
   */
  breadcrumbs?: BreadcrumbItem[];
  headerSlot?: ReactNode;
  /** Slot for a right-side panel (e.g. @bravo chat). Rendered inline beside main content. */
  rightPanelSlot?: ReactNode;
  children: ReactNode;
  contentClassName?: string;
  topBar?: boolean;
}

export type { SidebarContext } from "./AppSidebar";

function Header({ headerSlot }: { headerSlot?: ReactNode }) {
  const breadcrumbs = useBreadcrumb();
  const { isMobile } = useSidebar();

  return (
    <header className="flex h-12 shrink-0 items-center gap-2">
      {/* The trail takes the room it needs and yields the rest: min-w-0 lets it
          shrink, so the top bar's controls keep their width on a deep page. */}
      <div className="flex min-w-0 flex-1 items-center gap-2 px-4">
        {isMobile && (
          <>
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 data-[orientation=vertical]:h-4" />
          </>
        )}
        <BreadcrumbBar items={breadcrumbs} />
      </div>
      {headerSlot && <div className="flex shrink-0 items-center gap-1 px-4">{headerSlot}</div>}
    </header>
  );
}

function SecondaryPanel({
  title,
  items,
  activeId,
  onSelect,
  counts,
}: {
  title: string;
  items: SubNavItem[];
  activeId?: string;
  onSelect: (id: string) => void;
  counts?: Record<string, number>;
}) {
  return (
    <div
      data-secondary-panel=""
      className="hidden w-52 shrink-0 flex-col overflow-hidden border-r bg-sidebar text-sidebar-foreground md:flex"
    >
      <div className="flex h-full min-h-0 w-52 flex-col">
        <div className="shrink-0 px-4 py-3">
          <h2 className="text-sm font-medium whitespace-nowrap">{title}</h2>
        </div>
        {/*
          The list must be the scroll container, not just a flex child. `flex-1`
          alone lets it shrink below its content height, and the panel above
          clips overflow — so on a short window the last entries (Settings has
          twelve) were simply unreachable. `min-h-0` allows the shrink and
          `overflow-y-auto` gives the user a way to reach the rest.
        */}
        <nav className="flex-1 min-h-0 overflow-y-auto px-2 pb-2">
          <ul className="flex flex-col gap-0.5">
            {items.map((item) => (
              <li key={item.id}>
                <button
                  onClick={() => onSelect(item.id)}
                  data-testid={`subnav-${item.id}`}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors outline-none border-none cursor-pointer bg-transparent whitespace-nowrap",
                    activeId === item.id
                      ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                      : "text-sidebar-foreground/70 hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                  )}
                >
                  <span className="[&_svg]:size-4 shrink-0">{item.icon}</span>
                  <span>{item.label}</span>
                  <NavCount count={counts?.[item.id]} />
                </button>
              </li>
            ))}
          </ul>
        </nav>
      </div>
    </div>
  );
}

export function AppShell<V extends string = string>({
  collapsed,
  onCollapsedChange,
  breadcrumbs,
  headerSlot,
  rightPanelSlot,
  children,
  contentClassName,
  activeSubNav,
  onSubNavChange,
  ...sidebarProps
}: AppShellProps<V>) {
  const sidebarContext = sidebarProps.sidebarContext;
  const projectCtx =
    sidebarContext?.level === "project"
      ? (sidebarContext as Extract<SidebarContextType, { level: "project" }>)
      : undefined;
  const activeView = sidebarContext?.level === "workspace" ? sidebarContext.activeView : undefined;

  // The panel serves two masters, one at a time: a project's own sections while
  // a project is open, and otherwise the current workspace view's sub-nav. Both
  // are the same second level of navigation, which is why they share one strip
  // rather than the project's taking over the rail.
  //
  // hiddenSubNavIds arrives via sidebarProps (inherited from AppSidebarProps) and
  // is also spread to AppSidebar below, so both sub-nav render paths stay in sync.
  const workspaceSubNav = activeView
    ? subNavConfig()[activeView]?.filter((item) => !sidebarProps.hiddenSubNavIds?.includes(item.id))
    : undefined;

  const panel:
    | { items: SubNavItem[]; title: string; activeId?: string; onSelect: (id: string) => void }
    | undefined = projectCtx
    ? {
        items: projectSubNavConfig(projectCtx),
        title: projectCtx.project.name,
        activeId: projectCtx.activeProjectView,
        onSelect: (id) => openProjectSection(projectCtx, id),
      }
    : workspaceSubNav && workspaceSubNav.length > 0 && onSubNavChange && activeView
      ? {
          items: workspaceSubNav,
          title: viewLabel(activeView) ?? activeView,
          activeId: activeSubNav,
          onSelect: onSubNavChange,
        }
      : undefined;

  // The panel is mounted only while it has something to show. It used to stay
  // mounted at zero width so its contents could animate shut, which meant that
  // after leaving a project the previous project's sections were still in the
  // document: clickable, announceable, and matching any query for the project's
  // name. Two hundred milliseconds of polish is not worth a strip of stale
  // navigation that the page no longer has any way to reach.

  return (
    <SidebarProvider>
      <BreadcrumbProvider base={breadcrumbs}>
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
            {panel && (
              <SecondaryPanel
                title={panel.title}
                items={panel.items}
                activeId={panel.activeId}
                onSelect={panel.onSelect}
                counts={sidebarProps.subNavCounts}
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
