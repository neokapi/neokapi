import { type ReactNode, useState, useEffect } from "react";
import {
  cn,
  SidebarProvider,
  SidebarInset,
  SidebarTrigger,
  useSidebar,
  BreadcrumbProvider,
  useBreadcrumb,
  Separator,
} from "@neokapi/ui";
import { CtrlSidebar } from "./CtrlSidebar";

export interface CtrlShellProps {
  children: ReactNode;
  headerSlot?: ReactNode;
  contentClassName?: string;
}

function Header({ headerSlot }: { headerSlot?: ReactNode }) {
  const breadcrumb = useBreadcrumb();
  const { isMobile } = useSidebar();

  return (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b">
      <div className="flex items-center gap-2 px-4">
        {isMobile && (
          <>
            <SidebarTrigger className="-ml-1" />
            <Separator orientation="vertical" className="mr-2 data-[orientation=vertical]:h-4" />
          </>
        )}
        <span className="text-sm font-medium">{breadcrumb}</span>
      </div>
      <div className="flex-1 min-w-0" />
      {headerSlot && <div className="flex items-center gap-1 px-4">{headerSlot}</div>}
    </header>
  );
}

/** True when viewport is >= 1024px (lg breakpoint). */
function useIsLargeScreen(): boolean {
  const [isLarge, setIsLarge] = useState(
    typeof window !== "undefined" ? window.matchMedia("(min-width: 1024px)").matches : true,
  );
  useEffect(() => {
    const mql = window.matchMedia("(min-width: 1024px)");
    const handler = (e: MediaQueryListEvent) => setIsLarge(e.matches);
    mql.addEventListener("change", handler);
    return () => mql.removeEventListener("change", handler);
  }, []);
  return isLarge;
}

export function CtrlShell({ children, headerSlot, contentClassName }: CtrlShellProps) {
  const isLargeScreen = useIsLargeScreen();

  return (
    <SidebarProvider defaultOpen={isLargeScreen}>
      <BreadcrumbProvider>
        <CtrlSidebar />
        <SidebarInset>
          <Header headerSlot={headerSlot} />
          <div className="flex flex-1 min-h-0 min-w-0 overflow-hidden">
            <div
              className={cn(
                "flex-1 flex flex-col min-h-0 min-w-0 overflow-auto p-4",
                contentClassName,
              )}
            >
              {children}
            </div>
          </div>
        </SidebarInset>
      </BreadcrumbProvider>
    </SidebarProvider>
  );
}
