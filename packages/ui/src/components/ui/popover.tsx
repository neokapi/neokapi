import * as React from "react";
import { useState, useRef, useEffect, useCallback } from "react";
import { cn } from "../../lib/utils";

interface PopoverContextValue {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  triggerRef: React.RefObject<HTMLButtonElement | null>;
}

const PopoverContext = React.createContext<PopoverContextValue | null>(null);

function usePopoverContext() {
  const ctx = React.useContext(PopoverContext);
  if (!ctx) throw new Error("Popover components must be used within <Popover>");
  return ctx;
}

interface PopoverProps {
  children: React.ReactNode;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
}

function Popover({ children, open: controlledOpen, onOpenChange }: PopoverProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const open = controlledOpen ?? internalOpen;
  const handleOpenChange = useCallback(
    (v: boolean) => {
      onOpenChange ? onOpenChange(v) : setInternalOpen(v);
    },
    [onOpenChange],
  );

  return (
    <PopoverContext value={{ open, onOpenChange: handleOpenChange, triggerRef }}>
      <div className="relative">{children}</div>
    </PopoverContext>
  );
}

const PopoverTrigger = React.forwardRef<HTMLButtonElement, React.ButtonHTMLAttributes<HTMLButtonElement>>(
  ({ className, onClick, ...props }, ref) => {
    const { open, onOpenChange, triggerRef } = usePopoverContext();
    return (
      <button
        ref={(node) => {
          (triggerRef as React.MutableRefObject<HTMLButtonElement | null>).current = node;
          if (typeof ref === "function") ref(node);
          else if (ref) (ref as React.MutableRefObject<HTMLButtonElement | null>).current = node;
        }}
        className={className}
        onClick={(e) => {
          onOpenChange(!open);
          onClick?.(e);
        }}
        {...props}
      />
    );
  },
);
PopoverTrigger.displayName = "PopoverTrigger";

interface PopoverContentProps extends React.HTMLAttributes<HTMLDivElement> {
  align?: "start" | "center" | "end";
  side?: "top" | "bottom";
  sideOffset?: number;
}

function PopoverContent({
  className,
  align = "center",
  side = "bottom",
  children,
  ...props
}: PopoverContentProps) {
  const { open, onOpenChange } = usePopoverContext();
  const contentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const handleClick = (e: MouseEvent) => {
      if (contentRef.current && !contentRef.current.contains(e.target as Node)) {
        onOpenChange(false);
      }
    };
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [open, onOpenChange]);

  if (!open) return null;

  const alignClasses = {
    start: "left-0",
    center: "left-1/2 -translate-x-1/2",
    end: "right-0",
  };

  const sideClasses = {
    top: "bottom-full mb-1",
    bottom: "top-full mt-1",
  };

  return (
    <div
      ref={contentRef}
      className={cn(
        "absolute z-50 min-w-[8rem] rounded-md border border-border bg-popover p-4 text-popover-foreground shadow-md outline-none",
        sideClasses[side],
        alignClasses[align],
        className,
      )}
      onMouseDown={(e) => e.stopPropagation()}
      {...props}
    >
      {children}
    </div>
  );
}

export { Popover, PopoverTrigger, PopoverContent };
