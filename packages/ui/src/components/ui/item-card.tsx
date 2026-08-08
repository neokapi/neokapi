/**
 * ItemCard — universal card for list items. Built on shadcn Card.
 *
 * Standardizes card appearance across all pages: consistent padding,
 * hover states, selection highlighting, and group behavior for child
 * hover effects (e.g., reveal delete buttons).
 */

import { Card } from "./card";
import { cn } from "../../lib/utils";

export interface ItemCardProps extends React.ComponentPropsWithRef<"div"> {
  /** Whether the card is in a selected state. */
  selected?: boolean;
  /** Make the card clickable with cursor-pointer. */
  clickable?: boolean;
}

/**
 * Standard item card with consistent styling. React 19 passes `ref` through
 * props, so no forwardRef wrapper is needed.
 * Use `group` class for child hover effects (e.g., `group-hover:opacity-100`).
 *
 * A `clickable` card is a button: reachable by Tab and activated by Enter/Space,
 * with a focus ring. Nested action buttons keep their own keys — the card only
 * activates when it itself holds focus.
 */
export function ItemCard({
  selected,
  clickable,
  className,
  children,
  onKeyDown,
  ...props
}: ItemCardProps) {
  return (
    <Card
      role={clickable ? "button" : undefined}
      tabIndex={clickable ? 0 : undefined}
      onKeyDown={(e: React.KeyboardEvent<HTMLDivElement>) => {
        if (clickable && e.currentTarget === e.target && (e.key === "Enter" || e.key === " ")) {
          e.preventDefault();
          props.onClick?.(e as unknown as React.MouseEvent<HTMLDivElement>);
        }
        onKeyDown?.(e);
      }}
      className={cn(
        "group p-4 transition-colors",
        clickable &&
          "cursor-pointer outline-none hover:border-primary/30 focus-visible:ring-2 focus-visible:ring-ring",
        selected && "border-primary/40 bg-primary/5",
        className,
      )}
      {...props}
    >
      {children}
    </Card>
  );
}
