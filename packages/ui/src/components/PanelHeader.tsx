import * as React from "react";
import { cn } from "../lib/utils";

interface PanelHeaderProps {
  title?: string;
  children?: React.ReactNode;
  actions?: React.ReactNode;
  className?: string;
}

export function PanelHeader({ title, children, actions, className }: PanelHeaderProps) {
  return (
    <div className={cn("flex items-center gap-2 border-b border-border bg-background px-3 py-2", className)}>
      {title && <span className="text-xs font-semibold text-muted-foreground">{title}</span>}
      {children}
      {actions && <div className="ml-auto flex items-center gap-1">{actions}</div>}
    </div>
  );
}
