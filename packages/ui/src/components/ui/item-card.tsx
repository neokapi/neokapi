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
 */
export function ItemCard({ selected, clickable, className, children, ...props }: ItemCardProps) {
  return (
    <Card
      className={cn(
        "group p-4 transition-colors",
        clickable && "cursor-pointer hover:border-primary/30",
        selected && "border-primary/40 bg-primary/5",
        className,
      )}
      {...props}
    >
      {children}
    </Card>
  );
}
