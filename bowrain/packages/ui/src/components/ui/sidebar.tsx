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

  const sidebarStyle: React.CSSProperties = {
    background: "var(--sidebar-bg)",
    color: "var(--semantic-text)",
    backdropFilter: "blur(var(--sidebar-backdrop-blur, 16px))",
    WebkitBackdropFilter: "blur(var(--sidebar-backdrop-blur, 16px))",
  };

  if (collapsible === "none") {
    return (
      <div
        className={cn(
          "flex h-full w-[--sidebar-width] flex-col",
          className,
        )}
        style={{
          ...sidebarStyle,
          borderRight: "1px solid var(--semantic-border)",
        }}
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
          "fixed inset-y-0 z-10 flex h-svh w-[--sidebar-width] flex-col transition-[left,right,width] duration-200 ease-linear",
          side === "left"
            ? "left-0 group-data-[collapsible=offcanvas]:left-[calc(var(--sidebar-width)*-1)]"
            : "right-0 group-data-[collapsible=offcanvas]:right-[calc(var(--sidebar-width)*-1)]",
          "group-data-[collapsible=icon]:w-[--sidebar-width-icon] group-data-[collapsible=icon]:overflow-hidden",
          className,
        )}
        style={{
          ...sidebarStyle,
          borderRight: side === "left" ? "1px solid var(--semantic-border)" : undefined,
          borderLeft: side === "right" ? "1px solid var(--semantic-border)" : undefined,
          boxShadow: "var(--sidebar-glow, none)",
        }}
        {...props}
      >
        {children}
      </div>
    </div>
  );
}

function SidebarHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-sidebar="header"
      className={cn("flex shrink-0 flex-col gap-2 p-4 border-b", className)}
      style={{ borderColor: "var(--semantic-border)" }}
      {...props}
    />
  );
}

function SidebarContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-sidebar="content"
      className={cn(
        "flex min-h-0 flex-1 flex-col gap-4 overflow-auto p-4",
        "[&::-webkit-scrollbar]:w-1.5",
        "[&::-webkit-scrollbar-track]:bg-transparent",
        "[&::-webkit-scrollbar-thumb]:rounded-full",
        "[&::-webkit-scrollbar-thumb]:bg-white/10",
        className,
      )}
      {...props}
    />
  );
}

function SidebarFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-sidebar="footer"
      className={cn("flex shrink-0 flex-col gap-2 p-4 border-t", className)}
      style={{ borderColor: "var(--semantic-border)" }}
      {...props}
    />
  );
}

function SidebarGroup({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div data-sidebar="group" className={cn("flex flex-col gap-2 p-2", className)} {...props} />;
}

function SidebarGroupLabel({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-sidebar="group-label"
      className={cn(
        "flex h-8 shrink-0 items-center px-2 text-xs font-medium",
        "group-data-[collapsible=icon]:hidden",
        className,
      )}
      style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
      {...props}
    />
  );
}

function SidebarGroupContent({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div data-sidebar="group-content" className={cn("flex flex-col gap-1", className)} {...props} />;
}

function SidebarMenu({ className, ...props }: React.HTMLAttributes<HTMLUListElement>) {
  return <ul data-sidebar="menu" className={cn("flex w-full min-w-0 flex-col gap-1", className)} {...props} />;
}

function SidebarMenuItem({ className, ...props }: React.HTMLAttributes<HTMLLIElement>) {
  return (
    <li
      data-sidebar="menu-item"
      className={cn("group/menu-item relative", className)}
      style={{ color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)" }}
      {...props}
    />
  );
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
      data-sidebar="menu-button"
      data-active={isActive}
      data-size={size}
      className={cn(
        "peer/menu-button flex w-full items-center gap-2 overflow-hidden rounded-md px-2 text-left outline-none transition-[width,height,padding,background,color] duration-200 ease-linear",
        "focus-visible:outline-none focus-visible:ring-2",
        "disabled:pointer-events-none disabled:opacity-50",
        "aria-disabled:pointer-events-none aria-disabled:opacity-50",
        "[&>span:last-child]:truncate [&>svg]:size-4 [&>svg]:shrink-0",
        sizeClasses[size],
        "group-data-[collapsible=icon]:!size-8 group-data-[collapsible=icon]:!px-0 group-data-[collapsible=icon]:justify-center group-data-[collapsible=icon]:[&>span:last-child]:hidden",
        className,
      )}
      style={
        isActive
          ? {
              background: "var(--semantic-primary)",
              color: "var(--semantic-text-inverse)",
            }
          : {
              color: "color-mix(in srgb, var(--semantic-text) 60%, transparent)",
            }
      }
      onMouseEnter={(e) => {
        if (!isActive) {
          e.currentTarget.style.background = "var(--semantic-surface-elevated)";
          e.currentTarget.style.color = "var(--semantic-text)";
        }
        props.onMouseEnter?.(e);
      }}
      onMouseLeave={(e) => {
        if (!isActive) {
          e.currentTarget.style.background = "";
          e.currentTarget.style.color = "color-mix(in srgb, var(--semantic-text) 60%, transparent)";
        }
        props.onMouseLeave?.(e);
      }}
      title={tooltip}
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
      data-sidebar="trigger"
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
      data-sidebar="inset"
      className={cn(
        "relative flex min-h-svh flex-1 flex-col bg-background",
        className,
      )}
      {...props}
    />
  );
}

function SidebarSeparator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      data-sidebar="separator"
      className={cn("mx-4 my-2 h-px border-0", className)}
      style={{ background: "var(--semantic-border)" }}
      {...props}
    />
  );
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
