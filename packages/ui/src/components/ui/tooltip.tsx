"use client";

import * as React from "react";
import { Tooltip as TooltipPrimitive } from "radix-ui";

import { cn } from "../../lib/utils";
import { usePortalThemeClass } from "../../lib/portal-theme";

/**
 * Tracks whether a {@link TooltipProvider} is mounted above, so
 * {@link SimpleTooltip} can fall back to a local provider when a host (tests,
 * Storybook, embedded docs) renders it without an app-wide one.
 */
const TooltipProviderMountedContext = React.createContext(false);

function TooltipProvider({
  delayDuration = 0,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Provider>) {
  return (
    <TooltipProviderMountedContext.Provider value={true}>
      <TooltipPrimitive.Provider
        data-slot="tooltip-provider"
        delayDuration={delayDuration}
        {...props}
      />
    </TooltipProviderMountedContext.Provider>
  );
}

function Tooltip({ ...props }: React.ComponentProps<typeof TooltipPrimitive.Root>) {
  return <TooltipPrimitive.Root data-slot="tooltip" {...props} />;
}

function TooltipTrigger({ ...props }: React.ComponentProps<typeof TooltipPrimitive.Trigger>) {
  return <TooltipPrimitive.Trigger data-slot="tooltip-trigger" {...props} />;
}

function TooltipContent({
  className,
  sideOffset = 0,
  children,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  const portalThemeClass = usePortalThemeClass();
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        className={cn(
          portalThemeClass,
          "z-50 inline-flex w-fit max-w-xs origin-(--radix-tooltip-content-transform-origin) items-center gap-1.5 rounded-md bg-foreground px-3 py-1.5 text-xs text-background has-data-[slot=kbd]:pr-1.5 data-[side=bottom]:slide-in-from-top-2 data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2 data-[side=top]:slide-in-from-bottom-2 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=delayed-open]:zoom-in-95 data-[state=open]:animate-in data-[state=open]:fade-in-0 data-[state=open]:zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95",
          className,
        )}
        {...props}
      >
        {children}
        <TooltipPrimitive.Arrow className="z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-foreground fill-foreground" />
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  );
}

interface SimpleTooltipProps {
  /** Tooltip body. Falsy (`undefined`/`null`/`""`/`false`) renders the trigger alone. */
  content: React.ReactNode;
  /** A single element that can carry the trigger props (rendered via `asChild`). */
  children: React.ReactElement;
  side?: React.ComponentProps<typeof TooltipPrimitive.Content>["side"];
  align?: React.ComponentProps<typeof TooltipPrimitive.Content>["align"];
  sideOffset?: number;
  /** Extra classes for the tooltip content bubble. */
  className?: string;
}

/**
 * One-liner tooltip: `<SimpleTooltip content="…">{trigger}</SimpleTooltip>`.
 *
 * The standard replacement for native `title=` attributes. Expects a single
 * app-wide {@link TooltipProvider}; when none is mounted (tests, Storybook,
 * embedded docs) it provides its own. A falsy `content` renders the trigger
 * unchanged, so conditional tooltips stay a one-liner. Note that disabled
 * elements don't emit pointer events — wrap them in a `<span>` to keep the
 * tooltip firing.
 */
function SimpleTooltip({
  content,
  children,
  side,
  align,
  sideOffset,
  className,
}: SimpleTooltipProps) {
  const hasProvider = React.useContext(TooltipProviderMountedContext);
  if (content === undefined || content === null || content === "" || content === false) {
    return children;
  }
  const tooltip = (
    <Tooltip>
      <TooltipTrigger asChild>{children}</TooltipTrigger>
      <TooltipContent side={side} align={align} sideOffset={sideOffset} className={className}>
        {content}
      </TooltipContent>
    </Tooltip>
  );
  return hasProvider ? tooltip : <TooltipProvider>{tooltip}</TooltipProvider>;
}

export { SimpleTooltip, Tooltip, TooltipContent, TooltipProvider, TooltipTrigger };
