"use client";

import { Button, Tooltip, TooltipContent, TooltipTrigger, cn } from "@neokapi/ui-primitives";
import { ComponentPropsWithRef, forwardRef } from "react";
import { Slottable } from "@radix-ui/react-slot";

export type TooltipIconButtonProps = ComponentPropsWithRef<typeof Button> & {
  tooltip: string;
  side?: "top" | "bottom" | "left" | "right";
};

export const TooltipIconButton = forwardRef<HTMLButtonElement, TooltipIconButtonProps>(
  ({ children, tooltip, side = "bottom", className, ...rest }, ref) => {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            {...rest}
            className={cn("aui-button-icon size-6 p-1", className)}
            ref={ref}
          >
            <Slottable>{children}</Slottable>
            <span className="aui-sr-only sr-only">{tooltip}</span>
          </Button>
        </TooltipTrigger>
        <TooltipContent side={side}>{tooltip}</TooltipContent>
      </Tooltip>
    );
  },
);

TooltipIconButton.displayName = "TooltipIconButton";
