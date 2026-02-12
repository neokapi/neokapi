import * as React from "react";
import { useState, useRef, useEffect, useCallback } from "react";
import { cn } from "../../lib/utils";

interface DropdownMenuContextValue {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const DropdownMenuContext = React.createContext<DropdownMenuContextValue | null>(null);

function useDropdownMenuContext() {
  const ctx = React.useContext(DropdownMenuContext);
  if (!ctx) throw new Error("DropdownMenu components must be used within <DropdownMenu>");
  return ctx;
}

function DropdownMenu({ children, open: controlledOpen, onOpenChange }: {
  children: React.ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const [internalOpen, setInternalOpen] = useState(false);
  const open = controlledOpen ?? internalOpen;
  const handleOpenChange = useCallback(
    (v: boolean) => { onOpenChange ? onOpenChange(v) : setInternalOpen(v); },
    [onOpenChange],
  );

  return (
    <DropdownMenuContext value={{ open, onOpenChange: handleOpenChange }}>
      <div className="relative inline-block">{children}</div>
    </DropdownMenuContext>
  );
}

function DropdownMenuTrigger({ className, children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) {
  const { open, onOpenChange } = useDropdownMenuContext();
  return (
    <button className={className} onClick={() => onOpenChange(!open)} {...props}>
      {children}
    </button>
  );
}

function DropdownMenuContent({ className, align = "end", children, ...props }: React.HTMLAttributes<HTMLDivElement> & { align?: "start" | "end" }) {
  const { open, onOpenChange } = useDropdownMenuContext();
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onOpenChange(false);
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div
      ref={ref}
      className={cn(
        "absolute top-full z-50 mt-1 min-w-[8rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md",
        align === "end" ? "right-0" : "left-0",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}

function DropdownMenuItem({ className, onClick, children, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  const { onOpenChange } = useDropdownMenuContext();
  return (
    <div
      className={cn(
        "relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-sm outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus:bg-accent focus:text-accent-foreground cursor-pointer",
        className,
      )}
      onClick={(e) => {
        onClick?.(e);
        onOpenChange(false);
      }}
      role="menuitem"
      {...props}
    >
      {children}
    </div>
  );
}

function DropdownMenuSeparator({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("-mx-1 my-1 h-px bg-border", className)} role="separator" {...props} />;
}

function DropdownMenuLabel({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("px-2 py-1.5 text-sm font-semibold", className)} {...props} />;
}

export {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuLabel,
};
