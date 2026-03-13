"use client";

import * as React from "react";
import { cn } from "../../lib/utils";

interface CollapsibleContextValue {
  open: boolean;
  toggle: () => void;
}

const CollapsibleContext = React.createContext<CollapsibleContextValue | undefined>(undefined);

interface CollapsibleProps {
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
  children: React.ReactNode;
  className?: string;
}

function Collapsible({
  open: controlledOpen,
  defaultOpen = false,
  onOpenChange,
  children,
  className,
}: CollapsibleProps) {
  const [internalOpen, setInternalOpen] = React.useState(defaultOpen);
  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : internalOpen;

  const toggle = React.useCallback(() => {
    const newValue = !open;
    if (!isControlled) {
      setInternalOpen(newValue);
    }
    onOpenChange?.(newValue);
  }, [open, isControlled, onOpenChange]);

  return (
    <CollapsibleContext.Provider value={{ open, toggle }}>
      <div className={cn("", className)} data-state={open ? "open" : "closed"}>
        {children}
      </div>
    </CollapsibleContext.Provider>
  );
}

function useCollapsible() {
  const context = React.useContext(CollapsibleContext);
  if (!context) {
    throw new Error("useCollapsible must be used within a Collapsible");
  }
  return context;
}

interface CollapsibleTriggerProps {
  asChild?: boolean;
  children: React.ReactNode;
  className?: string;
}

function CollapsibleTrigger({ asChild, children, className }: CollapsibleTriggerProps) {
  const { open, toggle } = useCollapsible();

  if (asChild && React.isValidElement(children)) {
    return React.cloneElement(
      children as React.ReactElement<{
        onClick?: () => void;
        "data-state"?: string;
      }>,
      {
        onClick: toggle,
        "data-state": open ? "open" : "closed",
      },
    );
  }

  return (
    <button
      type="button"
      onClick={toggle}
      data-state={open ? "open" : "closed"}
      className={className}
    >
      {children}
    </button>
  );
}

interface CollapsibleContentProps {
  children: React.ReactNode;
  className?: string;
}

function CollapsibleContent({ children, className }: CollapsibleContentProps) {
  const { open } = useCollapsible();

  if (!open) return null;

  return (
    <div data-state={open ? "open" : "closed"} className={cn("", className)}>
      {children}
    </div>
  );
}

export { Collapsible, CollapsibleTrigger, CollapsibleContent };
export type { CollapsibleProps, CollapsibleTriggerProps, CollapsibleContentProps };
