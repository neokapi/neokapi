import type { ReactNode } from "react";
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

export function CtrlShell({ children, headerSlot, contentClassName }: CtrlShellProps) {
  return (
    <SidebarProvider>
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
