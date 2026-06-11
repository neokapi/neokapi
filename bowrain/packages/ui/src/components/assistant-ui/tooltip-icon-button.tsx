"use client";

import { Button, Tooltip, TooltipContent, TooltipTrigger, cn } from "@neokapi/ui-primitives";
import { ComponentPropsWithRef } from "react";
import { Slottable } from "@radix-ui/react-slot";

export type TooltipIconButtonProps = ComponentPropsWithRef<typeof Button> & {
  tooltip: string;
  side?: "top" | "bottom" | "left" | "right";
};

// React 19 passes `ref` through props, so no forwardRef wrapper is needed.
export function TooltipIconButton({
  children,
  tooltip,
  side = "bottom",
  className,
  ...rest
}: TooltipIconButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          {...rest}
          className={cn("aui-button-icon size-6 p-1", className)}
        >
          <Slottable>{children}</Slottable>
          <span className="aui-sr-only sr-only">{tooltip}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent side={side}>{tooltip}</TooltipContent>
    </Tooltip>
  );
}
