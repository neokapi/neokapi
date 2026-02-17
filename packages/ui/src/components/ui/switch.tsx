"use client";

import * as React from "react";
import { cn } from "../../lib/utils";

interface SwitchProps {
  checked?: boolean;
  defaultChecked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
  disabled?: boolean;
  id?: string;
  className?: string;
}

const Switch = React.forwardRef<HTMLButtonElement, SwitchProps>(
  ({ checked, defaultChecked, onCheckedChange, disabled, id, className }, ref) => {
    const [internalChecked, setInternalChecked] = React.useState(
      defaultChecked ?? false
    );
    const isControlled = checked !== undefined;
    const currentChecked = isControlled ? checked : internalChecked;

    const handleClick = React.useCallback(() => {
      if (disabled) return;
      const newValue = !currentChecked;
      if (!isControlled) {
        setInternalChecked(newValue);
      }
      onCheckedChange?.(newValue);
    }, [currentChecked, disabled, isControlled, onCheckedChange]);

    return (
      <button
        ref={ref}
        id={id}
        type="button"
        role="switch"
        aria-checked={currentChecked}
        disabled={disabled}
        onClick={handleClick}
        className={cn(
          "peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent",
          "transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background",
          "disabled:cursor-not-allowed disabled:opacity-50",
          currentChecked ? "bg-primary" : "bg-input",
          className
        )}
      >
        <span
          className={cn(
            "pointer-events-none block h-4 w-4 rounded-full bg-background shadow-lg ring-0 transition-transform",
            currentChecked ? "translate-x-4" : "translate-x-0"
          )}
        />
      </button>
    );
  }
);
Switch.displayName = "Switch";

export { Switch };
export type { SwitchProps };
