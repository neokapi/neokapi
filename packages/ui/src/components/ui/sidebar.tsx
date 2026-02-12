import * as React from "react";
import { createContext, useContext, useState, useCallback } from "react";
import { cn } from "../../lib/utils";

const SIDEBAR_WIDTH = "16rem";
const SIDEBAR_WIDTH_ICON = "3rem";

interface SidebarContextValue {
  state: "expanded" | "collapsed";
  open: boolean;
  setOpen: (open: boolean) => void;
  toggleSidebar: () => void;
}

const SidebarContext = createContext<SidebarContextValue | null>(null);

export function useSidebar() {
  const ctx = useContext(SidebarContext);
  if (!ctx) throw new Error("useSidebar must be used within <SidebarProvider>");
  return ctx;
}

interface SidebarProviderProps extends React.HTMLAttributes<HTMLDivElement> {
  defaultOpen?: boolean;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

function SidebarProvider({
  defaultOpen = true,
  open: controlledOpen,
  onOpenChange,
  className,
  children,
  ...props
}: SidebarProviderProps) {
  const [internalOpen, setInternalOpen] = useState(defaultOpen);
  const open = controlledOpen ?? internalOpen;
  const setOpen = useCallback(
    (v: boolean) => { onOpenChange ? onOpenChange(v) : setInternalOpen(v); },
    [onOpenChange],
  );
  const toggleSidebar = useCallback(() => setOpen(!open), [open, setOpen]);
  const state = open ? "expanded" : "collapsed";

  return (
    <SidebarContext value={{ state, open, setOpen, toggleSidebar }}>
      <div
        className={cn("group/sidebar-wrapper flex min-h-svh w-full", className)}
        style={{
          "--sidebar-width": SIDEBAR_WIDTH,
          "--sidebar-width-icon": SIDEBAR_WIDTH_ICON,
        } as React.CSSProperties}
        {...props}
      >
        {children}
      </div>
    </SidebarContext>
  );
}

interface SidebarProps extends React.HTMLAttributes<HTMLElement> {
  side?: "left" | "right";
  collapsible?: "offcanvas" | "icon" | "none";
}

function Sidebar({
  side = "left",
  collapsible = "offcanvas",
  className,
  children,
  ...props
}: SidebarProps) {
  const { state } = useSidebar();

  if (collapsible === "none") {
    return (
      <div
        className={cn(
          "flex h-full w-[--sidebar-width] flex-col bg-sidebar text-sidebar-foreground",
          className,
        )}
        {...props}
      >
        {children}
      </div>
    );
  }

  return (
    <div
      className="group peer"
      data-state={state}
      data-collapsible={state === "collapsed" ? collapsible : ""}
      data-side={side}
    >
      {/* Gap placeholder */}
      <div
        className={cn(
          "relative w-[--sidebar-width] bg-transparent transition-[width] duration-200 ease-linear",
          "group-data-[collapsible=offcanvas]:w-0",
          "group-data-[collapsible=icon]:w-[--sidebar-width-icon]",
        )}
      />
      {/* Actual sidebar */}
      <div
        className={cn(
          "fixed inset-y-0 z-10 flex h-svh w-[--sidebar-width] flex-col bg-sidebar transition-[left,right,width] duration-200 ease-linear",
          side === "left"
            ? "left-0 border-r border-sidebar-border group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]"
            : "right-0 border-l border-sidebar-border group-data-[collapsible=offcanvas]:right-[calc(var(--sidebar-width)*-1)]",
          "group-data-[collapsible=icon]:w-[--sidebar-width-icon] group-data-[collapsible=icon]:overflow-hidden",
          className,
        )}
        {...props}
      >
        {children}
      </div>
    </div>
  );
}

function SidebarHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-2 p-2", className)} {...props} />;
}

function SidebarContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("flex min-h-0 flex-1 flex-col gap-2 overflow-auto", className)}
      {...props}
    />
  );
}

function SidebarFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("flex flex-col gap-2 p-2", className)} {...props} />;
}

function SidebarGroup({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("relative flex w-full min-w-0 flex-col p-2", className)} {...props} />;
}

function SidebarGroupLabel({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "flex h-8 shrink-0 items-center rounded-md px-2 text-xs font-medium text-sidebar-foreground/70 outline-none",
        "group-data-[collapsible=icon]:hidden",
        className,
      )}
      {...props}
    />
  );
}

function SidebarGroupContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("w-full text-sm", className)} {...props} />;
}

function SidebarMenu({ className, ...props }: React.HTMLAttributes<HTMLUListElement>) {
  return <ul className={cn("flex w-full min-w-0 flex-col gap-1", className)} {...props} />;
}

function SidebarMenuItem({ className, ...props }: React.HTMLAttributes<HTMLLIElement>) {
  return <li className={cn("group/menu-item relative", className)} {...props} />;
}

interface SidebarMenuButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean;
  tooltip?: string;
  size?: "default" | "sm" | "lg";
}

function SidebarMenuButton({
  isActive = false,
  tooltip,
  size = "default",
  className,
  children,
  ...props
}: SidebarMenuButtonProps) {
  const sizeClasses = {
    default: "h-8 text-sm",
    sm: "h-7 text-xs",
    lg: "h-12 text-sm group-data-[collapsible=icon]:!p-0",
  };

  return (
    <button
      className={cn(
        "peer/menu-button flex w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left outline-none ring-sidebar-ring transition-[width,height,padding] hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 active:bg-sidebar-accent active:text-sidebar-accent-foreground disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50 [&>svg]:size-4 [&>svg]:shrink-0",
        isActive && "bg-sidebar-accent text-sidebar-accent-foreground font-medium",
        sizeClasses[size],
        "group-data-[collapsible=icon]:!size-8 group-data-[collapsible=icon]:!p-2 [&>span:last-child]:truncate group-data-[collapsible=icon]:[&>span:last-child]:hidden",
        className,
      )}
      title={tooltip}
      data-active={isActive}
      {...props}
    >
      {children}
    </button>
  );
}

function SidebarTrigger({ className, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const { toggleSidebar } = useSidebar();
  return (
    <button
      className={cn("h-7 w-7 inline-flex items-center justify-center", className)}
      onClick={toggleSidebar}
      {...props}
    >
      <span className="text-sm">{"\u2630"}</span>
      <span className="sr-only">Toggle Sidebar</span>
    </button>
  );
}

function SidebarInset({ className, ...props }: React.HTMLAttributes<HTMLElement>) {
  return (
    <main
      className={cn(
        "relative flex min-h-svh flex-1 flex-col bg-background",
        className,
      )}
      {...props}
    />
  );
}

function SidebarSeparator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("mx-2 w-auto bg-sidebar-border h-px", className)} {...props} />;
}

export {
  SidebarProvider,
  Sidebar,
  SidebarHeader,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuItem,
  SidebarMenuButton,
  SidebarTrigger,
  SidebarInset,
  SidebarSeparator,
};
